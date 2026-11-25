// internal/llm/client.go
package llm

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"tallymind/internal/ledger"
	"text/template"
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

	// 2. 如果是远程 HTTP/HTTPS 链接 -> 下载到内存 (独立 20s 下载超时，不依赖全局 Client.Timeout)
	if strings.HasPrefix(mediaURL, "http://") || strings.HasPrefix(mediaURL, "https://") {
		dlCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, mediaURL, nil)
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

	var extraHints string
	if strings.TrimSpace(contextHints) != "" {
		extraHints = "\n" + strings.TrimSpace(contextHints) + "\n"
	}

	// ⭐️ 动态从配置路径读取 Markdown 提示词文件 (0 硬编码，0 秒热更新)
	if c.cfg.PromptTemplate != "" {
		content, err := os.ReadFile(c.cfg.PromptTemplate)
		if err == nil {
			tmpl, err := template.New("system_prompt").Parse(string(content))
			if err == nil {
				var buf bytes.Buffer
				if err := tmpl.Execute(&buf, struct {
					Today        string
					ContextHints string
				}{
					Today:        today,
					ContextHints: extraHints,
				}); err == nil {
					return buf.String()
				}
			}
		} else {
			slog.Warn("读取外挂提示词模板失败，使用内置保底提示词", "path", c.cfg.PromptTemplate, "err", err)
		}
	}

	return c.defaultFallbackPrompt(today, extraHints)
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
			// 不设置全局 Client.Timeout，避免静默截断大于 20s 的长文本推理；总超时交由单次 reqCtx 掌控
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment, // 保留 HTTP_PROXY / NO_PROXY 分流能力
				DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
				TLSHandshakeTimeout: 10 * time.Second,
				IdleConnTimeout:     90 * time.Second,
			},
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

	// 在重试循环外构建一次 System Prompt（仍保持每次请求热读取最新模板，但不随故障转移重复读盘）
	systemPrompt := c.buildSystemPrompt(contextHints)

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

		// ⭐️ 使用局部闭包：单次请求独立上下文，完美契合 defer cancel() 与 defer resp.Body.Close()
		body, err := func() ([]byte, error) {
			timeout := cmp.Or(provider.Timeout, 60*time.Second)
			reqCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			reqPayload := ChatRequest{
				Model: provider.Model,
				Messages: []ChatMessage{
					{Role: "system", Content: systemPrompt},
					{Role: "user", Content: parts},
				},
				Temperature:      provider.Temperature,
				TopP:             provider.TopP,
				FrequencyPenalty: provider.FrequencyPenalty,
				PresencePenalty:  provider.PresencePenalty,
				MaxTokens:        provider.MaxTokens,
				ReasoningEffort:  provider.ReasoningEffort,
				ResponseFormat:   &ResponseFormat{Type: "json_object"},
			}

			reqBytes, err := json.Marshal(reqPayload)
			if err != nil {
				return nil, fmt.Errorf("序列化 LLM 请求体失败: %w", err)
			}

			baseURL := strings.TrimRight(provider.BaseURL, "/")
			apiURL := fmt.Sprintf("%s/chat/completions", baseURL)

			req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, apiURL, bytes.NewBuffer(reqBytes))
			if err != nil {
				return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
			}

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", provider.APIKey))

			for k, v := range provider.ExtraHeaders {
				req.Header.Set(k, v)
			}

			resp, err := c.httpClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("调用 [%s] 网络异常: %w", provider.Model, err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("读取 [%s] 响应体失败: %w", provider.Model, err)
			}

			if resp.StatusCode != http.StatusOK {
				slog.WarnContext(ctx, "[LLM] API 返回非 200 状态码，准备容灾切换",
					"status", resp.StatusCode,
					"model", provider.Model,
					"error_body", string(body),
				)
				return nil, fmt.Errorf("provider [%s] 请求失败 (HTTP %d): %s", provider.Model, resp.StatusCode, string(body))
			}

			return body, nil
		}()

		if err != nil {
			lastErr = err
			slog.WarnContext(ctx, "[LLM] Provider 调用失败，准备切换下一个", "model", provider.Model, "err", err)

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

	if chatResp.Usage != nil {
		thinkingTokens := max(0, chatResp.Usage.TotalTokens-chatResp.Usage.PromptTokens-chatResp.Usage.CompletionTokens)
		slog.DebugContext(ctx, "[LLM] Token 消耗统计",
			"prompt_tokens", chatResp.Usage.PromptTokens,
			"completion_tokens", chatResp.Usage.CompletionTokens,
			"thinking_tokens", thinkingTokens,
			"total_tokens", chatResp.Usage.TotalTokens,
		)
	}

	// 7. 校验结束原因并提取文本
	choice := chatResp.Choices[0]
	if choice.FinishReason == "length" {
		slog.ErrorContext(ctx, "[LLM] 输出因达到 max_tokens 上限被截断",
			"finish_reason", choice.FinishReason,
			"content_len", len(choice.Message.Content),
		)
		return nil, fmt.Errorf("账单内容过长，AI 输出达到 max_tokens 上限被截断（请拆分发送或调大 max_tokens）")
	}

	content := cleanMarkdownJSON(strings.TrimSpace(choice.Message.Content))

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

func (c *Client) defaultFallbackPrompt(today, extraHints string) string {
	return fmt.Sprintf(`你是一个专业的全模态财务记账助手。将用户发送的文本、语音、账单小票/发票截图提取为标准 Beancount JSON 数据。今日基准日期：%[1]s。%[2]s
【核心原则】：
1. amount: 实付金额绝对值(>0)，currency: 默认 "CNY"。
2. category: 提取为标准二级Beancount支出科目 (如 Expenses:Food:Breakfast)。
3. account: 结算账户(必须见明确凭证提取，严禁臆测，无凭据设为 "")。
4. metadata: owner(出资人), beneficiary(受益人)优先归一化为映射表Key，日常自用设为 ""。
【要求】：只输出合法 JSON 对象 {"transactions": [...]}，不含多余废话。`, today, extraHints)
}
