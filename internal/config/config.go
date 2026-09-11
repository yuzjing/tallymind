package config

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"

	"tallymind/internal/ledger"
	"tallymind/internal/llm"
)

type Config struct {
	App    AppConfig     `toml:"app"`
	Log    LogConfig     `toml:"log"`
	Ledger ledger.Config `toml:"ledger"`
	LLM    LLMConfig     `toml:"llm"`
	WeCom  WeComConfig   `toml:"wecom"`
}

// AppConfig 基础服务配置
type AppConfig struct {
	Env               string `toml:"env"`                 // "development" / "production"
	Debug             bool   `toml:"debug"`               // true / false
	Port              int    `toml:"port"`                // 监听端口
	ReceiptSignSecret string `toml:"receipt_sign_secret"` // 小票签名密钥
	TemplateDir       string `toml:"template_dir"`        // 模板目录路径
	ReceiptTemplate   string `toml:"receipt_template"`    // 小票模板路径
	ReportTemplate    string `toml:"report_template"`     // 报告模板路径
	PublicURL         string `toml:"public_url"`          // 应用外部公网主域名
	PanelURL          string `toml:"panel_url"`           // 看板后端容器地址
	PanelPath         string `toml:"panel_path"`          // 看板后端容器地址路径

	// 功能开关
	EnableWeComWSS  bool `toml:"enable_wecom_wss"`
	EnableWeComHTTP bool `toml:"enable_wecom_http"`
	EnableHTTPAPI   bool `toml:"enable_http_api"`
	EnableLLM       bool `toml:"enable_llm"`
	EnableReporter  bool `toml:"enable_reporter"`

	ReportChannels []string `toml:"report_channels"`
	AlertChannels  []string `toml:"alert_channels"`
}

type LogConfig struct {
	Level    string        `toml:"level"`
	ToStdout bool          `toml:"to_stdout"`
	File     FileLogConfig `toml:"file"`
}

type FileLogConfig struct {
	Enabled    bool   `toml:"enabled"`
	Dir        string `toml:"dir"`
	MaxSize    int    `toml:"max_size"`
	MaxBackups int    `toml:"max_backups"`
	MaxAge     int    `toml:"max_age"`
	Compress   bool   `toml:"compress"`
}

// WeComConfig 企业微信配置
type WeComConfig struct {
	CorpID          string `toml:"corp_id"`
	AgentID         int64  `toml:"agent_id"`
	Secret          string `toml:"secret"`
	Token           string `toml:"token"`
	EncodingAESKey  string `toml:"encoding_aes_key"`
	SuccessTemplate string `toml:"success_template"`
	FailureTemplate string `toml:"failure_template"`
	ReportTemplate  string `toml:"report_template"`
	BotID           string `toml:"bot_id"`
	BotSecret       string `toml:"bot_secret"`
}

// LLMProviderConfig 专门用于反序列化 toml 的 Provider DTO
type LLMProviderConfig struct {
	APIKey           string            `toml:"api_key"`
	BaseURL          string            `toml:"base_url"`
	Model            string            `toml:"model"`
	MaxTokens        int64             `toml:"max_tokens"`
	Temperature      *float64          `toml:"temperature"`
	TopP             *float64          `toml:"top_p"`
	FrequencyPenalty *float64          `toml:"frequency_penalty"`
	PresencePenalty  *float64          `toml:"presence_penalty"`
	Timeout          string            `toml:"timeout"`
	ExtraHeaders     map[string]string `toml:"extra_headers"`
}

// LLMConfig 专门用于反序列化 toml 的 LLM DTO
type LLMConfig struct {
	Providers      []LLMProviderConfig `toml:"providers"`
	PromptTemplate string              `toml:"prompt_template"`
}

