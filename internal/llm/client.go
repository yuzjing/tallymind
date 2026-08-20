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

	// 4. 组装为标准的 Base64 Data URI
	base64Str := base64.StdEncoding.EncodeToString(mediaBytes)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64Str), nil
}

// buildSystemPrompt 动态生成 Prompt，传入当前日期供 AI 参考
func buildSystemPrompt() string {
	today := time.Now().Format("2006-01-02")

	return fmt.Sprintf(`你是一个专业的财务记账 JSON 提取助手。
任务：将用户的自然语言（或多笔消费描述）准确解析为符合规范的 JSON 对象。
今日参考日期为：%s。

【字段提取规则与空值不传递原则】：
1. transactions (数组): 包含解析出的所有交易对象（一句话多笔消费需拆分为多项）。
2. amount (数字, 必填): 交易金额绝对值（必须 > 0）。如果无法识别有效金额，请返回空数组 {"transactions": []}。
3. date (字符串): 交易实际日期 (格式 YYYY-MM-DD)。
   - 若用户提及"昨天/前天/大前天/具体日期"，请结合今日日期(%s)进行精确推算；
   - 若用户【未提及日期】，请直接设为 ""，切勿盲目猜测！
4. payee (字符串): 商户/交易对手 (如"盒马鲜生"、"滴滴打车")。若用户未提及，直接设为 ""。
5. narration (字符串): 详细消费备注 (如"买水果")。若无补充信息，直接设为 ""。
6. category (字符串): 支出/收入分类 (必须带标准前缀，如 Expenses:Food:Groceries, Expenses:Transport:Taxi, Income:Salary)。若无法确定，直接设为 ""！
7. account (字符串): 付款/资产账户 (如 Assets:WeChat, Liabilities:CreditCard)。若用户未提及支付方式，直接设为 ""！
8. type (字符串): 交易类型，可选 "expense" (普通支出，默认), "income" (收入), "refund" (退款)。

【输出格式强制要求】：
必须且只能输出合法的 JSON 对象，不要包含任何 Markdown 代码块标记（如 `+"```json"+`）或多余的解释文字！`, today, today)
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

	slog.Debug("LLM 提取结果", "content", content, "json", batch)

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
