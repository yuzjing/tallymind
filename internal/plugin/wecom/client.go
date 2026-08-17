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

type MessageRequest struct {
	ToUser                   string               `json:"touser"`                     // 接收者，多个接收者用逗号分隔
	ToParty                  string               `json:"toparty"`                    // 接收者，多个接收者用逗号分隔
	ToTag                    string               `json:"totag"`                      // 接收者，多个接收者用逗号分隔
	MsgType                  string               `json:"msgtype"`                    // 消息类型
	AgentID                  int64                `json:"agentid"`                    // 自建应用 ID
	EnableDuplicationCheck   bool                 `json:"enable_duplication_check"`   // 是否开启重复消息检查
	DuplicationCheckInterval int                  `json:"duplication_check_interval"` // 重复消息检查间隔，单位秒
	Markdown                 *MarkdownContent     `json:"markdown,omitempty"`         // Markdown 消息内容
	TemplateCard             *TemplateCardContent `json:"template_card,omitempty"`    // 模板卡片消息内容
}

type MarkdownContent struct {
	Content string `json:"content"` // Markdown 内容
}

type TemplateCardContent struct {
	// ==================== 1. 公用基础字段 ====================
	CardType string `json:"card_type"` // "text_notice" 或 "news_notice"

	// 主标题信息 (两种卡片均支持 title 和 desc)
	MainTitle struct {
		Title string `json:"title"`          // 一级标题
		Desc  string `json:"desc,omitempty"` // 标题辅助信息/描述
	} `json:"main_title"`

	// 卡片底部跳转行为 (两种卡片均必填)
	CardAction struct {
		Type string `json:"type"` // 1: 跳转 URL, 2: 打开小程序
		URL  string `json:"url,omitempty"`
	} `json:"card_action"`

	// ==================== 2. 文本通知型 (text_notice) 专属字段 ====================

	// 仅 text_notice 支持：二级副标题
	SubTitleText string `json:"sub_title_text,omitempty"`

	// 仅 text_notice 支持：关键数据突显 (如大号字体显示 "￥500.00")
	EmphasisContent *struct {
		Title string `json:"title"`          // 突显数值
		Desc  string `json:"desc,omitempty"` // 数值描述
	} `json:"emphasis_content,omitempty"`

	// 仅 text_notice 支持：左右键值对列表 (如 "分类: 餐饮")
	HorizontalContentList []struct {
		KeyName string `json:"keyname"`
		Value   string `json:"value"`
	} `json:"horizontal_content_list,omitempty"`

	// ==================== 3. 图文展示型 (news_notice) 专属字段 ====================

	// 仅 news_notice 支持：顶部大图 (CardImage 和 ImageTextArea 必须二选一填一个)
	CardImage *struct {
		URL         string  `json:"url"`                    // 图片 URL
		AspectRatio float64 `json:"aspect_ratio,omitempty"` // 宽高比，默认 1.3
	} `json:"card_image,omitempty"`

	// 仅 news_notice 支持：左图右文摘要区
	ImageTextArea *struct {
		Type  int    `json:"type,omitempty"`  // 1: 跳转 URL
		URL   string `json:"url,omitempty"`   // 跳转地址
		Title string `json:"title,omitempty"` // 标题
		Desc  string `json:"desc,omitempty"`  // 描述
	} `json:"image_text_area,omitempty"`

	// 仅 news_notice 支持：卡片下方垂直列表项 (如文章/明细推荐)
	VerticalContentList []struct {
		Title string `json:"title"`          // 列表标题
		Desc  string `json:"desc,omitempty"` // 列表描述
	} `json:"vertical_content_list,omitempty"`

	// 仅 news_notice 支持：底部多链接跳转列表 (最多 3 个)
	JumpList []struct {
		Type  int    `json:"type"`  // 1: 跳转 URL
		Title string `json:"title"` // 链接文案
		URL   string `json:"url"`   // 链接地址
	} `json:"jump_list,omitempty"`
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
func (c *Client) Push(ctx context.Context, msg *notifier.Message) error {
	toUser := msg.Target
	if toUser == "" {
		return fmt.Errorf("target user is empty")
	}

	switch msg.Type {
	case notifier.TypeText:
		return c.SendMessage(ctx, NewMarkdownMessage(toUser, msg.Content))
	case notifier.TypeJSON:
		if card, ok := msg.Data.(*TemplateCardContent); ok {
			return c.SendMessage(ctx, NewTemplateCardMessage(toUser, card))
		}
		return fmt.Errorf("wecom push error: msg.Data is not a *wecom.TemplateCard")

	default:
		slog.WarnContext(ctx, "[WeCom] 收到暂未显式适配的消息类型，降级处理", "type", msg.Type)
		return c.SendMessage(ctx, NewMarkdownMessage(toUser, fmt.Sprintf("收到 [%s] 消息:\n%s", msg.Type, msg.Content)))
	}
}

func NewMarkdownMessage(toUser string, content string) *MessageRequest {
	return &MessageRequest{
		ToUser:  toUser,
		MsgType: "markdown",
		Markdown: &MarkdownContent{
			Content: content,
		},
	}
}

func NewTemplateCardMessage(toUser string, card *TemplateCardContent) *MessageRequest {
	return &MessageRequest{
		ToUser:       toUser,
		MsgType:      "template_card",
		TemplateCard: card,
	}
}
