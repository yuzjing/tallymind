// internal/plugin/wecom/ws_client.go
package wecom

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"tallymind/internal/config"
	"tallymind/internal/ledger"
	"tallymind/internal/llm"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type WSHeader struct {
	ReqID string `json:"req_id"`
}
type WSFrame struct {
	Cmd     string   `json:"cmd"`
	Headers WSHeader `json:"headers"`
	ErrCode int      `json:"err_code,omitempty"`
	ErrMsg  string   `json:"err_msg,omitempty"`
	Body    string   `json:"body,omitempty"`
}

//1.鉴权订阅请求body

type SubscribeBody struct {
	BotID     string `json:"bot_id"`
	BotSecret string `json:"bot_secret"`
}

//2. 消息回调body

type MsgCallbackBody struct {
	MsgID    string `json:"msg_id"`
	AIBotID  string `json:"aibot_id"`
	ChatType string `json:"chat_type"`
	From     struct {
		UserID string `json:"user_id"`
	} `json:"from"`
	MsgType string `json:"msg_type"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

//3.回复消息body

type RespondMsgBody struct {
	MsgType string `json:"msg_type"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

type WSClient struct {
	wecomCfg  config.WeComConfig
	ledgerCfg config.LedgerConfig
	llmClient *llm.Client
	conn      *websocket.Conn
}

func NewWSClient(wecomCfg config.WeComConfig, ledgerCfg config.LedgerConfig, llmClient *llm.Client) *WSClient {
	return &WSClient{
		wecomCfg:  wecomCfg,
		ledgerCfg: ledgerCfg,
		llmClient: llmClient,
	}
}

//Start 启动长连接：鉴权订阅 -> 启动心跳协程 -> 消息监听循环

func (ws *WSClient) Start(ctx context.Context) {
	wsURL := "wss://openws.work.weixin.qq.com"

	for {
		select {
		case <-ctx.Done():
			log.Println("[INFO] 收到停止信号，企微 WSS 长连接服务已安全退出")
			return
		default:
			log.Printf("[INFO] 正在建立企微 WSS 长连接 (%s)...\n", wsURL)
			err := ws.connectAndSubscribe(ctx, wsURL)
			if err != nil {
				log.Printf("[WARN] 企微 WSS 长连接异常断开: %v | 5 秒后自动尝试重连...\n", err)
				time.Sleep(5 * time.Second)
			}
		}
	}
}

// connectAndListen 执行建立连接、鉴权、心跳与消息读取
func (ws *WSClient) connectAndSubscribe(ctx context.Context, wsURL string) error {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{})
	if err != nil {
		return err
	}
	ws.conn = conn

	defer ws.conn.Close()

	// 1. 发起企微官方规定的 aibot_subscribe 鉴权订阅包
	if err := ws.suscribe(); err != nil {
		return fmt.Errorf("[ERROR] 企微 WSS 长连接鉴权失败: %w", err)
	}
	log.Println("[INFO] 企微 WSS 长连接鉴权订阅成功！进入 24 小时消息监听状态...")

	// 2. 后台启动 30s 定时 Ping 心跳保活协程
	go ws.startHeartbeat(ctx)

	// 3. 消息读取主循环
	for {
		var frame WSFrame
		if err := ws.conn.ReadJSON(&frame); err != nil {
			return fmt.Errorf("读取 WSS 消息帧失败: %w", err)
		}

		ws.handleFrame(ctx, frame)

	}
}

func (ws *WSClient) suscribe() error {
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
		Body: string(bodyBytes),
	}

	return ws.conn.WriteJSON(frame)
}

func (ws *WSClient) startHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
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
			if err := ws.conn.WriteJSON(pingFrame); err != nil {
				log.Printf("[WARN] 心跳包发送失败: %v", err)
				return
			}
		}

	}
}

func (ws *WSClient) handleFrame(ctx context.Context, frame WSFrame) {
	// 只处理企微推过来的家人消息回调 (aibot_msg_callback)
	if frame.Cmd == "aibot_msg_callback" {
		return
	}

	var msgBody MsgCallbackBody
	if err := json.Unmarshal([]byte(frame.Body), &msgBody); err != nil {
		log.Printf("[WARN] 解析消息回调体失败: %v", err)
		return
	}

	userText := msgBody.Text.Content
	userID := msgBody.From.UserID
	reporter := cmp.Or(userID, ws.ledgerCfg.DefaultReporter)

	log.Printf("[INFO] 收到企微长连接消息 | 用户: %s | 内容: %s\n", userID, userText)

	// 1. 组装 RequestContext
	reqCtx := ledger.RequestContext{
		UserID:           userID,
		Reporter:         reporter,
		SourceChannel:    "wecom_wss",
		DefaultCurrency:  ws.ledgerCfg.DefaultCurrency,
		FallbackCategory: ws.ledgerCfg.FallbackCategory,
		FallbackAccount:  ws.ledgerCfg.FallbackAccount,
	}

	// 2. 调用 LLM 客户端解析自然语言 -> BatchTransactions 结构体
	if ws.llmClient == nil {
		log.Printf("[ERROR] LLM 客户端未初始化，无法解析消息")
		ws.respondMsg(frame.Headers.ReqID, "记账失败：系统未配置大模型 API")
		return
	}

	batch, err := ws.llmClient.ParseTransaction(ctx, userText)
	if err != nil {
		log.Printf("[WARN] AI 提取账单失败: %v\n", err)
		ws.respondMsg(frame.Headers.ReqID, "记账失败：AI 提取账单失败")
		return
	}

	// 3. 追加写入 Beancount 文件
	if err := ledger.AppendBatchTransactions(ws.ledgerCfg.FilePath, *batch, reqCtx); err != nil {
		log.Printf("[ERROR] 追加写入账单文件失败: %v\n", err)
		ws.respondMsg(frame.Headers.ReqID, fmt.Sprintf("保存账单失败: %v", err))
		return
	}

	replySummary := batch.ToSummaryString()
	ws.respondMsg(frame.Headers.ReqID, replySummary)
}

func (ws *WSClient) respondMsg(reqID string, replyText string) {
	respBody := RespondMsgBody{
		MsgType: "text",
	}

	respBody.Text.Content = replyText
	bodyBytes, _ := json.Marshal(respBody)

	frame := WSFrame{
		Cmd: "aibot_msg_callback",
		Headers: WSHeader{
			ReqID: reqID,
		},
		Body: string(bodyBytes),
	}

	if err := ws.conn.WriteJSON(frame); err != nil {
		log.Printf("[ERROR] 回传 WSS 消息失败: %v\n", err)
	}
}
