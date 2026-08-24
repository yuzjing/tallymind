// internal/config/config.go
package config

import (
	"cmp"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"tallymind/internal/ledger"
	"tallymind/internal/llm" // 直接组合 llm 包的 Config
)

// AppConfig 基础服务配置
type AppConfig struct {
	Env      string // "development" / "production"
	Debug    bool   // true / false
	LogLevel string // "debug" / "info" / "warn" / "error"
	LogDir   string // 日志文件路径
	Port     string // 监听端口

	// 1. 通道独立开关
	EnableWeComWSS  bool // 是否开启企微 WSS 长连接服务
	EnableWeComHTTP bool // 是否开启企微自建应用 HTTP 回调与主动推送服务
	EnableHTTPAPI   bool // 是否开启通用 HTTP API 接口

	// 2. 核心 AI 开关
	EnableLLM bool // 是否开启 AI 大模型自然语言解析

	// 3. Git 自动 commit 备份配置
	EnableGitBackup bool   // 是否开启本地账本 Git 自动备份
	GitBackupCron   string // 备份定时表达式，默认每 6 小时 "0 */6 * * *"

	// 4. 定时报表与任务配置
	EnableReporter bool // 是否开启定时报表推送总开关
	ReportChannels []string
	AlertChannels  []string

	//5. 消息模板
	TemplateDir string // 模板目录路径
	PublicURL   string // 应用基础 URL
}

// WeComConfig 企业微信配置
type WeComConfig struct {
	// 1. 企微自建应用 / 主动推送 / HTTP 回调凭证
	CorpID         string // 企业 ID (corpid)
	AgentID        int64  // 自建应用 ID (agentid)
	Secret         string // 自建应用 Secret
	Token          string // 消息回调 Token
	EncodingAESKey string // 消息加解密 EncodingAESKey

	// 2. 企微 API 模式机器人 (WSS 长连接专有)
	BotID           string
	BotSecret       string
	SuccessTemplate string
	FailureTemplate string
}

// Config 全局顶层配置结构体 (模块化子结构体设计)
type Config struct {
	App    AppConfig
	Ledger ledger.Config
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
			LogDir:   cmp.Or(os.Getenv("LOG_DIR"), ""), // 默认日志目录
			Port:     cmp.Or(os.Getenv("SERVER_PORT"), "8080"),

			// 通道独立开关
			EnableWeComWSS:  getEnvBool("ENABLE_WECOM_WSS", false), // 默认开启 WSS 长连接
			EnableWeComHTTP: getEnvBool("ENABLE_WECOM_HTTP", true),
			EnableHTTPAPI:   getEnvBool("ENABLE_HTTP_API", true), // 默认开启 HTTP API

			// AI 开关
			EnableLLM: getEnvBool("ENABLE_LLM", false),

			// Git 自动备份
			EnableGitBackup: getEnvBool("ENABLE_GIT_BACKUP", true),
			GitBackupCron:   cmp.Or(os.Getenv("GIT_BACKUP_CRON"), "0 */6 * * *"),

			// 定时报表开关与 Cron
			EnableReporter: getEnvBool("ENABLE_REPORTER", false),
			ReportChannels: getEnvStringSlice("REPORTER_CHANNELS", []string{}),
			AlertChannels:  getEnvStringSlice("ALERT_CHANNELS", []string{}),

			TemplateDir: cmp.Or(os.Getenv("TEMPLATE_DIR"), "templates"),
			PublicURL:   cmp.Or(os.Getenv("PUBLIC_URL"), "http://localhost:8080"),
		},
		Ledger: ledger.Config{
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
			MaxTokens:        getEnvInt64("LLM_MAX_TOKENS", 4096),
			Temperature:      getEnvFloat("LLM_TEMPERATURE", 0.2),
			TopP:             getEnvFloat("LLM_TOP_P", 0.0),
			FrequencyPenalty: getEnvFloat("LLM_FREQUENCY_PENALTY", 0.0),
			PresencePenalty:  getEnvFloat("LLM_PRESENCE_PENALTY", 0.0),
		},
		WeCom: WeComConfig{
			CorpID:          os.Getenv("WECOM_CORP_ID"),
			AgentID:         getEnvInt64("WECOM_AGENT_ID", 0),
			Secret:          os.Getenv("WECOM_SECRET"),
			Token:           os.Getenv("WECOM_TOKEN"),
			EncodingAESKey:  os.Getenv("WECOM_ENCODING_AES_KEY"),
			SuccessTemplate: os.Getenv("WECOM_SUCCESS_TEMPLATE"),
			FailureTemplate: os.Getenv("WECOM_FAILURE_TEMPLATE"),

			// WSS 机器人凭证
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

// 辅助函数 2：读取 int64 类型环境变量
func getEnvInt64(key string, defaultVal int64) int64 {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.ParseInt(valStr, 10, 64)
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

// 辅助函数：从环境变量读取逗号分隔的字符串切片
func getEnvStringSlice(key string, defaultVals []string) []string {
	valStr := os.Getenv(key)
	if strings.TrimSpace(valStr) == "" {
		return defaultVals
	}

	parts := strings.Split(valStr, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(strings.ToLower(p))
		if name != "" {
			res = append(res, name)
		}
	}
	return res
}
