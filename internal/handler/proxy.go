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

	// 只要路径或目标地址未配置，静默关闭代理
	if mountPath == "" || targetURL == "" {
		return nil
	}

	target, err := url.Parse(targetURL)
	if err != nil {
		return err
	}

	mountPath = "/" + strings.Trim(mountPath, "/")
	proxy := httputil.NewSingleHostReverseProxy(target)

	// -------------------------------------------------------------
	// 1. 通用 SSO 票据换票端点: GET {mountPath}/auth?token=xxx
	// -------------------------------------------------------------
	r.GET(mountPath+"/auth", func(c *gin.Context) {
		token := c.Query("token")

		// 校验外部传入的时效票据 (Token 无效或过期直接 404 隐身)
		if !crypto.VerifySignedToken(signSecret, "panel_sso", token) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		// 票据核验成功，签发 2 小时有效期的会话 Token
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
			mountPath, // 严格限制在挂载路径生效
			"",        // 自动继承当前请求域名
			true,      // 强制仅 HTTPS 传输 (防止网络抓包)
			true,      // HttpOnly (防止客户端脚本窃取)
		)

		// 重定向至看板主页
		c.Redirect(http.StatusFound, mountPath+"/")
	})

	// -------------------------------------------------------------
	// 2. 目标服务反向代理主路由 (集成会话安全拦截中间件)
	// -------------------------------------------------------------
	authProxyHandler := func(c *gin.Context) {
		cookie, err := c.Cookie(SessionCookieName)

		// 未持有合法会话凭证时，统一返回 404 伪装 (隐形防御，防探测)
		if err != nil || !crypto.VerifySignedToken(signSecret, "session_user", cookie) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		// 鉴权通过，透明反代流量至后端目标服务
		proxy.ServeHTTP(c.Writer, c.Request)
	}

	r.Any(mountPath, authProxyHandler)
	r.Any(mountPath+"/*any", authProxyHandler)

	return nil
}
