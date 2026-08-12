// internal/config/config.go
package config

import (
	"cmp"
	"os"
	"strconv"

	"tallymind/internal/llm" // 直接组合 llm 包的 Config
)

// AppConfig 基础服务配置
type AppConfig struct {
	Env      string // "development" / "production"
	Debug    bool   // true / false
	LogLevel string // "debug" / "info" / "warn" / "error"
	RunMode  string // "websocket" / "http" / "both"
	Port     string // 监听端口
}

// LedgerConfig 账本配置
type LedgerConfig struct {
	FilePath         string
	DefaultCurrency  string
	DefaultReporter  string
	FallbackCategory string
	FallbackAccount  string
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
	return &Config{
		App: AppConfig{
			Env:      cmp.Or(os.Getenv("APP_ENV"), "development"),
			Debug:    getEnvBool("DEBUG", false),
			LogLevel: cmp.Or(os.Getenv("LOG_LEVEL"), "info"),
			RunMode:  cmp.Or(os.Getenv("RUN_MODE"), "websocket"),
			Port:     cmp.Or(os.Getenv("SERVER_PORT"), "8080"),
		},
		Ledger: LedgerConfig{
			FilePath:         os.Getenv("BEANCOUNT_FILE_PATH"),
			DefaultCurrency:  cmp.Or(os.Getenv("DEFAULT_CURRENCY"), "CNY"),
			DefaultReporter:  cmp.Or(os.Getenv("DEFAULT_REPORTER"), "Unknown"),
			FallbackCategory: cmp.Or(os.Getenv("FALLBACK_CATEGORY"), "Expenses:Uncategorized"),
			FallbackAccount:  cmp.Or(os.Getenv("FALLBACK_ACCOUNT"), "Assets:Pending:Unknown"),
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
