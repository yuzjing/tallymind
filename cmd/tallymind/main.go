// cmd/tallymind/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "tallymind/docs"
	"tallymind/internal/config"
	"tallymind/internal/cron"
	"tallymind/internal/handler"
	"tallymind/internal/llm"
	"tallymind/internal/logger"
	"tallymind/internal/notifier"
	"tallymind/internal/plugin/wecom"
	"tallymind/internal/service"
)

func main() {
	cfg, err := config.Load("config.toml")
	if err != nil {
		slog.Error("加载配置文件失败", "err", err)
		os.Exit(1)
	}
	closeLogger := logger.InitLogger(cfg.Log)
	defer closeLogger()

	slog.Info("🚀 tallymind 启动中 | 应用开关配置", "App", cfg.App)
	slog.Info("📁 账本存储配置", "ledger", cfg.Ledger)

	var llmClient *llm.Client
	if cfg.App.EnableLLM {
		var err error
		llmClient, err = llm.NewClient(cfg.LLM.ToDomain(cfg.App.TemplateDir))
		if err != nil {
			slog.Warn("⚠️ 大模型客户端未就绪 (降级运行)", "err", err)
		} else {
			slog.Info("🤖 大模型客户端已就绪", "model", cfg.LLM.Providers)
		}
	} else {
		slog.Info("⏹️ ENABLE_LLM=false，已关闭大模型功能")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 1. 初始化核心业务领域服务与统一通知中心
	accountingService := service.NewAccountingService(cfg, llmClient)
	notifierMgr := notifier.NewManager() // 默认已注册 "webhook" 通用驱动

	// 2. 初始化外部通知渠道客户端 (WeCom) 并注册到通知中心
	var wecomClient *wecom.Client
	var wecomHandler *wecom.WeComHandler
	if cfg.App.EnableWeComHTTP || cfg.App.EnableWeComWSS {
		wecomClient = wecom.NewClient(&cfg.WeCom)
		// ⭐️ 核心注册：将企业微信作为 "wecom" 驱动注册进通知中心
		notifierMgr.Register("wecom", wecomClient)

		wecomHandler = wecom.NewWeComHandler(
			accountingService,
			wecomClient,
			cfg.App.TemplateDir,
			cfg.WeCom.SuccessTemplate,
			cfg.WeCom.FailureTemplate,
			cfg.WeCom.ReportTemplate,
		)
	}

	// 3. 实例化多目标调度器 (注入 notifierMgr)
	scheduler := cron.NewScheduler(cfg, accountingService, notifierMgr)

	// 4. 独立启动后台定时报表推送调度器
	if cfg.App.EnableReporter {
		go scheduler.Start(ctx)
		slog.Info("⏰ 定时报表调度引擎已在后台启动守护")
	} else {
		slog.Info("⏹️ ENABLE_REPORTER=false，定时报表推送已关闭")
	}

	// 5. 启动企业微信 WSS 长连接服务
	var wsClient *wecom.WSClient
	if cfg.App.EnableWeComWSS {
		if cfg.WeCom.BotID != "" && cfg.WeCom.BotSecret != "" {
			wsClient = wecom.NewWSClient(cfg.WeCom, accountingService)
			go wsClient.Start(ctx)
			slog.Info("💬 企微长连接后台服务已启动")
		} else {
			slog.Warn("⚠️ WECOM_BOT_ID 或 WECOM_BOT_SECRET 未配置，跳过 WSS 启动")
		}
	} else {
		slog.Info("⏹️ ENABLE_WECOM_WSS=false，企微长连接后台服务已关闭")
	}

	// 6. 启动 HTTP REST API 与 Web 服务
	var srv *http.Server
	if cfg.App.EnableHTTPAPI {
		if cfg.App.Env == "production" {
			gin.SetMode(gin.ReleaseMode)
		}

		r := gin.Default()

		if cfg.App.Env != "production" {
			r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
			slog.Info("📖 Swagger API 文档已开启", "path", "/swagger/index.html")
		} else {
			slog.Info("🔒 生产环境 (production) 已安全禁用 Swagger API 文档")
		}

		txHandler := handler.NewTransactionHandler(accountingService)
		r.POST("/api/v1/transaction", txHandler.HandleTransaction)

		receiptHandler := handler.NewReceiptHandler(cfg, accountingService)
		receiptHandler.RegisterRoutes(r)

		reportHandler := handler.NewReportHandler(cfg, accountingService)
		reportHandler.RegisterRoutes(r)

		if cfg.App.EnableWeComHTTP && wecomHandler != nil {
			callbackHandler := wecom.NewCallbackHandler(&cfg.WeCom, wecomHandler)
			callbackHandler.RegisterRoutes(r)
			slog.Info("已加载企业微信 HTTP 回调插件路由")
		}

		if err := handler.RegisterPanelProxy(r, cfg.App.PanelPath, cfg.App.PanelURL, cfg.App.ReceiptSignSecret); err != nil {
			slog.Warn("⚠️ 看板反向代理挂载失败", "err", err)
		} else if cfg.App.PanelURL != "" {
			slog.Info("📊 统计大盘反向代理已挂载", "path", cfg.App.PanelPath, "target", cfg.App.PanelURL)
		}

		srv = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.App.Port),
			Handler: r,
		}

		go func() {
			slog.Info("🌐 HTTP API 服务正在监听", "port", cfg.App.Port)
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("HTTP 服务异常退出", "err", err)
				os.Exit(1)
			}
		}()
	} else {
		slog.Info("⏹️ ENABLE_HTTPAPI=false，🌐 HTTP API 服务已关闭")
	}

	// 7. 阻塞等待优雅停机
	<-sigChan
	slog.Info("收到终止信号，正在安全关闭 tallymind 各子系统...")

	cancel()

	if srv != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP 服务强制关闭异常", "err", err)
		} else {
			slog.Info("HTTP 服务已安全关闭")
		}
	}

	slog.Info("✅ tallymind 进程已完全退出")
}
