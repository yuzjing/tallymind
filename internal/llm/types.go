// internal/llm/types.go
package llm

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

type Config struct {
	Providers      []Provider
	PromptTemplate string
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
	APIKey           string            `json:"api_key"`
	BaseURL          string            `json:"base_url"`
	Model            string            `json:"model"`
	MaxTokens        int64             `json:"max_tokens,omitempty"`  // 模型独立最大 Token
	Temperature      float64           `json:"temperature,omitempty"` // 模型独立采样温度
	TopP             float64           `json:"top_p,omitempty"`       // 模型独立 TopP
	FrequencyPenalty float64           `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64           `json:"presence_penalty,omitempty"`
	Timeout          time.Duration     `json:"timeout,omitempty"`       // 请求超时时间 (毫秒)
	ExtraHeaders     map[string]string `json:"extra_headers,omitempty"` // 请求头扩展
}

// ProviderPool 并发安全的多提供商轮询池
type ProviderPool struct {
	providers []Provider
	index     atomic.Int32
}

// NewProviderPoolFromConfig 从 Config 中自动构建 ProviderPool (支持逗号分隔的多个 Key)
func NewProviderPool(providers []Provider) (*ProviderPool, error) {

	var valid []Provider
	for _, p := range providers {
		// 清理首尾空格
		p.APIKey = strings.TrimSpace(p.APIKey)
		p.BaseURL = strings.TrimSpace(p.BaseURL)
		p.Model = strings.TrimSpace(p.Model)

		// 过滤无效配置
		if p.APIKey != "" && p.BaseURL != "" {
			valid = append(valid, p)
		}
	}

	if len(valid) == 0 {
		return nil, fmt.Errorf("llm: 没有可用的 Provider 配置")
	}

	return &ProviderPool{
		providers: valid,
	}, nil
}

func (p *ProviderPool) NextProvider() (Provider, bool) {
	if len(p.providers) == 0 {
		return Provider{}, false
	}
	idx := p.index.Add(1) - 1
	target := p.providers[uint32(idx)%uint32(len(p.providers))]
	return target, true
}

func (p *ProviderPool) PoolSize() int {
	return len(p.providers)
}

// IsEmpty 检查 Key 池是否为空
func (p *ProviderPool) IsEmptyKeyPool() bool {
	return len(p.providers) == 0
}
