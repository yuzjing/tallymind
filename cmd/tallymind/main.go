// cmd/tallymind/main.go
package main

import (
	"log"

	"tallymind/internal/config"
)

func main() {
	cfg := config.Load()

	log.Printf("[%s] Started | Env: %s | Debug: %t\n", "tallymind", cfg.AppEnv, cfg.Debug)
}
