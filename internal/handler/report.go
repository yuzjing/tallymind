// internal/handler/report.go
package handler

import (
	"html/template"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"log/slog"
	"tallymind/internal/config"
	"tallymind/internal/crypto"
	"tallymind/internal/service"
)

const ReportSessionCookieName = "tallymind_report_session"

type ReportHandler struct {
	cfg            *config.Config
	accountService *service.AccountingService
}

func NewReportHandler(cfg *config.Config, accountService *service.AccountingService) *ReportHandler {
	return &ReportHandler{
		cfg:            cfg,
		accountService: accountService,
	}
}

func (h *ReportHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/report", h.RenderReport)
}

// RenderReport 响应用户的实时请求 (从 URL 动态提取 period: weekly | monthly | quarterly | yearly)
func (h *ReportHandler) RenderReport(c *gin.Context) {
	token := c.Query("token")
	period := c.DefaultQuery("period", "weekly")
	dateStr := c.Query("date")

	startDate := c.Query("start")
	endDate := c.Query("end")

	// 解析基准时间，未传则默认取当前时间
	targetTime := time.Now()
	if dateStr != "" {
		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			targetTime = t
		}
	}

	cookie, cookieErr := c.Cookie(ReportSessionCookieName)

	authorized := false

	// -------------------------------------------------------------
	// 通行证 A: URL 携带合法 Token
	// -------------------------------------------------------------
	if token != "" && crypto.VerifySignedToken(h.cfg.App.ReceiptSignSecret, "report_view", token) {
		authorized = true
		// 顺手下发 2 小时临时 Session Cookie (全站有效)
		sessionToken := crypto.GenerateSignedToken(
			h.cfg.App.ReceiptSignSecret,
			"report_session",
			2*time.Hour,
			crypto.DefaultTokenSignLen,
		)
		c.SetCookie(ReportSessionCookieName, sessionToken, 7200, "/", "", false, true)
	}

	// -------------------------------------------------------------
	// 通行证 B: 浏览器已持有合法 Session Cookie
	// -------------------------------------------------------------
	if !authorized && cookieErr == nil && crypto.VerifySignedToken(h.cfg.App.ReceiptSignSecret, "report_session", cookie) {
		authorized = true
	}

	// -------------------------------------------------------------
	// 拦截未授权访问
	// -------------------------------------------------------------
	if !authorized {
		c.String(http.StatusForbidden, "403 Forbidden")
		return
	}

	//  实时按需调用计算引擎
	reportData, err := h.accountService.GetPeriodicReport(c.Request.Context(), period, targetTime, startDate, endDate)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "计算报表失败", "err", err)
		c.String(http.StatusInternalServerError, "实时计算报表失败: %v", err)
		return
	}

	reportData.Token = token

	// 即时渲染 templates/web/periodic_report.html
	tmplPath := filepath.Join(h.cfg.App.TemplateDir, h.cfg.App.ReportTemplate)
	tmpl, err := template.New("periodic_report.html").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"min": func(a, b int) int {
			if a < b {
				return a
			}
			return b
		},
	}).ParseFiles(tmplPath)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "加载报表模板失败", "path", tmplPath, "err", err)
		c.String(http.StatusInternalServerError, "模板解析失败: %v", err)
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(c.Writer, reportData); err != nil {
		slog.ErrorContext(c.Request.Context(), "执行模板渲染失败", "err", err)
	}
}
