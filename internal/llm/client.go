// internal/llm/client.go
package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"tallymind/internal/ledger"
	"time"

	"strings"
)

type Client struct {
	cfg          Config
	providerPool *ProviderPool
	httpClient   *http.Client
}

// toDataURI 智能多媒体转码器 (支持：图片、PDF文档、音频、视频全部格式)
func (c *Client) toDataURI(ctx context.Context, mediaURL string) (string, error) {
	//1. 如果已经是 Data URI (Base64)，直接返回
	if strings.HasPrefix(mediaURL, "data:") {
		return mediaURL, nil
	}

	var mediaBytes []byte
	var mimeType string

	// 2. 如果是远程 HTTP/HTTPS 链接 -> 下载到内存
	if strings.HasPrefix(mediaURL, "http://") || strings.HasPrefix(mediaURL, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
		if err != nil {
			return "", fmt.Errorf("创建媒体文件下载请求失败: %w", err)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("下载远程媒体文件失败: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("下载远程媒体文件错误码 %d", resp.StatusCode)
		}

		// 优先从 HTTP Header 提取真实的 MIME 类型并剥离 charset

		mimeType, _, _ = mime.ParseMediaType(resp.Header.Get("Content-Type"))

		mediaBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("read bytes failed: %w", err)
		}
	} else {
		// 3. 如果是本地文件路径 -> 直接读取
		var err error
		mediaBytes, err = os.ReadFile(mediaURL)
		if err != nil {
			return "", fmt.Errorf("read file failed: %w", err)
		}

	}

	// 3. 智能三级 MIME 探测与标准化修正
	// A. 如果 Header 没有，用文件头 Magic Bytes 自动嗅探
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType, _, _ = mime.ParseMediaType(http.DetectContentType(mediaBytes))

	}

	// B. 如果仍是泛类型，让 Go 标准库查系统扩展名字典 (自动识别 .mp3, .wav, .pdf, .mp4 等几百种后缀)
	if mimeType == "" || mimeType == "application/octet-stream" {
		if extMime := mime.TypeByExtension(filepath.Ext(mediaURL)); extMime != "" {
			mimeType, _, _ = mime.ParseMediaType(extMime)
		}
	}
	// C. 终极默认兜底 (确保永远不为空)
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = "image/jpeg"
	}
	slog.InfoContext(ctx, "[LLM] 成功加载多模态文件", "size_bytes", len(mediaBytes), "mime", mimeType)

	// 4. 组装为标准的 Base64 Data URI
	base64Str := base64.StdEncoding.EncodeToString(mediaBytes)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64Str), nil
}

