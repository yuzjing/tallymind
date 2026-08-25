// internal/config/config.go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"cmp"
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
	PublicURL         string `yaml:"public_url"`          // 应用外部公网主域名

	// 1. 通道独立开关
	EnableWeComWSS  bool `yaml:"enable_wecom_wss"`  // 是否开启企微 WSS 长连接服务
	EnableWeComHTTP bool `yaml:"enable_wecom_http"` // 是否开启企微 HTTP 回调与主动推送服务
	EnableHTTPAPI   bool `yaml:"enable_http_api"`   // 是否开启通用 HTTP API 接口

	// 2. 核心 AI 开关
	EnableLLM bool `yaml:"enable_llm"` // 是否开启大模型自然语言解析

	// 3. Git 自动 commit 备份配置
	EnableGitBackup bool   `yaml:"enable_git_backup"` // 是否开启本地账本 Git 自动备份
	GitBackupCron   string `yaml:"git_backup_cron"`   // 备份定时表达式 (如 "0 */6 * * *")

	// 4. 定时报表与通道配置
	EnableReporter bool     `yaml:"enable_reporter"` // 是否开启定时报表推送总开关
	ReportChannels []string `yaml:"report_channels"` // 报表目标渠道切片 (如 ["wecom"])
	AlertChannels  []string `yaml:"alert_channels"`  // 告警目标渠道切片
}

// WeComConfig 企业微信配置
type WeComConfig struct {
	// 1. 企微自建应用 / 主动推送 / HTTP 回调凭证
	CorpID          string `yaml:"corp_id"`
	AgentID         int64  `yaml:"agent_id"`
	Secret          string `yaml:"secret"`
	Token           string `yaml:"token"`
	EncodingAESKey  string `yaml:"encoding_aes_key"`
	SuccessTemplate string `yaml:"success_template"`
	FailureTemplate string `yaml:"failure_template"`

	// 2. 企微 API 模式机器人 (WSS 长连接专有)
	BotID     string `yaml:"bot_id"`
	BotSecret string `yaml:"bot_secret"`
}

// Config 全局顶层配置结构体
type Config struct {
	App    AppConfig     `yaml:"app"`
	Ledger ledger.Config `yaml:"ledger"`
	LLM    llm.Config    `yaml:"llm"`
	WeCom  WeComConfig   `yaml:"wecom"`
}

// Load 直接读取并解析 YAML 配置文件
func Load(configPath ...string) (*Config, error) {
	path := "config.yaml"
	if len(configPath) > 0 && configPath[0] != "" {
		path = configPath[0]
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 [%s] 失败: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析 YAML 配置失败: %w", err)
	}

	// 默认值保底注入 (防止配置文件中某项未填)
	setDefaults(&cfg)

	return &cfg, nil
}

// setDefaults 为未显式配置的字段提供安全默认值
func setDefaults(cfg *Config) {
	cfg.App.Env = cmp.Or(cfg.App.Env, "development")
	cfg.App.LogLevel = cmp.Or(cfg.App.LogLevel, "info")
	cfg.App.Port = cmp.Or(cfg.App.Port, "8080")
	cfg.App.TemplateDir = cmp.Or(cfg.App.TemplateDir, "templates")
	cfg.App.ReceiptSignSecret = cmp.Or(cfg.App.ReceiptSignSecret, "tallymind_default_secret_key")

	cfg.Ledger.FilePath = cmp.Or(cfg.Ledger.FilePath, "data/2026.bean")
	cfg.Ledger.DefaultCurrency = cmp.Or(cfg.Ledger.DefaultCurrency, "CNY")
	cfg.Ledger.DefaultReporter = cmp.Or(cfg.Ledger.DefaultReporter, "User")
	cfg.Ledger.FallbackCategory = cmp.Or(cfg.Ledger.FallbackCategory, "Expenses:Uncategorized")
	cfg.Ledger.FallbackAccount = cmp.Or(cfg.Ledger.FallbackAccount, "Assets:Pending:Unknown")
	cfg.Ledger.FallbackPayee = cmp.Or(cfg.Ledger.FallbackPayee, "日常消费")

	cfg.WeCom.SuccessTemplate = cmp.Or(cfg.WeCom.SuccessTemplate, "wecom/expense_success.yaml")
	cfg.WeCom.FailureTemplate = cmp.Or(cfg.WeCom.FailureTemplate, "wecom/expense_fail.yaml")
}
