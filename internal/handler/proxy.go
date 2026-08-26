// internal/handler/proxy.go
package handler

import (
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

		// -------------------------------------------------------------
		// 1. 命中换票逻辑: /tallyview/auth?token=xxx
		// -------------------------------------------------------------
		if subPath == "auth" {
			token := c.Query("token")

			// 验签失败：直接 404 伪装，防探测
			if !crypto.VerifySignedToken(signSecret, "panel_sso", token) {
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
				7200,      // 2 小时时效 (秒)
				mountPath, // 仅在挂载路径下生效
				"",        // 当前域名
				true,      // 仅 HTTPS 传输
				true,      // HttpOnly
			)

			// 重定向至看板首页 (如 /tallyview/)
			c.Redirect(http.StatusFound, mountPath+"/")
			return
		}

		// -------------------------------------------------------------
		// 2. 正常看板页面与静态资源反代 (校验 Cookie)
		// -------------------------------------------------------------
		cookie, err := c.Cookie(SessionCookieName)
		if err != nil || !crypto.VerifySignedToken(signSecret, "session_user", cookie) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		// 授权通过，转发请求至后端 Fava 容器
		proxy.ServeHTTP(c.Writer, c.Request)
	}

	// 仅注册单前缀与通配路由，完美避开 Gin 冲突
	r.Any(mountPath, unifiedHandler)
	r.Any(mountPath+"/*any", unifiedHandler)

	return nil
}
