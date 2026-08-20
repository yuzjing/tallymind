// plugin/wecom/client.go
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
	"tallymind/internal/config"
	"tallymind/internal/notifier"
	"time"
)

type Client struct {
	cfg *config.WeComConfig

	// 内存维护态，不暴露给外部

	mu          sync.RWMutex
	accessToken string
	expiresAt   time.Time
	httpClient  *http.Client
}

type WecomTokenResponse struct {
	Errcode     int64  `json:"errcode"` // 错误码
	Errmsg      string `json:"errmsg"`  // 错误信息
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

// GetAccessToken 内部私有或对包内公开，自动处理刷新逻辑

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

	// 双重校验
	if c.accessToken != "" && time.Now().Before(c.expiresAt) {

		return c.accessToken, nil
	}
	slog.Debug("[WeCom] 内存 Token 已过期或不存在，准备向企微 API 刷新 Token")

	params := url.Values{}
	params.Set("corpid", c.cfg.CorpID)
	params.Set("corpsecret", c.cfg.Secret)

	requestURL := "https://qyapi.weixin.qq.com/cgi-bin/gettoken?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 解析响应
	var tokenResp WecomTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if tokenResp.Errcode != 0 {
		return "", fmt.Errorf("failed to get access token: [%d] %s", tokenResp.Errcode, tokenResp.Errmsg)
	}

	// 更新状态
	c.accessToken = tokenResp.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn-30) * time.Second)

	return c.accessToken, nil
}

func (c *Client) SendMessage(ctx context.Context, msg *MessageRequest) error {
	accessToken, err := c.GetAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	if msg.AgentID == 0 {
		msg.AgentID = c.cfg.AgentID
	}

	reruestURL := "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=" + accessToken

	jsonBody, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reruestURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()
	// 解析响应
	var result struct {
		Errcode int64  `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Errcode != 0 {
		return fmt.Errorf("failed to send message: [%d] %s", result.Errcode, result.Errmsg)
	}

	return nil
}

// Push 隐式实现 internal/notifier/notifier.go 的 Notifier 接口
func (c *Client) Push(ctx context.Context, msg notifier.Message) error {
	toUser := msg.Target
	if toUser == "" {
		return fmt.Errorf("target user is empty")
	}

	switch msg.Type {
	case notifier.TypeText:
		return c.SendMessage(ctx, NewTextMessage(toUser, msg.Content))
	case notifier.TypeJSON:
		if card, ok := msg.Data.(*TemplateCardContent); ok {
			return c.SendMessage(ctx, NewTemplateCardMessage(toUser, card))
		}
		return fmt.Errorf("wecom push error: msg.Data is not a *wecom.TemplateCard")

	default:
		slog.WarnContext(ctx, "[WeCom] 收到暂未显式适配的消息类型，降级处理", "type", msg.Type)
		return c.SendMessage(ctx, NewTextMessage(toUser, fmt.Sprintf("收到 [%s] 消息:\n%s", msg.Type, msg.Content)))
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
