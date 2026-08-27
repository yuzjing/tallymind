// internal/handler/receipt.go
package handler

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"tallymind/internal/config"
	"tallymind/internal/crypto"
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
		c.String(http.StatusForbidden, "403 Forbidden: 签名无效")
		return
	}

	// 计算动态看板跳转链接：仅当配置了 URL 和 Path 时才拼接
	panelPath := ""
	if strings.TrimSpace(h.cfg.App.PanelURL) != "" && strings.TrimSpace(h.cfg.App.PanelPath) != "" {
		ssoToken := crypto.GenerateSignedToken(
			h.cfg.App.ReceiptSignSecret,
			"panel_sso",
			2*time.Hour,
			crypto.DefaultTokenSignLen,
		)
		cleanPath := "/" + strings.Trim(h.cfg.App.PanelPath, "/")
		// 直连大盘首页并附带 Token
		panelPath = fmt.Sprintf("%s/?token=%s", cleanPath, ssoToken)

		slog.Debug("🧾 [Receipt] 动态渲染小票大盘直达链接", "panel_path", panelPath)
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
		Receipt:   data,
		NowTime:   time.Now().Format("2006-01-02 15:04:05"),
		PanelPath: panelPath,
	})

}
