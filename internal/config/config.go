// internal/config/config.go
package config

import (
	"cmp"
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"tallymind/internal/llm" // 直接组合 llm 包的 Config
)

// AppConfig 基础服务配置
type AppConfig struct {
	Env      string // "development" / "production"
	Debug    bool   // true / false
	LogLevel string // "debug" / "info" / "warn" / "error"
	Port     string // 监听端口

	// 1. 通道独立开关
	EnableWeComWSS bool // 是否开启企微 WSS 长连接服务
	EnableHTTPAPI  bool // 是否开启通用 HTTP API 接口

	// 2. 核心 AI 开关
	EnableLLM bool // 是否开启 AI 大模型自然语言解析

	// 3. Git 自动 commit 备份配置
	EnableGitBackup bool   // 是否开启本地账本 Git 自动备份
	GitBackupCron   string // 备份定时表达式，默认每 6 小时 "0 */6 * * *"

	// 4. 定时报表与任务配置
	EnableReporter    bool   // 是否开启定时报表推送总开关
	WeeklyReportCron  string // 周报表达式，默认每周日 20:00 "0 20 * * 0"
	MonthlyReportCron string // 月报表达式，默认每月1日 09:00 "0 9 1 * *"
}

// LedgerConfig 账本配置
type LedgerConfig struct {
	FilePath         string
	DefaultCurrency  string
	DefaultReporter  string
	FallbackCategory string
	FallbackAccount  string
	FallbackPayee    string
}

// WeComConfig 企业微信配置
type WeComConfig struct {
	BotID     string
	BotSecret string
}

// Config 全局顶层配置结构体 (模块化子结构体设计)
type Config struct {
	App    AppConfig
	Ledger LedgerConfig
	LLM    llm.Config
	WeCom  WeComConfig
}

// Load 从环境变量中加载全部配置
func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		App: AppConfig{
			Env:      cmp.Or(os.Getenv("APP_ENV"), "development"),
			Debug:    getEnvBool("DEBUG", false),
			LogLevel: cmp.Or(os.Getenv("LOG_LEVEL"), "info"),
			Port:     cmp.Or(os.Getenv("SERVER_PORT"), "8080"),

			// 通道独立开关
			EnableWeComWSS: getEnvBool("ENABLE_WECOM_WSS", false), // 默认开启 WSS 长连接
			EnableHTTPAPI:  getEnvBool("ENABLE_HTTP_API", true),   // 默认开启 HTTP API

			// AI 开关
			EnableLLM: getEnvBool("ENABLE_LLM", false),

			// Git 自动备份
			EnableGitBackup: getEnvBool("ENABLE_GIT_BACKUP", true),
			GitBackupCron:   cmp.Or(os.Getenv("GIT_BACKUP_CRON"), "0 */6 * * *"),

			// 定时报表开关与 Cron
			EnableReporter:    getEnvBool("ENABLE_REPORTER", false),
			WeeklyReportCron:  cmp.Or(os.Getenv("WEEKLY_REPORT_CRON"), "0 20 * * 0"),
			MonthlyReportCron: cmp.Or(os.Getenv("MONTHLY_REPORT_CRON"), "0 9 1 * *"),
		},
		Ledger: LedgerConfig{
			FilePath:         os.Getenv("BEANCOUNT_FILE_PATH"),
			DefaultCurrency:  cmp.Or(os.Getenv("DEFAULT_CURRENCY"), "CNY"),
			DefaultReporter:  cmp.Or(os.Getenv("DEFAULT_REPORTER"), "Unknown"),
			FallbackCategory: cmp.Or(os.Getenv("FALLBACK_CATEGORY"), "Expenses:Uncategorized"),
			FallbackAccount:  cmp.Or(os.Getenv("FALLBACK_ACCOUNT"), "Assets:Pending:Unknown"),
			FallbackPayee:    cmp.Or(os.Getenv("FALLBACK_PAYEE"), "日常消费"),
		},
		LLM: llm.Config{
			APIKey:           os.Getenv("LLM_API_KEY"),
			BaseURL:          os.Getenv("LLM_BASE_URL"),
			Model:            os.Getenv("LLM_MODEL"),
			MaxTokens:        getEnvInt("LLM_MAX_TOKENS", 4096),
			Temperature:      getEnvFloat("LLM_TEMPERATURE", 0.2),
			TopP:             getEnvFloat("LLM_TOP_P", 0.0),
			FrequencyPenalty: getEnvFloat("LLM_FREQUENCY_PENALTY", 0.0),
			PresencePenalty:  getEnvFloat("LLM_PRESENCE_PENALTY", 0.0),
		},
		WeCom: WeComConfig{
			BotID:     os.Getenv("WECOM_BOT_ID"),
			BotSecret: os.Getenv("WECOM_BOT_SECRET"),
		},
	}
}

// 辅助函数 1：读取 bool 类型环境变量
func getEnvBool(key string, defaultVal bool) bool {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		return defaultVal
	}
	return val
}

// 辅助函数 2：读取 int 类型环境变量
func getEnvInt(key string, defaultVal int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultVal
	}
	return val
}

// 辅助函数 3：读取 float64 类型环境变量
func getEnvFloat(key string, defaultVal float64) float64 {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return defaultVal
	}
	return val
}
