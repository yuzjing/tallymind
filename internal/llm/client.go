// internal/llm/client.go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"tallymind/internal/ledger"
	"time"

	"strings"
)

type Config struct {
	APIKey           string
	BaseURL          string
	Model            string
	MaxTokens        int
	Temperature      float64
	TopP             float64
	FrequencyPenalty float64
	PresencePenalty  float64
}

// openai 请求标准体
type ChatCompletionRequest struct {
	Model            string          `json:"model"`
	Messages         []ChatMessage   `json:"messages"`
	Temperature      float64         `json:"temperature,omitempty"`
	TopP             float64         `json:"top_p,omitempty"`
	FrequencyPenalty float64         `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64         `json:"presence_penalty,omitempty"`
	MaxTokens        int             `json:"max_tokens,omitempty"`
	ResponseFormat   *ResponseFormat `json:"response_format,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ResponseFormat struct {
	Type string `json:"type"`
}

type ChatCompletionResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
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
		httpClient: &http.Client{
			Timeout: 20 * time.Second, // 20 秒超时控制
		},
	}, nil
}

func (c *Client) ParseTransaction(ctx context.Context, userText string) (*ledger.BatchTransactions, error) {
	if strings.TrimSpace(userText) == "" {
		return nil, fmt.Errorf("输入文本不能为空")
	}

	// 1. 组装请求 Payload (全参数自动打包)
	reqPayload := ChatCompletionRequest{
		Model:            c.cfg.Model,
		Messages:         []ChatMessage{{Role: "system", Content: buildSystemPrompt()}, {Role: "user", Content: userText}},
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

	// 2. 构造 HTTP POST 请求
	apiURL := fmt.Sprintf("%s/chat/completions", c.cfg.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.cfg.APIKey))

	// 3. 发起网络请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 LLM API 网络异常: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 LLM 响应失败: %w", err)
	}

	// 4. 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		log.Printf("[ERROR] LLM API 返回异常 (HTTP %d): %s\n", resp.StatusCode, string(respBytes))
		return nil, fmt.Errorf("LLM API 调用失败 (HTTP %d)", resp.StatusCode)
	}

	// 5. 解包 OpenAI 响应
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

	// 6. 提取 AI 吐出的 JSON 文本并清洗可能的 Markdown 杂质
	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	content = cleanMarkdownJSON(content)

	// 7. 第二次反序列化：转换为 Go 的 ledger.BatchTransactions 结构体
	var batch ledger.BatchTransactions
	if err := json.Unmarshal([]byte(content), &batch); err != nil {
		log.Printf("[ERROR] LLM 返回文本无法解析为 BatchTransactions | 原始文本: %s | 错误: %v\n", content, err)
		return nil, fmt.Errorf("AI 提取的数据无法转为合规账单: %w", err)
	}

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
