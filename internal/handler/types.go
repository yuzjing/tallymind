// internal/handler/types.go
package handler

import (
	"sync"
	"tallymind/internal/reporter"
)

// API通用结构体
type APIResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ReceiptPageData H5 移动端小票渲染上下文
type ReceiptPageData struct {
	Receipt    reporter.ReplyData `json:"receipt"`
	NowTime    string             `json:"now_time"`
	PanelPath  string             `json:"panel_path"`
	ReportPath string             `json:"report_path"`
}

// ReceiptStore 小票内存缓存结构体
type ReceiptStore struct {
	mu       sync.RWMutex
	receipts map[string]reporter.ReplyData
}
