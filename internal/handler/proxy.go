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

	mountPath = "/" + strings.Trim(mountPath, "/")
	proxy := httputil.NewSingleHostReverseProxy(target)

	// 统一处理代理与换票分发 (避免 Gin Radix Tree 路由冲突)
	unifiedHandler := func(c *gin.Context) {
		subPath := strings.TrimPrefix(c.Param("any"), "/")
		reqPath := c.Request.URL.Path

		slog.Debug("🔍 [Proxy] 收到看板路由请求",
			"method", c.Request.Method,
			"path", reqPath,
			"subpath", subPath,
			"client_ip", c.ClientIP(),
		)

		// -------------------------------------------------------------
		// 1. 命中换票逻辑: /tallyview/auth?token=xxx
		// -------------------------------------------------------------
		if subPath == "auth" {
			token := c.Query("token")
			slog.Debug("🔑 [Proxy] 正在执行 SSO 换票验证", "token", token)

			// 验签失败：直接 404 伪装，防探测
			if !crypto.VerifySignedToken(signSecret, "panel_sso", token) {
				slog.Warn("❌ [Proxy] SSO 换票验签失败或已过期", "token", token)
				c.AbortWithStatus(http.StatusNotFound)
				return
			}

			// 签发 2 小时有效期的会话 Token
			sessionToken := crypto.GenerateSignedToken(
				signSecret,
				"session_user",
				2*time.Hour,
				crypto.DefaultTokenSignLen,
			)

			// 写入仅限 HTTPS 传输的 HttpOnly 安全会话 Cookie
			c.SetCookie(
				SessionCookieName,
				sessionToken,
				7200, // 2 小时时效 (秒)
				"/",  // , // 仅在挂载路径下生效
				"",   // 当前域名
				true, // 仅 HTTPS 传输
				true, // HttpOnly
			)
			slog.Info("✅ [Proxy] SSO 换票成功！已下发 2 小时 Cookie，302 跳转首页", "redirect_to", mountPath+"/")

			// 重定向至看板首页 (如 /tallyview/)
			c.Redirect(http.StatusFound, mountPath+"/")
			return
		}

		// -------------------------------------------------------------
		// 2. 正常看板页面与静态资源反代 (校验 Cookie)
		// -------------------------------------------------------------
		cookie, err := c.Cookie(SessionCookieName)
		if err != nil {
			slog.Warn("🔒 [Proxy] 请求未携带 Session Cookie，触发 404 伪装拦截", "path", reqPath, "err", err)
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		if !crypto.VerifySignedToken(signSecret, "session_user", cookie) {
			slog.Warn("🔒 [Proxy] Session Cookie 签名无效或已过期", "path", reqPath, "cookie", cookie)
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		slog.Debug("🚀 [Proxy] 会话鉴权通过，流量透明转发至 Fava 容器", "target", targetURL, "path", reqPath)

		// 授权通过，转发请求至后端 容器
		proxy.ServeHTTP(c.Writer, c.Request)
	}

	// 仅注册单前缀与通配路由，完美避开 Gin 冲突
	r.Any(mountPath, unifiedHandler)
	r.Any(mountPath+"/*any", unifiedHandler)

	return nil
}
