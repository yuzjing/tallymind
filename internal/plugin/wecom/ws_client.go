// internal/plugin/wecom/ws_client.go
package wecom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"tallymind/internal/config"
	"tallymind/internal/notifier"
	"tallymind/internal/service"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type WSHeader struct {
	ReqID string `json:"req_id"`
}

type WSFrame struct {
	Cmd     string          `json:"cmd"`
	Headers WSHeader        `json:"headers"`
	ErrCode int             `json:"err_code,omitempty"`
	ErrMsg  string          `json:"err_msg,omitempty"`
	Body    json.RawMessage `json:"body,omitempty"`
}

type SubscribeBody struct {
	BotID     string `json:"bot_id"`
	BotSecret string `json:"secret"`
}

type MsgCallbackBody struct {
	MsgID       string `json:"msgid"`
	AIBotID     string `json:"aibotid"`
	ChatType    string `json:"chattype"`
	ResponseURL string `json:"response_url"`
	From        struct {
		UserID string `json:"userid"`
	} `json:"from"`
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

type WSClient struct {
	wecomCfg       config.WeComConfig
	accountService *service.AccountingService
	conn           *websocket.Conn
	mu             sync.Mutex
}

func NewWSClient(wecomCfg config.WeComConfig, accountService *service.AccountingService) *WSClient {
	return &WSClient{
		wecomCfg:       wecomCfg,
		accountService: accountService,
	}
}

func buildWeComRespondBody(msg notifier.Message) *MessageRequest {
	switch msg.Type {
	case notifier.TypeImage:
		return &MessageRequest{
			MsgType: "image",
			Image:   &MediaContent{MediaID: msg.FilePath},
		}

	case notifier.TypeVideo:
		return &MessageRequest{
			MsgType: "video",
			Video:   &VideoContent{MediaID: msg.FilePath, Title: "账单视频"},
		}

	case notifier.TypeDocument:
		return &MessageRequest{
			MsgType: "file",
			File:    &MediaContent{MediaID: msg.FilePath},
		}

	case notifier.TypeJSON:
		if card, ok := msg.Data.(*TemplateCardContent); ok {
			return NewTemplateCardMessage("", card)
		}
		return NewTextMessage("", fmt.Sprintf("收到数据: %v", msg.Data))

	default:
		return NewTextMessage("", msg.Content)
	}
}

func (ws *WSClient) safeWriteJSON(v any) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.conn == nil {
		return fmt.Errorf("websocket 连接未建立")
	}
	raw, _ := json.Marshal(v)
	slog.Debug("WSS 我方发送原始帧", "raw", string(raw))

	return ws.conn.WriteJSON(v)
}

func (ws *WSClient) Start(ctx context.Context) {
	wsURL := "wss://openws.work.weixin.qq.com"

	for {
		select {
		case <-ctx.Done():
			slog.Info("收到停止信号，企微 WSS 长连接服务已安全退出")
			return
		default:
			slog.Debug("正在建立企微 WSS 长连接", "URL", wsURL)
			err := ws.connectAndSubscribe(ctx, wsURL)
			if err != nil {
				slog.Debug("企微 WSS 长连接异常断开", "err", err)
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func (ws *WSClient) connectAndSubscribe(ctx context.Context, wsURL string) error {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{})
	if err != nil {
		return err
	}
	ws.conn = conn
	defer ws.conn.Close()

	if err := ws.subscribe(); err != nil {
		return fmt.Errorf("[ERROR] 企微 WSS 长连接鉴权失败: %w", err)
	}
	slog.Info("WSS 长连接鉴权成功，进入 24 小时消息监听状态...")

	go ws.startHeartbeat(ctx)

	for {
		_, rawBytes, err := ws.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("读取 WSS 原始数据失败: %w", err)
		}

		slog.Debug("WSS 企微发来原始帧", "raw", string(rawBytes))

		var frame WSFrame
		if err := json.Unmarshal(rawBytes, &frame); err != nil {
			slog.Debug("原始数据反序列化为 WSFrame 失败", "raw", err)
			continue
		}

		if frame.ErrCode != 0 {
			slog.Debug("企微官方报错", "错误码", frame.ErrCode, "错误信息", frame.ErrMsg)
		}

		ws.handleFrame(ctx, frame)
	}
}

func (ws *WSClient) subscribe() error {
	subBody := SubscribeBody{
		BotID:     ws.wecomCfg.BotID,
		BotSecret: ws.wecomCfg.BotSecret,
	}
	bodyBytes, _ := json.Marshal(subBody)

	frame := WSFrame{
		Cmd: "aibot_subscribe",
		Headers: WSHeader{
			ReqID: uuid.New().String(),
		},
		Body: bodyBytes,
	}

	return ws.safeWriteJSON(frame)
}

func (ws *WSClient) startHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingFrame := WSFrame{
				Cmd: "ping",
				Headers: WSHeader{
					ReqID: uuid.New().String(),
				},
			}
			if err := ws.safeWriteJSON(pingFrame); err != nil {
				slog.Debug("WSS 心跳包发送失败", "err", err)
				return
			}
		}
	}
}

