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
	"tallymind/internal/config"
	"tallymind/internal/ledger"
	"tallymind/internal/llm"
	"tallymind/internal/notifier"
	"time"

	"tallymind/internal/reporter"

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

//1.鉴权订阅请求body

type SubscribeBody struct {
	BotID     string `json:"bot_id"`
	BotSecret string `json:"secret"`
}

//2. 消息回调body

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
	wecomCfg  config.WeComConfig
	ledgerCfg ledger.Config
	llmClient *llm.Client
	conn      *websocket.Conn
	mu        sync.Mutex
}

// buildWeComRespondBody 将通用的 notifier.Message 转为企微官方数据体
func buildWeComRespondBody(msg notifier.Message) *MessageRequest {
	switch msg.Type {
	case notifier.TypeImage:
		return &MessageRequest{
			MsgType: "image",
			Image:   &MediaContent{MediaID: msg.FilePath}, // 企微上传素材后的 MediaID 或路径
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
		// 如果传的是卡片，将 msg.Data (any) 转为 raw JSON
		if card, ok := msg.Data.(*TemplateCardContent); ok {
			return NewTemplateCardMessage("", card)
		}
		return NewTextMessage("", fmt.Sprintf("收到数据: %v", msg.Data))

	default: // 默认当 Text 发送
		return NewTextMessage("", msg.Content)
	}
}

func NewWSClient(wecomCfg config.WeComConfig, ledgerCfg ledger.Config, llmClient *llm.Client) *WSClient {
	return &WSClient{
		wecomCfg:  wecomCfg,
		ledgerCfg: ledgerCfg,
		llmClient: llmClient,
	}
}

// safeWriteJSON 加锁并发安全写入
func (ws *WSClient) safeWriteJSON(v any) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.conn == nil {
		return fmt.Errorf("websocket 连接未建立")
	}
	// 打印我们发给企微的原始文本
	bytes, _ := json.Marshal(v)
	slog.Debug("WSS 我方发送原始帧", "raw", string(bytes))

	return ws.conn.WriteJSON(v)
}

//Start 启动长连接：鉴权订阅 -> 启动心跳协程 -> 消息监听循环

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
	slog.Info("WSS 长连接鉴权成功，进入 24 小时消息监听状态...")

	// 2. 后台启动 15s 定时 Ping 心跳保活协程
	go ws.startHeartbeat(ctx)

	// 3. 消息读取主循环
	for {
		// ⭐️ 改用 ReadMessage，直接读取原始字节流！
		_, rawBytes, err := ws.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("读取 WSS 原始数据失败: %w", err)
		}

		// ⭐️ 核心黑科技：在控制台 100% 打印企微发过来的原始 JSON 明文！
		slog.Debug("WSS 企微发来原始帧", "raw", string(rawBytes))

		// 尝试解析外壳帧
		var frame WSFrame
		if err := json.Unmarshal(rawBytes, &frame); err != nil {
			slog.Debug("原始数据反序列化为 WSFrame 失败", "raw", err)
			continue
		}

		// 如果企微返回了错误码，直接在控制台高亮打印！
		if frame.ErrCode != 0 {
			slog.Debug("企微官方报错", "错误码", frame.ErrCode, "错误信息", frame.ErrMsg)
		}

		// 处理消息帧
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

				slog.Debug("WSS 心跳包发送失败", "err", err) // 重连失败，继续等待下一次心跳
				return
			}
		}

	}
}

func (ws *WSClient) handleFrame(ctx context.Context, frame WSFrame) {
	// 只处理企微推过来的家人消息回调 (aibot_msg_callback)
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

	slog.Debug("WSS 收到企微消息回调", "用户ID", userID, "消息内容", userText)

	// 1. 组装 RequestContext
	reqCtx := ledger.RequestContext{
		UserID:        userID,
		SourceChannel: "wecom_wss",
	}

	// 2. 调用 LLM 客户端解析自然语言 -> BatchTransactions 结构体
	var batch *ledger.BatchTransactions
	// ⭐️ 如果大模型未初始化 (ENABLE_LLM=false)，自动开启 Mock 假数据测试！
	if ws.llmClient == nil {

		slog.Info("ENABLE_LLM=false，使用 MVP Mock 数据测试长连接记账全流程...")

		batch = &ledger.BatchTransactions{
			Transactions: []ledger.Transaction{
				{
					Date:      time.Now().Format("2006-01-02"),
					Payee:     "测试商户",
					Narration: fmt.Sprintf("微信接收文本: %s", userText),
					Category:  "Expenses:Food:Groceries",
					Account:   "Assets:WeChat:Husband",
					Amount:    35.0,
					Currency:  "CNY",
				},
			},
		}
	} else {
		// 正常调用大模型解析
		slog.Info("🤖 正在调用 DeepSeek 大模型提取账单...")
		var err error
		batch, err = ws.llmClient.ParseTransaction(ctx, userText)
		if err != nil {

			slog.Error("大模型解析失败", "err", err) //
			ws.respondMsg(frame.Headers.ReqID, msgBody.ResponseURL, notifier.Message{
				Type:    notifier.TypeText,
				Content: fmt.Sprintf("AI 解析失败: %v", err),
			})
			return
		}
	}

	// 3. 追加写入 Beancount 文件
	if err := ledger.AppendBatchTransactions(ws.ledgerCfg.FilePath, batch, ws.ledgerCfg, reqCtx); err != nil {
		slog.Error("追加写入账单文件失败", "err", err)
		ws.respondMsg(frame.Headers.ReqID, msgBody.ResponseURL, notifier.Message{
			Type:    notifier.TypeText,
			Content: fmt.Sprintf("保存账单失败: %v", err),
		})
		return
	}

	replyData := reporter.BuildReplyData(batch, "", "")

	var replyText string
	if replyData.IsSingle {
		replyText = fmt.Sprintf("👌 已记好：%s ￥%.2f (%s)", replyData.PrimaryName, replyData.FirstItem.Amount, replyData.PrimaryCategory)
	} else {
		replyText = fmt.Sprintf("👌 已记好：共计入 %d 笔消费，合计 ￥%.2f", replyData.Count, replyData.TotalAmount)
	}

	// 5. 原汁原味调用你原本的 ws.respondMsg！
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
	if err != nil {
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
					slog.Debug("企微 response_url 快捷回复成功 ", "code", resp.StatusCode)
				}
			}
		}()
	}
}
