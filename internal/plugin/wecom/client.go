// internal/plugin/wecom/client.go
package wecom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"tallymind/internal/config"
	"tallymind/internal/notifier"
)

type Client struct {
	cfg         *config.WeComConfig
	mu          sync.RWMutex
	accessToken string
	expiresAt   time.Time
	httpClient  *http.Client
}

type WecomTokenResponse struct {
	Errcode     int64  `json:"errcode"`
	Errmsg      string `json:"errmsg"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

func NewClient(cfg *config.WeComConfig) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetAccessToken 线程安全获取 AccessToken (内存缓存 + 提前 30 秒自动刷新)
func (c *Client) GetAccessToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.accessToken != "" && time.Now().Before(c.expiresAt) {
		token := c.accessToken
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// 双重校验 (Double-Check)
	if c.accessToken != "" && time.Now().Before(c.expiresAt) {
		return c.accessToken, nil
	}

	slog.DebugContext(ctx, "[WeCom] 内存 Token 已过期或不存在，向企微 API 刷新...")

	params := url.Values{}
	params.Set("corpid", c.cfg.CorpID)
	params.Set("corpsecret", c.cfg.Secret)

	requestURL := "https://qyapi.weixin.qq.com/cgi-bin/gettoken?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建刷新 Token 请求失败: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求企微 Token 接口网络错误: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp WecomTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("解析 Token 响应失败: %w", err)
	}

	if tokenResp.Errcode != 0 {
		return "", fmt.Errorf("企微返回 Token 错误: [%d] %s", tokenResp.Errcode, tokenResp.Errmsg)
	}

	c.accessToken = tokenResp.AccessToken
	// 提前 30 秒过期保底
	c.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn-30) * time.Second)

	return c.accessToken, nil
}

// SendMessage 发送企业微信自建应用消息 (/cgi-bin/message/send)
func (c *Client) SendMessage(ctx context.Context, msg *MessageRequest) error {
	accessToken, err := c.GetAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("获取发送凭证失败: %w", err)
	}

	if msg.AgentID == 0 {
		msg.AgentID = c.cfg.AgentID
	}

	requestURL := "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=" + accessToken

	jsonBody, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息请求体失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("创建发送消息请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("执行发送消息网络错误: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Errcode int64  `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("解析企微发送响应失败: %w", err)
	}

	if result.Errcode != 0 {
		return fmt.Errorf("企微发送消息失败: [%d] %s", result.Errcode, result.Errmsg)
	}

	return nil
}

// SendTextMessage 便捷发送纯文本/Markdown 给指定用户
func (c *Client) SendTextMessage(ctx context.Context, toUser, content string) error {
	return c.SendMessage(ctx, NewTextMessage(toUser, content))
}

// Push 实现 internal/notifier/notifier.go 的 Notifier 统一接口
func (c *Client) Push(ctx context.Context, msg notifier.Message) error {
	toUser := msg.Target
	if toUser == "" {
		return fmt.Errorf("[WeCom] 发送失败: Target 接收人 UserID 为空")
	}

	switch msg.Type {
	case notifier.TypeText:
		return c.SendMessage(ctx, NewTextMessage(toUser, msg.Content))
	case notifier.TypeJSON:
		if card, ok := msg.Data.(*TemplateCardContent); ok {
			return c.SendMessage(ctx, NewTemplateCardMessage(toUser, card))
		}
		return fmt.Errorf("[WeCom] msg.Data 不是有效的 *TemplateCardContent")
	default:
		slog.WarnContext(ctx, "[WeCom] 收到暂未显式适配的消息类型，降级为文本发送", "type", msg.Type)
		return c.SendMessage(ctx, NewTextMessage(toUser, msg.Content))
	}
}

// GetMediaURL 根据 MediaID 生成带有有效 Token 的企微临时素材下载地址
func (c *Client) GetMediaURL(ctx context.Context, mediaID string) (string, error) {
	token, err := c.GetAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("获取素材下载 Token 失败: %w", err)
	}
	return fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/media/get?access_token=%s&media_id=%s", token, mediaID), nil
}