func (ws *WSClient) handleFrame(ctx context.Context, frame WSFrame) {
	if frame.Cmd != "aibot_msg_callback" || len(frame.Body) == 0 {
		return
	}

	var msgBody MsgCallbackBody
	if err := json.Unmarshal(frame.Body, &msgBody); err != nil {
		slog.Debug("消息回调体反序列化失败", "raw", err)
		return
	}

	userText := msgBody.Text.Content
	userID := msgBody.From.UserID

	slog.InfoContext(ctx, "[WSS] 收到企微机器人消息", "userID", userID, "text", userText)

	input := service.AccountingInput{
		UserID:        userID,
		SourceChannel: "wecom_wss",
		MessageID:     msgBody.MsgID,
		MessageTime:   time.Now(),
		UserText:      userText,
	}

	replyData, err := ws.accountService.Process(ctx, input)
	if err != nil {
		slog.ErrorContext(ctx, "[WSS] 记账失败", "err", err)
		ws.respondMsg(frame.Headers.ReqID, msgBody.ResponseURL, notifier.Message{
			Type:    notifier.TypeText,
			Content: fmt.Sprintf("❌ 记账失败: %v", err),
		})
		return
	}

	replyText := replyData.SummaryHeadline()
	if replyData.JumpURL != "" {
		replyText += fmt.Sprintf("\n\n👉 点击查看电子小票: %s", replyData.JumpURL)
	}

	ws.respondMsg(frame.Headers.ReqID, msgBody.ResponseURL, notifier.Message{
		Type:    notifier.TypeText,
		Content: replyText,
	})
}

func (ws *WSClient) respondMsg(reqID string, responseURL string, msg notifier.Message) {
	respBody := buildWeComRespondBody(msg)
	bodyBytes, _ := json.Marshal(respBody)

	frame := WSFrame{
		Cmd: "aibot_respond_msg",
		Headers: WSHeader{
			ReqID: reqID,
		},
		Body: bodyBytes,
	}

	err := ws.safeWriteJSON(frame)
	if err == nil {
		return
	}
	slog.Debug("WSS 长连接回传失败，尝试使用 response_url 降级回复...", "err", err, "responseURL", responseURL)

	if responseURL != "" {
		go func() {
			req, err := http.NewRequest("POST", responseURL, bytes.NewBuffer(bodyBytes))
			if err == nil {
				req.Header.Set("Content-Type", "application/json")
				client := &http.Client{Timeout: 5 * time.Second}
				resp, err := client.Do(req)
				if err == nil {
					resp.Body.Close()
					slog.Debug("企微 response_url 快捷回复成功", "code", resp.StatusCode)
				}
			}
		}()
	}
}
