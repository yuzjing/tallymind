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
func buildSystemPrompt() string {
	today := time.Now().Format("2006-01-02")
	year := today[:4]

	return fmt.Sprintf(`你是一个专业的全模态财务记账助手。将用户发送的文本、语音、账单小票/发票截图提取为标准 Beancount JSON 数据。今日基准日期：%[1]s。

【核心提取原则】：
1. amount: 实付金额绝对值（必须 > 0，退款/收入亦为正数，无有效交易返回 {"transactions": []}）；currency: 默认 "CNY"，见外币符号精准提取(如 USD, JPY)。
2. date: YYYY-MM-DD，结合今日(%[1]s)推算相对日期，未提及设为 ""。
3. payee: 店铺/商户/机构名称；narration: 商品明细或备注说明；type: "expense"(支出), "income"(收入), "refund"(退款)。
4. category: Beancount 科目，日常以 Expenses: 或 Income: 开头（如 Expenses:Food:Drinks）；期初建账/初始资金注入使用 Equity:Opening-Balances。
5. account: 资金结算账户，以 Assets: 或 Liabilities: 开头（如 Assets:WeChat:Wallet, Liabilities:Alipay:Huabei，银行卡用大写英文缩写如 Liabilities:CreditCard:CMB, Assets:Bank:ICBC，无依据设为 ""）。
6. tags: 字符串数组。提取特征标签（如周期扣费 "#recurring"、待报销 "#reimbursement"、特定场景如 "#medical"、"#renovation" 等），无特征设为 []。
7. metadata (无依据设为 ""):
   - owner: 实际消费归属人 (如 "wife", "husband"，默认 "")。
   - beneficiary: 实际受益人 (如 "baby", "parents", "wife", "family"，自由推断)。
   - invoice_status: 电子发票填 "done"，需开票/待报销填 "pending"。
   - original_amount / discount_amount: 原价与优惠减免金额。
   - time / location / link: 小票具体时间(HH:MM:SS)、分店地点、订单流水号。

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
      "account": "Assets:WeChat:Wallet",
      "type": "expense",
      "tags": [],
      "metadata": {
        "owner": "husband",
        "beneficiary": "",
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

【要求】：只输出合法 JSON 对象，不含任何 Markdown 标记或多余废话。`, today, year)
}

func NewClient(cfg Config) (*Client, error) {
	// 1. 必填配置校验：统一使用 fmt.Errorf 抛出明确错误
	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("LLM 配置错误: providers 列表不能为空")
	}

	return &Client{
		cfg:          cfg,
		providerPool: NewProviderPool(cfg.Providers),
		httpClient: &http.Client{
			Timeout: 20 * time.Second, // 20 秒超时控制
		},
	}, nil
}

func (c *Client) ParseTransaction(ctx context.Context, userText string, attachments ...Attachment) (*ledger.BatchTransactions, error) {
	trimmedText := strings.TrimSpace(userText)
	// 1. 组装多模态
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
			ImageURL: &ImageURL{URL: dataURI}, // 使用 Data URI 转码后的 URL
		})

	}
	// 2. 一次性前置拦截：如果既没有用户文本，也没有任何成功转码的媒体附件，直接退出！

	if trimmedText == "" && len(mediaParts) == 0 {
		return nil, fmt.Errorf("未提供任何有效的账单文本或多模态附件")
	}
	// 3. 动态组装提示词

	promptText := "请仔细分析传入的信息，识别提取出所有的消费记账明细。"

	if strings.TrimSpace(userText) != "" {
		promptText = fmt.Sprintf("请分析传入的信息。用户的补充说明：%s", strings.TrimSpace(userText))
	}

	// 4. 一次性将文本与媒体合并到 part

	parts := make([]ContentPart, 0, len(mediaParts)+1)
	parts = append(parts, ContentPart{Type: "text", Text: promptText})
	parts = append(parts, mediaParts...)
	slog.DebugContext(ctx, "[LLM] 准备发起多模态请求",
		"model", provider.Model,
		"parts_count", len(parts),
		"user_text", userText)

	// 5. 核心容灾重试循环：基于 ProviderPool 动态切换服务商与 Key
	var respBytes []byte
	var lastErr error

	for attempt := 0; attempt < len(c.providerPool.providers); attempt++ {
		provider, ok := c.providerPool.NextAPIKey()
		if !ok {
			break
		}
		//  组装 ChatRequest
		reqPayload := ChatRequest{
			Model: provider.Model,
			Messages: []ChatMessage{
				{Role: "system", Content: buildSystemPrompt()},
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

		// // 看看发给大模型的数据结构长啥样 (截取前 1000 个字符，防止 Base64 刷屏)
		// preview := string(reqBytes)
		// if len(preview) > 3000 {
		// 	preview = preview[:3000] + "...[后面太长已截断]"
		// }
		// slog.WarnContext(ctx, "📦【发包检查】即将发给大模型的 Payload", "preview", preview)

		if err != nil {
			return nil, fmt.Errorf("构造 LLM 请求失败: %w", err)
		}

		// 构造 HTTP POST 请求
		apiURL := fmt.Sprintf("%s/chat/completions", provider.BaseURL)
		req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(reqBytes))
		if err != nil {
			return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", provider.APIKey))

		// 发起http请求
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("调用  [%s] LLM API 网络异常: %w", provider.Model, err)
			continue
		}
		respBytes, err = io.ReadAll(resp.Body)

		resp.Body.Close()
		// 遇到 429 限流或 403 权限/配额问题，自动切换下一个提供商/Key 重试！

		if resp.StatusCode != http.StatusOK {
			slog.WarnContext(ctx, "[LLM] API 返回非 200 状态码，自动切换备用服务商/Key",
				"status", resp.StatusCode,
				"model", provider.Model,
			)
			lastErr = fmt.Errorf("provider [%s] rate limited (HTTP %d)", provider.Model, resp.StatusCode)
			time.Sleep(100 * time.Millisecond)
			continue // 👈 切换下一个重试
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("provider [%s] failed (HTTP %d)", provider.Model, resp.StatusCode)
			continue
		}

		// 成功，清空错误跳出重试
		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, fmt.Errorf("所有大模型服务商均调用失败: %w", lastErr)
	}
	// 看看接口返回的原始未清洗文本到底是什么
	slog.DebugContext(ctx, "📥【原始响应】API 返回的完整原始文本", "raw_resp", string(respBytes))

	// 9. 解包 OpenAI 响应
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 LLM 响应 JSON 失败: %w", err)
	}

	if chatResp.Error != nil && chatResp.Error.Message != "" {
		return nil, fmt.Errorf("LLM API 报错: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM API 未返回任何有效的 Choice 结果")
	}

	// 10. 提取 AI 吐出的 JSON 文本并清洗可能的 Markdown 杂质
	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	content = cleanMarkdownJSON(content)

	// 11. 第二次反序列化：转换为 Go 的 ledger.BatchTransactions 结构体
	var batch ledger.BatchTransactions
	if err := json.Unmarshal([]byte(content), &batch); err != nil {
		slog.Error("LLM 返回文本无法解析为 BatchTransactions", "raw_content", content, "err", err)
		return nil, fmt.Errorf("AI 提取的数据无法转为合规账单: %w", err)
	}

	slog.DebugContext(ctx, "[LLM] 大模型提取完成",
		"raw_json", content,
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
