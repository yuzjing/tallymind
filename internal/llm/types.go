// internal/llm/types.go
package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
)

type Config struct {
	Providers        []Provider
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

// Provider 单个提供商实体 (支持同平台多Key，也支持跨平台)
type Provider struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

// ProviderPool 并发安全的多提供商轮询池
type ProviderPool struct {
	providers []Provider
	index     atomic.Int32
}

// NewProviderPoolFromConfig 从 Config 中自动构建 ProviderPool (支持逗号分隔的多个 Key)
func NewProviderPool(rawJSON string) (*ProviderPool, error) {

	if strings.TrimSpace(rawJSON) == "" {
		return nil, fmt.Errorf("LLM 配置错误: LLM_PROVIDERS 不能为空")
	}
	var providers []Provider
	if err := json.Unmarshal([]byte(rawJSON), &providers); err != nil {
		return nil, fmt.Errorf("解析 LLM_PROVIDERS JSON 失败: %w", err)
	}

	for _, k := range rawKeys {
		if k = strings.TrimSpace(k); k != "" {
			providers = append(providers, Provider{
				APIKey:  k,
				BaseURL: cfg.BaseURL,
				Model:   cfg.Model,
			})
		}
	}
	return &ProviderPool{providers: providers}
}

func (p *ProviderPool) NextAPIKey() (Provider, bool) {
	if len(p.providers) == 0 {
		return Provider{}, false
	}
	idx := p.index.Add(1)
	return p.providers[int(idx)%len(p.providers)], true
}

func (p *ProviderPool) PoolSize() int {
	return len(p.providers)
}

// IsEmpty 检查 Key 池是否为空
func (p *ProviderPool) IsEmptyKeyPool() bool {
	return len(p.providers) == 0
}
