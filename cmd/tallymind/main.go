// cmd/tallymind/main.go
package main

import (
	"log"

	"tallymind/internal/config"
)

func main() {
	cfg := config.Load()

	log.Printf("[INFO] tallymind 正在启动... | 环境: %s | 模式: %s | Debug: %t | LLM: %t | 消息推送: %t\n",
		cfg.App.Env, cfg.App.RunMode, cfg.App.Debug, cfg.App.EnableLLM, cfg.App.EnableReporter)
}
