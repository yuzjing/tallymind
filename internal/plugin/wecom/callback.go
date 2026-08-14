// internal/plugin/wecom/callback.go
package wecom

import (
	"context"
	"encoding/xml"
	"log/slog"
	"net/http"
	"tallymind/internal/config"

	"github.com/gin-gonic/gin"
)

// EncryptedXMLMsg 企微 POST 发来的加密 XML 结构体
type EncryptedXMLMsg struct {
	XMLName    xml.Name `xml:"xml"`
	ToUserName string   `xml:"ToUserName"`
	AgentID    string   `xml:"AgentID"`
	Encrypt    string   `xml:"Encrypt"`
}

// PlainXMLMsg 企微消息解密后的明文 XML 结构体
type PlainXMLMsg struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int64    `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
	PicURL       string   `xml:"PicURL"`
	MsgID        string   `xml:"MsgID"`
	AgentID      string   `xml:"AgentID"`
}

// CallbackHandler 企微 Webhook 路由接收器
type CallbackHandler struct {
	cfg          *config.WeComConfig
	wecomHandler *WeComHandler
	crypt        *WXBizMsgCrypt
}

func NewCallbackHandler(cfg *config.WeComConfig, wecomHandler *WeComHandler) *CallbackHandler {
	crypt := NewWXBizMsgCrypt(cfg.Token, cfg.EncodingAESKey, cfg.CorpID)
	return &CallbackHandler{
		cfg:          cfg,
		wecomHandler: wecomHandler,
		crypt:        crypt,
	}
}

// RegisterRoutes 注册企微回调路由到 Gin 引擎上
func (h *CallbackHandler) RegisterRoutes(r *gin.Engine) {
	group := r.Group("/wecom/callback")
	{
		group.GET("/callback", h.VerifyURL)
		group.POST("/callback", h.ReceiveMessage)
	}

}

func (h *CallbackHandler) VerifyURL(c *gin.Context) {
	ctx := c.Request.Context()

	msgSignature := c.Query("msg_signature")
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")
	echoStr := c.Query("echostr")

	slog.InfoContext(ctx, "[WeCom] 收到企微 URL 验证请求", "signature", msgSignature)

	// 解密 echostr
	replyEchoStr, err := h.crypt.VerifyURL(msgSignature, timestamp, nonce, echoStr)
	if err != nil {
		slog.ErrorContext(ctx, "[WeCom] URL 验证签名解密失败", "err", err)
		c.JSON(http.StatusBadRequest, "verify failed")
		return
	}

	slog.InfoContext(ctx, "[WeCom] URL 验证成功", "echostr", replyEchoStr)
	c.String(http.StatusOK, string(replyEchoStr))
}

func (h *CallbackHandler) ReceiveMessage(c *gin.Context) {
	ctx := c.Request.Context()

	msgSignature := c.Query("msg_signature")
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")

	bodyDabata, err := c.GetRawData()
	if err != nil {
		slog.ErrorContext(ctx, "[WeCom] 读取回调 POST Body 失败", "err", err)
		c.String(http.StatusBadRequest, "read body failed")
		return
	}

	msg, err := h.crypt.DecryptMsg(msgSignature, timestamp, nonce, bodyDabata)
	if err != nil {
		slog.ErrorContext(ctx, "[WeCom] 消息解密失败", "err", err)
		c.JSON(http.StatusBadRequest, "decrypt failed")
		return
	}

	c.String(http.StatusOK, "success")

	go func() {
		// 使用 context.Background()，防止 Gin 的 c.Request.Context() 被取消
		bgCtx := context.Background()
		slog.InfoContext(bgCtx, "[WeCom] 收到用户消息", "fromUser", msg.FromUserName, "msgtype", msg.MsgType)

		h.wecomHandler.HandleMessage(bgCtx, msg)

	}()
}