// buildSystemPrompt 动态生成 Prompt，传入当前日期供 AI 参考
func (c *Client) buildSystemPrompt(contextHints string) string {
	today := time.Now().Format("2006-01-02")
	year := today[:4]

	var extraHints string
	if strings.TrimSpace(contextHints) != "" {
		extraHints = "\n" + strings.TrimSpace(contextHints) + "\n"
	}

	return fmt.Sprintf(`你是一个专业的全模态财务记账助手。将用户发送的文本、语音、账单小票/发票截图提取为标准 Beancount JSON 数据。今日基准日期：%[1]s。%[2]s

【核心提取原则】：
1. amount: 实付金额绝对值（必须 > 0，退款/收入亦为正数，无有效交易返回 {"transactions": []}）；currency: 默认 "CNY"，见外币符号精准提取(如 USD, JPY)。
2. date: YYYY-MM-DD，结合今日(%[1]s)推算相对日期，未提及设为 ""。
3. payee: 店铺/商户/机构名称；narration: 商品明细或备注说明；type: "expense"(支出), "income"(收入), "refund"(退款)。
4. category: Beancount 科目，日常以 Expenses: 或 Income: 开头（如 Expenses:Food:Drinks）；期初建账/初始资金注入使用 Equity:Opening-Balances。
5. account: 结算账户(如 Assets:Bank:CMB, Liabilities:CreditCard:ICBC, Assets:WeChat:Wallet, Liabilities:Alipay:Huabei)。【必须见图文明确凭据才提取，严禁根据聊天渠道臆测，无凭据必须为 ""】。
6. tags: 字符串数组。提取特征标签（如周期扣费 "#recurring"、待报销 "#reimbursement"、特定场景如 "#medical"、"#renovation" 等），无特征设为 []。
7. metadata  (全字段必填或规范留空):
   - owner: 实际出资/付款人【必填】。默认填当前发信人；仅在凭证/文本明确指示他人付款时归一化为【实体映射表】标准Key。
   - beneficiary: 实际消费受益人【必填】。默认填当前发信人；若文本或小票发票明确为他人消费，优先归一化为【实体映射表】标准Key，表外对象推断为英文/拼音 (如 "parents", "colleague", "friends")。
   - invoice_status: 电子发票填 "done"，需开票/待报销填 "pending"，无发票设为 ""。
   - original_amount / discount_amount: 原价与优惠减免金额 (无则设为 "")。
   - time / location / link: 小票具体时间(HH:MM:SS)、分店地点、订单流水号 (无则设为 "")。


【输出 JSON 示例】：
{
  "transactions": [
    {
      "amount": 4.00,
      "currency": "CNY",
      "date": "%[1]s",
      "payee": "蜜雪冰城(中关村店)",
      "narration": "冰鲜柠檬水",
      "category": "Expenses:Food:Drinks",
      "account": "",
      "type": "expense",
      "tags": [],
      "metadata": {
        "owner": "member_a",
        "beneficiary": "member_a",
        "invoice_status": "",
        "original_amount": "6.00",
        "discount_amount": "2.00",
        "time": "14:20:00",
        "location": "中关村店",
        "link": "20260824001"
      }
    }
  ]
}

【要求】：只输出合法 JSON 对象，不含任何 Markdown 标记或多余废话。`, today, year, extraHints)
}

func NewClient(cfg Config) (*Client, error) {
	// 1. 必填配置校验：统一使用 fmt.Errorf 抛出明确错误
	pool, err := NewProviderPool(cfg.Providers)
	if err != nil {
		return nil, fmt.Errorf("初始化 LLM 提供商池失败: %w", err)
	}

	return &Client{
		cfg:          cfg,
		providerPool: pool,
		httpClient: &http.Client{
			Timeout: 20 * time.Second, // 20 秒超时控制
		},
	}, nil
}

