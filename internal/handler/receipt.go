// internal/handler/receipt.go
package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"tallymind/internal/config"
	"tallymind/internal/crypto"
	"tallymind/internal/reporter"

	"github.com/gin-gonic/gin"
)

type ReceiptHandler struct {
	cfg   *config.Config
	store *ReceiptStore
}

var globalReceiptStore = &ReceiptStore{
	receipts: make(map[string]reporter.ReplyData),
}

func NewReceiptHandler(cfg *config.Config) *ReceiptHandler {
	return &ReceiptHandler{
		cfg:   cfg,
		store: globalReceiptStore,
	}
}

// SaveReceiptToMemory 保存小票到内存 (上限 50 条，超出自动淘汰最老数据防内存泄露)
func (h *ReceiptHandler) SaveReceiptToMemory(id string, data reporter.ReplyData) {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	if len(globalReceiptStore.receipts) > 50 {
		for k := range globalReceiptStore.receipts {
			delete(globalReceiptStore.receipts, k)
			break
		}
	}
	globalReceiptStore.receipts[id] = data
}

// BuildSignedReceiptURL 生成带 2 小时有效期的安全小票 URL
func BuildSignedReceiptURL(baseURL, secret, receiptID string) string {
	// 传入 2 小时 TTL 与 16 位签名长度
	token := crypto.GenerateSignedToken(secret, receiptID, 2*time.Hour, crypto.DefaultTokenSignLen)
	return fmt.Sprintf("%s/receipt/%s?token=%s", strings.TrimRight(baseURL, "/"), receiptID, token)
}

// RegisterRoutes 注册小票页面路由
func (h *ReceiptHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/receipt/:id", h.RenderReceipt)
}

// RenderReceipt 渲染移动端 HTML 小票页面
func (h *ReceiptHandler) RenderReceipt(c *gin.Context) {
	id := c.Param("id")
	token := c.Query("token")

	// 🔒 安全验签：Token 错误或超过 2 小时直接 403 阻断！
	if !crypto.VerifySignedToken(h.cfg.App.ReceiptSignSecret, id, token) {
		c.String(http.StatusForbidden, "403 Forbidden: 小票访问签名无效或链接已过期 (有效期 2 小时)")
		return
	}
	// 提取内存数据
	h.store.mu.RLock()
	data, exist := h.store.receipts[id]
	h.store.mu.RUnlock()

	if !exist {
		c.String(http.StatusNotFound, "404 Not Found: 小票数据不存在")
		return
	}

	// 渲染 templates/web/receipt.html
	tmplPath := filepath.Join(h.cfg.App.TemplateDir, "web/receipt.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		c.String(http.StatusInternalServerError, "小票模板加载失败: %v", err)
		return
	}
	pageData := ReceiptPageData{
		Receipt: data,
		NowTime: time.Now().Format("2006-01-02 15:04:05"),
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.Execute(c.Writer, pageData)

}
