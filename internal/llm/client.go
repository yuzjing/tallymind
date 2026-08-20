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

type Config struct {
	APIKey           string
	BaseURL          string
	Model            string
	MaxTokens        int64
	Temperature      float64
	TopP             float64
	FrequencyPenalty float64
	PresencePenalty  float64
}

// Attachment 通用媒体附件载体 (支持图片、文档、音频等)
type Attachment struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	MimeType string `json:"mime_type,omitempty"`
}

// openai 请求/响应标准体(多模态归一化)
type ContentPart struct {
	Type       string        `json:"type"`                  // "text" 或 "image_url"
	Text       string        `json:"text,omitempty"`        // 当 type="text"
	ImageURL   *ImageURL     `json:"image_url,omitempty"`   // 当 type="image_url"
	InputAudio *AudioContent `json:"input_audio,omitempty"` // 语音记账 (预留)
	File       *MediaURL     `json:"file,omitempty"`        // PDF 电子发票/文档 (预留)
}

// 通用媒体链接/DataURI载体 (图片、PDF 文件共用)
type MediaURL struct {
	URL string `json:"url"`
}

// 语音/音频消息载体 (兼容 OpenAI Audio 协议)
type AudioContent struct {
	Data   string `json:"data"`   // Base64 编码的音频原始数据
	Format string `json:"format"` // 音频格式: "mp3", "wav", "amr"
}

type ImageURL struct {
	URL string `json:"url"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // 支持 string 或 []ContentPart
}

type ChatRequest struct {
	Model            string          `json:"model"`
	Messages         []ChatMessage   `json:"messages"`
	Temperature      float64         `json:"temperature,omitempty"`
	TopP             float64         `json:"top_p,omitempty"`
	FrequencyPenalty float64         `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64         `json:"presence_penalty,omitempty"`
	MaxTokens        int64           `json:"max_tokens,omitempty"`
	ResponseFormat   *ResponseFormat `json:"response_format,omitempty"`
}

type ResponseFormat struct {
	Type string `json:"type"`
}

type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"message"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
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
	year := today[:4] // 当前年份，如 2026

	return fmt.Sprintf(`你是一个专业的财务记账与多模态账单提取专家。
任务：无论用户发送的是自然语言描述，还是【单笔小票/账单流水列表/支付截图/发票】，请提取图片或文字中可见的所有交易记录，输出标准 JSON。
今日参考基准日期为：%[1]s。

【核心提取指南】：
1. 账单流水列表扫描：
   - 仔细识别图片中的所有商户名、日期时间与金额。
   - 若图片包含多条流水记录（如微信/支付宝/京东账单），请将可见的每一笔记录都独立提取为一个交易对象加入 transactions 数组。
   - 符号处理："-10.00" 或 "支出 10.00" 代表支出 (amount: 10.0, type: "expense")；"+19.90" 或 "退款" 代表退款/收入 (amount: 19.9, type: "refund" 或 "income")。amount 必须为正数绝对值。
2. 字段规范：
   - date: 格式 YYYY-MM-DD。若图片显示"08-17"，结合基准年份推算为 "%[2]s-08-17"；若无日期则设为 ""。
   - payee: 商户/收款方（如"奈雪的茶"、"京东"、"盒马"）。
   - narration: 消费明细/商品备注（如"茶饮"、"数码配件"）。
   - category: 标准层级科目（如 Expenses:Food:Drinks, Expenses:Shopping:Online, Expenses:Food:Dining, Expenses:Transport:Taxi）。
   - account: 付款账户（如 Assets:WeChat, Assets:Alipay），未知填 ""。

【输出 JSON 示例】：
{
  "transactions": [
    {
      "amount": 10.00,
      "date": "%[1]s",
      "payee": "奈雪的茶",
      "narration": "茶饮消费",
      "category": "Expenses:Food:Drinks",
      "account": "Assets:WeChat",
      "type": "expense"
    },
    {
      "amount": 19.90,
      "date": "%[1]s",
      "payee": "京东商城平台商户",
      "narration": "退款",
      "category": "Income:Refund",
      "account": "Assets:WeChat",
      "type": "refund"
    }
  ]
}

【要求】：只输出合法的 JSON 对象，不包含任何 markdown 代码块标记或额外废话。`, today, year)
}

type Client struct {
	cfg        Config
	httpClient *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	// 1. 必填配置校验：统一使用 fmt.Errorf 抛出明确错误
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("LLM 配置错误: LLM_API_KEY 不能为空，请检查 .env 文件")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("LLM 配置错误: LLM_BASE_URL 不能为空，请检查 .env 文件")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("LLM 配置错误: LLM_MODEL 不能为空，请检查 .env 文件")
	}

	return &Client{
		cfg: cfg,
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
	parts = append(parts, parts...)
	slog.DebugContext(ctx, "[LLM] 准备发起多模态请求",
		"model", c.cfg.Model,
		"parts_count", len(parts),
		"user_text", userText)

	// 5. 组装 ChatRequest
	reqPayload := ChatRequest{
		Model: c.cfg.Model,
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

	// 看看发给大模型的数据结构长啥样 (截取前 500 个字符，防止 Base64 刷屏)
	preview := string(reqBytes)
	if len(preview) > 100 {
		preview = preview[:1000] + "...[后面太长已截断]"
	}
	slog.WarnContext(ctx, "📦【发包检查】即将发给大模型的 Payload", "preview", preview)

	if err != nil {
		return nil, fmt.Errorf("构造 LLM 请求失败: %w", err)
	}

	// 6. 构造 HTTP POST 请求
	apiURL := fmt.Sprintf("%s/chat/completions", c.cfg.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.cfg.APIKey))

	// 7. 发起http请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 LLM API 网络异常: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)

	// 看看接口返回的原始未清洗文本到底是什么
	slog.WarnContext(ctx, "📥【原始响应】谷歌 API 返回的完整原始文本", "raw_resp", string(respBytes))

	if err != nil {
		return nil, fmt.Errorf("读取 LLM 响应失败: %w", err)
	}

	// 8. 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		slog.Error("LLM API 返回异常", "status", resp.StatusCode, "body", string(respBytes))
		return nil, fmt.Errorf("LLM API 调用失败 (HTTP %d)", resp.StatusCode)
	}

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