func (c *Client) ParseTransaction(ctx context.Context, userText string, contextHints string, attachments ...Attachment) (*ledger.BatchTransactions, error) {
	trimmedText := strings.TrimSpace(userText)

	// 1. 组装多模态媒体部分
	var mediaParts []ContentPart
	for _, att := range attachments {
		if strings.TrimSpace(att.URL) == "" {
			continue
		}
		dataURI, err := c.toDataURI(ctx, att.URL)
		if err != nil {
			slog.WarnContext(ctx, "[LLM] 附件转DataURI失败", "type", att.Type, "err", err)
			continue
		}

		mediaParts = append(mediaParts, ContentPart{
			Type:     "image_url",
			ImageURL: &ImageURL{URL: dataURI},
		})
	}

	// 2. 前置防御拦截：既无文本也无有效附件
	if trimmedText == "" && len(mediaParts) == 0 {
		return nil, fmt.Errorf("未提供任何有效的账单文本或多模态附件")
	}

	// 3. 动态组装提示词
	promptText := "请仔细分析传入的信息，识别提取出所有的消费记账明细。"
	if trimmedText != "" {
		promptText = fmt.Sprintf("请分析传入的信息。用户的补充说明：%s", trimmedText)
	}

	// 4. 合并组装 Parts
	parts := make([]ContentPart, 0, len(mediaParts)+1)
	parts = append(parts, ContentPart{Type: "text", Text: promptText})
	parts = append(parts, mediaParts...)

	poolSize := c.providerPool.PoolSize()
	if poolSize == 0 {
		return nil, fmt.Errorf("LLM: 无可用的 Provider 服务商")
	}

	slog.DebugContext(ctx, "[LLM] 准备发起多模态请求",
		"parts_count", len(parts),
		"user_text", trimmedText,
		"pool_size", poolSize)

	// 5. 核心容灾重试循环：基于 ProviderPool 轮询与故障转移
	var respBytes []byte
	var lastErr error

	for range poolSize {
		// 快速响应 Context 取消/超时
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("LLM 请求已取消或超时: %w", err)
		}

		provider, ok := c.providerPool.NextProvider()
		if !ok {
			lastErr = fmt.Errorf("无法从池中获取有效 Provider")
			break
		}

		reqPayload := ChatRequest{
			Model: provider.Model,
			Messages: []ChatMessage{
				{Role: "system", Content: c.buildSystemPrompt(contextHints)},
				{Role: "user", Content: parts},
			},
			Temperature:      c.cfg.Temperature,
			TopP:             c.cfg.TopP,
			FrequencyPenalty: c.cfg.FrequencyPenalty,
			PresencePenalty:  c.cfg.PresencePenalty,
			MaxTokens:        c.cfg.MaxTokens,
			ResponseFormat:   &ResponseFormat{Type: "json_object"},
		}

		reqBytes, err := json.Marshal(reqPayload)
		if err != nil {
			return nil, fmt.Errorf("序列化 LLM 请求体失败: %w", err)
		}

		// 规范化 URL 拼接，防止尾部斜杠导致 double slash
		baseURL := strings.TrimRight(provider.BaseURL, "/")
		apiURL := fmt.Sprintf("%s/chat/completions", baseURL)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(reqBytes))
		if err != nil {
			return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", provider.APIKey))

		// 执行请求
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("调用 [%s] 网络异常: %w", provider.Model, err)
			slog.WarnContext(ctx, "[LLM] 网络请求失败，尝试切换下一个 Provider", "model", provider.Model, "err", err)
			continue
		}

		// 读取响应体并确保安全关闭
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if readErr != nil {
			lastErr = fmt.Errorf("读取 [%s] 响应体失败: %w", provider.Model, readErr)
			continue
		}

		// 非 200 状态码统一处理并打印原始错误信息
		if resp.StatusCode != http.StatusOK {
			slog.WarnContext(ctx, "[LLM] API 返回非 200 状态码，准备容灾切换",
				"status", resp.StatusCode,
				"model", provider.Model,
				"error_body", string(body), // 关键：记录上游返回的具体错误原因
			)
			lastErr = fmt.Errorf("provider [%s] 请求失败 (HTTP %d): %s", provider.Model, resp.StatusCode, string(body))

			// 遇到限流或异常时略作退避，但仍监听 Context
			select {
			case <-time.After(100 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			continue
		}

		// 请求成功
		respBytes = body
		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, fmt.Errorf("所有 LLM 服务商重试后均失败: %w", lastErr)
	}

	slog.DebugContext(ctx, "📥 [LLM] API 响应成功", "raw_resp_len", len(respBytes))

	// 6. 解包 OpenAI 响应
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 LLM 响应 JSON 失败: %w, raw: %s", err, string(respBytes))
	}

	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return nil, fmt.Errorf("LLM API 返回业务错误: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM API 未返回任何有效的 Choice 结果")
	}

	// 7. 提取 AI 生成的文本并清洗 Markdown 杂质
	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	content = cleanMarkdownJSON(content)

	// 8. 映射为领域记账模型
	var batch ledger.BatchTransactions
	if err := json.Unmarshal([]byte(content), &batch); err != nil {
		slog.ErrorContext(ctx, "LLM 返回文本无法解析为 BatchTransactions", "raw_content", content, "err", err)
		return nil, fmt.Errorf("AI 提取的数据无法转为合规账单: %w", err)
	}

	slog.DebugContext(ctx, "[LLM] 账单解析完成",
		"transaction_count", len(batch.Transactions))

	return &batch, nil
}

// cleanMarkdownJSON 辅助函数：清洗大模型偶发返回的 ```json ... ``` 包裹标记
func cleanMarkdownJSON(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")

	// 只要能找到合法的 { 和 }，直接切片截取，过滤掉前后所有的废话和 Markdown 标记！
	if start != -1 && end != -1 && start < end {
		return raw[start : end+1]
	}

	return strings.TrimSpace(raw)
}
