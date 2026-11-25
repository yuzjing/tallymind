// internal/handler/proxy.go
package handler

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"tallymind/internal/crypto"

	"github.com/gin-gonic/gin"
)

const SessionCookieName = "tallymind_session"

// RegisterPanelProxy 注册通用的带安全 SSO 票据核验与会话控制的反向代理网关
func RegisterPanelProxy(r gin.IRouter, mountPath, targetURL, signSecret string) error {
	mountPath = strings.TrimSpace(mountPath)
	targetURL = strings.TrimSpace(targetURL)

	if mountPath == "" || targetURL == "" {
		return nil
	}

	target, err := url.Parse(targetURL)
	if err != nil {
		return err
	}

	cleanMountPath := "/" + strings.Trim(mountPath, "/")
	proxy := httputil.NewSingleHostReverseProxy(target)

	unifiedHandler := func(c *gin.Context) {
		reqPath := c.Request.URL.Path
		token := c.Query("token")
		cookie, cookieErr := c.Cookie(SessionCookieName)

		slog.Debug("🔍 [Proxy] 收到看板路由请求",
			"method", c.Request.Method,
			"path", reqPath,
			"query_token_present", token != "",
			"cookie_present", cookieErr == nil,
		)

		// -------------------------------------------------------------
		// 方式 A：请求携带了 SSO Token (首屏直通：验签 + 隐式下发 Cookie + 直接反代)
		// -------------------------------------------------------------
		if token != "" {
			if crypto.VerifySignedToken(signSecret, "panel_sso", token) {
				slog.Info("🔑 [Proxy] Token 验签成功！下发 Cookie 并直接转发 Fava", "path", reqPath)

				sessionToken := crypto.GenerateSignedToken(
					signSecret,
					"session_user",
					2*time.Hour,
					crypto.DefaultTokenSignLen,
				)

				// 兼容公网 HTTPS (Cloudflare Tunnel 会透传 X-Forwarded-Proto=https) 与内网 HTTP 直连
				isHTTPS := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
				c.SetSameSite(http.SameSiteLaxMode)
				c.SetCookie(
					SessionCookieName,
					sessionToken,
					7200, // 2 小时有效
					"/",  // 根路径全站有效
					"",
					isHTTPS, // 公网 HTTPS 自动启用 Secure
					true,    // HttpOnly 防脚本窃取
				)

				// 阻断浏览器 Referer 携带 ?token=... 外泄，同时保留 0 重定向首屏秒开体验
				c.Header("Referrer-Policy", "no-referrer")

				// ⭐️ 直接反代给 Fava (0 重定向，首屏秒开！)
				proxy.ServeHTTP(c.Writer, c.Request)
				return
			}
			slog.Warn("❌ [Proxy] URL 携带的 Token 验签失败或已过期", "path", reqPath)
		}

		// -------------------------------------------------------------
		// 方式 B：无 Token 但携带合法 Session Cookie (Fava 内部跳转与静态资源)
		// -------------------------------------------------------------
		if cookieErr == nil && crypto.VerifySignedToken(signSecret, "session_user", cookie) {
			slog.Debug("🚀 [Proxy] Cookie 会话有效，转发 Fava", "path", reqPath)
			proxy.ServeHTTP(c.Writer, c.Request)
			return
		}

		// -------------------------------------------------------------
		// 方式 C：无 Token 且无 Cookie 的陌生访问 ➔ 404 伪装拦截
		// -------------------------------------------------------------
		slog.Warn("🔒 [Proxy] 无有效凭证，触发 404 伪装拦截", "path", reqPath)
		c.AbortWithStatus(http.StatusNotFound)
	}

	r.Any(cleanMountPath, unifiedHandler)
	r.Any(cleanMountPath+"/*any", unifiedHandler)

	return nil
}
