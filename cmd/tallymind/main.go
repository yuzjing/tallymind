// cmd/tallymind/main.go

package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"

	"os"
	"os/signal"
	"syscall"
	_ "tallymind/docs" // 自动生成的 docs
	"time"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/gin-gonic/gin"

	"tallymind/internal/config"
	"tallymind/internal/handler"
	"tallymind/internal/llm"
	"tallymind/internal/notifier"
	"tallymind/internal/plugin/wecom"
)

func initLogger(logLevel, logDir string) {
	var level slog.Level

	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo // 默认 info
	}

	var writers []io.Writer = []io.Writer{os.Stdout}

	if logDir != "" {
		_ = os.MkdirAll(logDir, 0755)
		logFilePath := filepath.Join(logDir, "tallymind.log")
		logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			writers = append(writers, logFile)
		}
	}

	multiwriter := io.MultiWriter(writers...)

	logger := slog.New(slog.NewTextHandler(multiwriter, &slog.HandlerOptions{
		Level: level,
	}))

	slog.SetDefault(logger) // 设置全局日志记录器
}

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		slog.Error("加载配置文件失败", "err", err)
		os.Exit(1)
	}
	initLogger(cfg.App.LogLevel, cfg.App.LogDir)

	slog.Info("🚀 tallymind 启动中 | 应用开关配置", "App", cfg.App)
	slog.Info("📁 账本存储配置", "ledger", cfg.Ledger)

	// 容错初始化各个子模块 (根据 .env 特性开关按需创建)

	var llmClient *llm.Client
	if cfg.App.EnableLLM {
		var err error
		llmClient, err = llm.NewClient(cfg.LLM)
		if err != nil {
			slog.Warn("⚠️ 大模型客户端未就绪 (降级运行)", "err", err)
		} else {
			slog.Info("🤖 大模型客户端已就绪", "model", cfg.LLM.Providers)
		}
	} else {
		slog.Info("⏹️ ENABLE_LLM=false，已关闭大模型功能")
	}

	// 上下文与操作系统优雅退出信号监听 (SIGINT / SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if cfg.App.EnableHTTPAPI {
		if cfg.App.Env == "production" {
			gin.SetMode(gin.ReleaseMode)
		}

		r := gin.Default()
		txHandler := handler.NewTransactionHandler(cfg.Ledger)

		if cfg.App.Env != "production" || cfg.App.Debug {
			r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
			slog.Info("📖 Swagger API 文档已开启", "path", "/swagger/index.html")
		} else {
			slog.Info("🔒 生产环境 (production) 已安全禁用 Swagger API 文档")
		}

		r.POST("/api/v1/transaction", txHandler.HandleTransaction)
		// 移动端 H5 电子小票控制器 (语义一清二楚！)
		receiptHandler := handler.NewReceiptHandler(cfg)
		receiptHandler.RegisterRoutes(r) // receipt/:id
		slog.Info("🌐 HTTP API 服务已启动", "port", cfg.App.Port)
		if cfg.App.EnableWeComHTTP {
			notifierMgr := notifier.NewManager()
			wecomClient := wecom.NewClient(&cfg.WeCom)
			notifierMgr.Register("wecom", wecomClient)
			wecomHandler := wecom.NewWeComHandler(txHandler, llmClient, wecomClient, cfg.App.TemplateDir, cfg.App.PublicURL, cfg.WeCom.SuccessTemplate, cfg.WeCom.FailureTemplate)
			callbackHandler := wecom.NewCallbackHandler(&cfg.WeCom, wecomHandler)
			callbackHandler.RegisterRoutes(r)
			slog.Info("已加载企业微信 HTTP 回调插件")
		}
		slog.Info("🌐 HTTP API 服务已启动", "port", cfg.App.Port)

		// 协程启动 Gin HTTP 服务
		go func() {
			if err := r.Run(":" + cfg.App.Port); err != nil {
				slog.Error("HTTP 服务异常退出", "err", err)
				os.Exit(1)
			}
		}()
	} else {
		slog.Info("⏹️ ENABLE_HTTPAPI=false, 🌐 HTTP API 服务已关闭")
	}

	var wsClient *wecom.WSClient
	if cfg.App.EnableWeComWSS {
		if cfg.WeCom.BotID != "" && cfg.WeCom.BotSecret != "" {
			wsClient = wecom.NewWSClient(cfg.WeCom, cfg.Ledger, llmClient)
			go wsClient.Start(ctx) // 启动企微长连接客户端
			slog.Info("💬 企微长连接后台服务已启动")
		} else {
			slog.Warn("⚠️ WECOM_BOT_ID 或 WECOM_BOT_SECRET 未配置，跳过 WSS 启动")
		}
	} else {
		slog.Info("⏹️ ENABLE_WECOM_WSS=false， 企微长连接后台服务已关闭")
	}

	//  阻塞主线程，等待退出信号
	<-sigChan
	slog.Info("收到退出信号，正在安全关闭 tallymind...")
	time.Sleep(300 * time.Millisecond)
}
