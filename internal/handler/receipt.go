// internal/handler/receipt.go
package handler

import (
	"html/template"
	"net/http"
	"path/filepath"
	"time"

	"tallymind/internal/config"
	"tallymind/internal/service"

	"github.com/gin-gonic/gin"
)

type ReceiptHandler struct {
	cfg            *config.Config
	accountService *service.AccountingService
}

func NewReceiptHandler(cfg *config.Config, accountService *service.AccountingService) *ReceiptHandler {
	return &ReceiptHandler{
		cfg:            cfg,
		accountService: accountService,
	}
}

// RegisterRoutes 注册小票页面路由
func (h *ReceiptHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/receipt/:id", h.RenderReceipt)
}

// RenderReceipt 渲染移动端 HTML 小票页面
func (h *ReceiptHandler) RenderReceipt(c *gin.Context) {
	id := c.Param("id")
	token := c.Query("token")

	// 1. 直接向 service 查询小票数据 (内部已自动完成安全验签与 2 小时时效判定)
	data, ok := h.accountService.GetReceipt(id, token)
	if !ok {
		c.String(http.StatusForbidden, "403 Forbidden: 小票访问签名无效、已过期 (有效期 2 小时) 或小票不存在")
		return
	}

	// 2. 动态拼接模板路径
	tmplPath := filepath.Join(h.cfg.App.TemplateDir, h.cfg.App.ReceiptTemplate)
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		c.String(http.StatusInternalServerError, "小票模板加载失败: %v", err)
		return
	}

	// 3. 输出 HTML 响应
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.Execute(c.Writer, ReceiptPageData{
		Receipt: data,
		NowTime: time.Now().Format("2006-01-02 15:04:05"),
	})

}
