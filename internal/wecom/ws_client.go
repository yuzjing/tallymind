// internal/wecom/ws_client.go
package wecom

import (
	"encoding/json"
	"fmt"
	"net/http"
	"github.com/gin-gonic/gin"
	"tallymind/internal/config"
	"tallymind/internal/ledger"
)

type WSHeader struct {
	ReqID string `json:"req_id"`
}
type WSFrame struct {
	Cmd string `json:"cmd"`
	Headers WSHeader `json:"headers"`
	ErrCode int `json:"err_code,omitempty"`
	ErrMsg string `json:"err_msg,omitempty"`
	Body string `json:"body,omitempty"`
}

//1.鉴权订阅请求body

type SubscribeBody truct {
	BotID string `json:"bot_id"`
	BotSecret string `json:"bot_secret"`
}

//2. 消息回调body

type MsgCallbackBody struct {
	MsgID string `json:"msg_id"`
	AIBotID string `json:"aibot_id"`
	ChatType string `json:"chat_type"`
	From struct{
		UserID string `json:"user_id"`
	}`json:"from"`
	MsgType string `json:"msg_type"`
	Text struct{
		Content string `json:"content"`
	}`json:"text"`
}


//3.回复消息body

type RespondMsgBody struct {
	MsgType string `json:"msg_type"`
	Text struct {
		Content string `json:"content"`
	} `json:"text"`
}
 
type WSClient struct {
	cfg *config.Config
	llmClient *llm.Client
	conn      *websocket.Conn
}

func NewWSClient(cfg *config.Config, llmClient *llm.Client) *WSClient {
	return &WSClient{
		cfg: cfg,
		llmClient: llmClient,
	}
}

//Start 启动长连接：鉴权订阅 -> 启动心跳协程 -> 消息监听循环

func (ws *WSClient) Start(ctx *gin.Context) error {



