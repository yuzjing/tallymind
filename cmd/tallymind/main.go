// cmd/tallymind/main.go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"tallymind/internal/config"
	"tallymind/internal/handler"
	"tallymind/internal/llm"
	"tallymind/internal/plugin/wecom"
)

func main() {
	cfg := config.Load()

	log.Printf("[INFO] 🚀 tallymind 启动中 | 应用开关配置: %+v\n", cfg.App)
	log.Printf("[INFO] 📁 账本存储配置: %+v\n", cfg.Ledger)

	// 容错初始化各个子模块 (根据 .env 特性开关按需创建)

	var llmClient *llm.Client
	if cfg.App.EnableLLM {
		var err error
		llmClient, err = llm.NewClient(cfg.LLM)
		if err != nil {
			log.Printf("[WARN] ⚠️ 大模型客户端初始化失败 (将降级运行): %v\n", err)
		} else {
			log.Printf("[INFO] 🤖 大模型客户端已就绪 | 模型: %s\n", cfg.LLM.Model)
		}
	} else {
		log.Printf("[INFO] ⏹️ ENABLE_LLM=false，已主动关闭大模型解析功能")
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
		r.POST("/api/v1/transaction", txHandler.HandleTransaction)
		log.Printf("[INFO] 🌐 HTTP API 服务已启动 | 监听端口 :%s\n", cfg.App.Port)

		// 协程启动 Gin HTTP 服务
		go func() {
			if err := r.Run(":" + cfg.App.Port); err != nil {
				log.Fatalf("[FATAL] 🚨 HTTP API 服务启动失败: %v\n", err)
			}
		}()
	} else {
		log.Printf("[INFO] ⏹️ ENABLE_HTTPAPI=false，已主动关闭通用 HTTP API 接口")
	}

	var wsClient *wecom.WSClient
	if cfg.App.EnableWeComWSS {
		if cfg.WeCom.BotID != "" && cfg.WeCom.BotSecret != "" {
			wsClient = wecom.NewWSClient(cfg.WeCom, cfg.Ledger, llmClient)
			go wsClient.Start(ctx) // 启动企微长连接客户端
			log.Println("[INFO] 💬 企微长连接客户端已就绪")
		} else {
			log.Println("[WARN] ⚠️ WECOM_BOT_ID 或 WECOM_BOT_SECRET 缺失，跳过企微长连接初始化")
		}
	} else {
		log.Println("[INFO] ⏹️ ENABLE_WECOM_WSS=false，已主动关闭企微 WSS 长连接服务")
	}

	//  阻塞主线程，等待退出信号
	<-sigChan
	log.Println("[INFO] 收到系统退出信号，正在安全关闭 tallymind...")
	time.Sleep(300 * time.Millisecond)
}
