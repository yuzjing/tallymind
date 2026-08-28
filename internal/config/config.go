package config

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"tallymind/internal/ledger"
	"tallymind/internal/llm"
)

// AppConfig 基础服务配置
type AppConfig struct {
	Env               string `yaml:"env"`                 // "development" / "production"
	Debug             bool   `yaml:"debug"`               // true / false
	LogLevel          string `yaml:"log_level"`           // "debug" / "info" / "warn" / "error"
	LogDir            string `yaml:"log_dir"`             // 日志输出目录
	Port              string `yaml:"port"`                // 监听端口
	ReceiptSignSecret string `yaml:"receipt_sign_secret"` // 小票签名密钥
	TemplateDir       string `yaml:"template_dir"`        // 模板目录路径
	ReceiptTemplate   string `yaml:"receipt_template"`    // 小票模板路径
	ReportTemplate    string `yaml:"report_template"`     // 报告模板路径
	PublicURL         string `yaml:"public_url"`          // 应用外部公网主域名
	PanelURL          string `yaml:"panel_url"`           // 看板后端容器地址
	PanelPath         string `yaml:"panel_path"`          // 看板后端容器地址路径

	// 功能开关
	EnableWeComWSS  bool   `yaml:"enable_wecom_wss"`
	EnableWeComHTTP bool   `yaml:"enable_wecom_http"`
	EnableHTTPAPI   bool   `yaml:"enable_http_api"`
	EnableLLM       bool   `yaml:"enable_llm"`
	EnableGitBackup bool   `yaml:"enable_git_backup"`
	GitBackupCron   string `yaml:"git_backup_cron"`
	EnableReporter  bool   `yaml:"enable_reporter"`

	ReportChannels []string `yaml:"report_channels"`
	AlertChannels  []string `yaml:"alert_channels"`
}

// WeComConfig 企业微信配置
type WeComConfig struct {
	CorpID          string `yaml:"corp_id"`
	AgentID         int64  `yaml:"agent_id"`
	Secret          string `yaml:"secret"`
	Token           string `yaml:"token"`
	EncodingAESKey  string `yaml:"encoding_aes_key"`
	SuccessTemplate string `yaml:"success_template"`
	FailureTemplate string `yaml:"failure_template"`
	ReportTemplate  string `yaml:"report_template"`
	BotID           string `yaml:"bot_id"`
	BotSecret       string `yaml:"bot_secret"`
}

// LLMProviderConfig 专门用于反序列化 YAML 的 Provider DTO
type LLMProviderConfig struct {
	APIKey           string            `yaml:"api_key"`
	BaseURL          string            `yaml:"base_url"`
	Model            string            `yaml:"model"`
	MaxTokens        int64             `yaml:"max_tokens"`
	Temperature      *float64          `yaml:"temperature"`
	TopP             *float64          `yaml:"top_p"`
	FrequencyPenalty *float64          `yaml:"frequency_penalty"`
	PresencePenalty  *float64          `yaml:"presence_penalty"`
	Timeout          string            `yaml:"timeout"`
	ExtraHeaders     map[string]string `yaml:"extra_headers"`
}

// LLMConfig 专门用于反序列化 YAML 的 LLM DTO
type LLMConfig struct {
	Providers      []LLMProviderConfig `yaml:"providers"`
	PromptTemplate string              `yaml:"prompt_template"`
}

// ToDomain 将 YAML 配置转换为纯净的 llm.Config 领域实体
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

// Config 全局顶层配置结构体
type Config struct {
	App    AppConfig     `yaml:"app"`
	Ledger ledger.Config `yaml:"ledger"`
	LLM    LLMConfig     `yaml:"llm"`
	WeCom  WeComConfig   `yaml:"wecom"`
}

// Load 读取并解析 YAML 配置文件 (支持环境变量替换)
func Load(configPath ...string) (*Config, error) {
	path := "config.yaml"
	if len(configPath) > 0 && configPath[0] != "" {
		path = configPath[0]
	}

	rawBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 [%s] 失败: %w", path, err)
	}

	// ⭐️ 核心增强：自动支持环境变量替换 (如 ${GEMINI_API_KEY})
	expandedYAML := os.ExpandEnv(string(rawBytes))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expandedYAML), &cfg); err != nil {
		return nil, fmt.Errorf("解析 YAML 配置失败: %w", err)
	}

	// 默认值保底注入
	setDefaults(&cfg)

	return &cfg, nil
}

func setDefaults(cfg *Config) {
	cfg.App.Env = cmp.Or(cfg.App.Env, "development")
	cfg.App.LogLevel = cmp.Or(cfg.App.LogLevel, "info")
	cfg.App.Port = cmp.Or(cfg.App.Port, "8080")
	cfg.App.TemplateDir = cmp.Or(cfg.App.TemplateDir, "templates")

	cfg.App.ReceiptTemplate = cmp.Or(cfg.App.ReceiptTemplate, "web/receipt.html")
	cfg.App.ReceiptSignSecret = cmp.Or(cfg.App.ReceiptSignSecret, "tallymind_default_secret_key")
	cfg.App.ReportTemplate = cmp.Or(cfg.App.ReportTemplate, "web/periodic_report.html")

	cfg.App.PanelURL = cmp.Or(cfg.App.PanelURL, "")
	cfg.App.PanelPath = cmp.Or(cfg.App.PanelPath, "")

	cfg.LLM.PromptTemplate = cmp.Or(cfg.LLM.PromptTemplate, "prompt/system_prompt.md")

	cfg.Ledger.FilePath = cmp.Or(cfg.Ledger.FilePath, "data/2026.bean")
	cfg.Ledger.DefaultCurrency = cmp.Or(cfg.Ledger.DefaultCurrency, "CNY")
	cfg.Ledger.DefaultReporter = cmp.Or(cfg.Ledger.DefaultReporter, "User")
	cfg.Ledger.FallbackCategory = cmp.Or(cfg.Ledger.FallbackCategory, "Expenses:Uncategorized")
	cfg.Ledger.FallbackAccount = cmp.Or(cfg.Ledger.FallbackAccount, "Assets:Pending:Unknown")
	cfg.Ledger.FallbackPayee = cmp.Or(cfg.Ledger.FallbackPayee, "日常消费")

	cfg.WeCom.SuccessTemplate = cmp.Or(cfg.WeCom.SuccessTemplate, "wecom/expense_success.yaml")
	cfg.WeCom.FailureTemplate = cmp.Or(cfg.WeCom.FailureTemplate, "wecom/expense_fail.yaml")
	cfg.WeCom.ReportTemplate = cmp.Or(cfg.WeCom.ReportTemplate, "wecom/report.yaml")

}

// derefOr 泛型安全解引用辅助函数：若指针存在则解引用，若为 nil 则返回 fallback 默认值
func derefOr[T any](ptr *T, fallback T) T {
	if ptr != nil {
		return *ptr
	}
	return fallback
}