// ToDomain 将 toml 配置转换为纯净的 llm.Config 领域实体
func (c *LLMConfig) ToDomain(templateDir string) llm.Config {
	fullPromptPath := filepath.Join(templateDir, c.PromptTemplate)
	providers := make([]llm.Provider, len(c.Providers))
	for i, p := range c.Providers {
		// 解析超时时间 (默认 30 秒)
		timeoutDur, err := time.ParseDuration(p.Timeout)
		if err != nil || timeoutDur <= 0 {
			timeoutDur = 30 * time.Second
		}

		providers[i] = llm.Provider{
			APIKey:           p.APIKey,
			BaseURL:          p.BaseURL,
			Model:            p.Model,
			MaxTokens:        cmp.Or(p.MaxTokens, int64(4096)), // 默认 4096
			Temperature:      derefOr(p.Temperature, 0.2),      // 默认 0.2
			TopP:             derefOr(p.TopP, 0.0),
			FrequencyPenalty: derefOr(p.FrequencyPenalty, 0.0),
			PresencePenalty:  derefOr(p.PresencePenalty, 0.0),
			Timeout:          timeoutDur,
			ExtraHeaders:     p.ExtraHeaders,
		}
	}

	return llm.Config{
		Providers:      providers,
		PromptTemplate: fullPromptPath,
	}
}

// Load 读取并解析 toml 配置文件 (支持环境变量替换)
func Load(configPath ...string) (*Config, error) {
	path := "config.toml"
	if len(configPath) > 0 && configPath[0] != "" {
		path = configPath[0]
	}

	rawBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 [%s] 失败: %w", path, err)
	}

	// ⭐️ 核心增强：自动支持环境变量替换 (如 ${GEMINI_API_KEY})
	expandedtoml := os.ExpandEnv(string(rawBytes))

	var cfg Config
	if err := toml.Unmarshal([]byte(expandedtoml), &cfg); err != nil {
		return nil, fmt.Errorf("解析 toml 配置失败: %w", err)
	}

	// 默认值保底注入
	setDefaults(&cfg)

	return &cfg, nil
}

func setDefaults(cfg *Config) {
	cfg.App.Env = cmp.Or(cfg.App.Env, "development")
	cfg.App.Port = cmp.Or(cfg.App.Port, 8080)
	cfg.App.TemplateDir = cmp.Or(cfg.App.TemplateDir, "templates")

	cfg.App.ReceiptTemplate = cmp.Or(cfg.App.ReceiptTemplate, "web/receipt.html")
	cfg.App.ReceiptSignSecret = cmp.Or(cfg.App.ReceiptSignSecret, "tallymind_default_secret_key")
	cfg.App.ReportTemplate = cmp.Or(cfg.App.ReportTemplate, "web/periodic_report.html")

	cfg.App.PanelURL = cmp.Or(cfg.App.PanelURL, "")
	cfg.App.PanelPath = cmp.Or(cfg.App.PanelPath, "")

	// Log 默认值注入
	cfg.Log.Level = cmp.Or(cfg.Log.Level, "info")
	cfg.Log.File.Dir = cmp.Or(cfg.Log.File.Dir, "./logs")
	cfg.Log.File.MaxSize = cmp.Or(cfg.Log.File.MaxSize, 10)
	cfg.Log.File.MaxBackups = cmp.Or(cfg.Log.File.MaxBackups, 3)
	cfg.Log.File.MaxAge = cmp.Or(cfg.Log.File.MaxAge, 14)

	cfg.LLM.PromptTemplate = cmp.Or(cfg.LLM.PromptTemplate, "prompt/system_prompt.md")

	cfg.Ledger.DataDir = cmp.Or(cfg.Ledger.DataDir, "data")
	cfg.Ledger.DefaultCurrency = cmp.Or(cfg.Ledger.DefaultCurrency, "CNY")
	cfg.Ledger.DefaultReporter = cmp.Or(cfg.Ledger.DefaultReporter, "User")
	cfg.Ledger.FallbackCategory = cmp.Or(cfg.Ledger.FallbackCategory, "Expenses:Uncategorized")
	cfg.Ledger.FallbackAccount = cmp.Or(cfg.Ledger.FallbackAccount, "Assets:Pending:Unknown")
	cfg.Ledger.FallbackPayee = cmp.Or(cfg.Ledger.FallbackPayee, "日常消费")

	cfg.WeCom.SuccessTemplate = cmp.Or(cfg.WeCom.SuccessTemplate, "wecom/expense_success.toml")
	cfg.WeCom.FailureTemplate = cmp.Or(cfg.WeCom.FailureTemplate, "wecom/expense_fail.toml")
	cfg.WeCom.ReportTemplate = cmp.Or(cfg.WeCom.ReportTemplate, "wecom/report.toml")

}

// derefOr 泛型安全解引用辅助函数：若指针存在则解引用，若为 nil 则返回 fallback 默认值
func derefOr[T any](ptr *T, fallback T) T {
	if ptr != nil {
		return *ptr
	}
	return fallback
}
