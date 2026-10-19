// internal/notifier/adapter.go
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// =============================================================================
// 1. 通用 HTTP Webhook 适配器 (无状态自包含 URL 通杀器)
// =============================================================================

// WebhookAdapter 通用 Webhook 驱动 (支持企微群/飞书群/钉钉群/Discord/Bark 等)
type WebhookAdapter struct {
	DefaultURL string
	client     *http.Client
}

func NewWebhookAdapter(defaultURL ...string) *WebhookAdapter {
	url := ""
	if len(defaultURL) > 0 {
		url = defaultURL[0]
	}
	return &WebhookAdapter{
		DefaultURL: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (w *WebhookAdapter) Push(ctx context.Context, msg Message) error {
	// 优先取 Message.Target 传入的动态 Webhook URL，没有则取默认 URL
	targetURL := msg.Target
	if targetURL == "" {
		targetURL = w.DefaultURL
	}
	if targetURL == "" {
		return fmt.Errorf("webhook push failed: target URL is empty")
	}

	// 组装标准 Webhook 兼容 Payload
	var bodyBytes []byte
	var err error

	if msg.Data != nil {
		bodyBytes, err = json.Marshal(msg.Data)
	} else {
		// 默认通用文本 JSON 结构
		payload := map[string]any{
			"msgtype": "text",
			"text": map[string]string{
				"content": msg.Content,
			},
		}
		bodyBytes, err = json.Marshal(payload)
	}
	if err != nil {
		return fmt.Errorf("marshal webhook body error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create webhook request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute webhook request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook server returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// =============================================================================
// 2. 通用凭据自刷新引擎 (AppID + AppSecret 抽象基类)
// =============================================================================

// TokenFetcher 定义获取最新 AccessToken 的远程协议闭包
type TokenFetcher func(ctx context.Context, appID, appSecret string) (token string, expiresIn time.Duration, err error)

// TokenManager 线程安全的通用凭据自动缓存与续期器
type TokenManager struct {
	AppID       string
	AppSecret   string
	fetcher     TokenFetcher
	cachedToken string
	expiresAt   time.Time
	mu          sync.RWMutex
}

func NewTokenManager(appID, appSecret string, fetcher TokenFetcher) *TokenManager {
	return &TokenManager{
		AppID:     appID,
		AppSecret: appSecret,
		fetcher:   fetcher,
	}
}

// GetToken 线程安全获取 Token (自动避让过期)
func (t *TokenManager) GetToken(ctx context.Context) (string, error) {
	t.mu.RLock()
	// 提前 60 秒判定过期，预留网络窗口
	if t.cachedToken != "" && time.Now().Add(60*time.Second).Before(t.expiresAt) {
		token := t.cachedToken
		t.mu.RUnlock()
		return token, nil
	}
	t.mu.RUnlock()

	t.mu.Lock()
	defer t.mu.Unlock()

	// 二次检查 (Double-Check)
	if t.cachedToken != "" && time.Now().Add(60*time.Second).Before(t.expiresAt) {
		return t.cachedToken, nil
	}

	token, expiresIn, err := t.fetcher(ctx, t.AppID, t.AppSecret)
	if err != nil {
		return "", fmt.Errorf("refresh access_token failed: %w", err)
	}

	t.cachedToken = token
	t.expiresAt = time.Now().Add(expiresIn)
	return token, nil
}
