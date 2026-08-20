// internal/plugin/wecom/handler.go
package wecom

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"tallymind/internal/handler"
	"tallymind/internal/llm"
	"tallymind/internal/template"
)

type WeComHandler struct {
	txHandler       *handler.TransactionHandler
	llmClient       *llm.Client
	client          *Client
	templateDir     string
	publicURL       string
	successTemplate string
	failureTemplate string
}

func NewWeComHandler(
	txHandler *handler.TransactionHandler, llmClient *llm.Client, client *Client, templateDir, publicURL, successTemplate, failureTemplate string) *WeComHandler {
	return &WeComHandler{
		txHandler:       txHandler,
		llmClient:       llmClient,
		client:          client,
		templateDir:     templateDir,
		publicURL:       publicURL,
		successTemplate: successTemplate,
		failureTemplate: failureTemplate,
	}
}

// HandleMessage 处理解密后的用户消息 (在 callback.go 的异步协程中运行)

func (h *WeComHandler) HandleMessage(ctx context.Context, msg *PlainXMLMsg) {
	userID := msg.FromUserName
	//  1. 统一提取：将企微协议消息转为通用的 (文本, 附件列表)
	userText, attachments, err := h.parseIncomingMessage(ctx, msg)
	if err != nil {
		slog.ErrorContext(ctx, "[WeCom] 收到暂不支持的消息类型", "userID", userID, "msgType", msg.MsgType, "err", err)
		_ = h.client.SendMessage(ctx, NewTextMessage("⚠️ 暂不支持 [%s] 类型消息记账", msg.MsgType))
		return
	}

	// 2. 调用通用的 LLM 记账管道
	batch, err := h.llmClient.ParseTransaction(ctx, userText, attachments...)
	if err != nil {
		slog.ErrorContext(ctx, "[WeCom] LLM 解析失败", "userID", userID, "err", err)
		_ = h.client.SendMessage(ctx, NewTextMessage(userID, "❌ **记账识别失败**\n> "+err.Error()))
		return
	}
	if batch == nil || len(batch.Transactions) == 0 {
		slog.WarnContext(ctx, "[WeCom] 消息中未识别出有效交易", "userID", userID)
		_ = h.client.SendMessage(ctx, NewTextMessage(userID, "⚠️ **未识别出有效账单**\n> 请输入具体的消费信息或清晰的小票截图。"))
		return
	}
	// 3. 调 transaction 包存盘，拿到摘要
	summary, err := h.txHandler.SaveBatch(ctx, userID, "wecom_plugin", batch)
	if err != nil {
		slog.ErrorContext(ctx, "[WeCom] 账单存盘失败", "userID", userID, "err", err)
		_ = h.client.SendMessage(ctx, NewTextMessage(userID, "❌ **账单保存失败**\n> "+err.Error()))
		return
	}
	//4.组装数据，通过外部 YAML 模板渲染出 MessageRequest！

	data := map[string]any{
		"Title":    "记账成功",
		"Summary":  summary,
		"JumpURL":  h.publicURL,
		"ImageURL": msg.PicURL,
	}
	slog.DebugContext(ctx, "Msg", "data", data)

	cardMsg, err := template.Render[MessageRequest](h.templateDir, h.successTemplate, data)
	if err != nil {
		slog.ErrorContext(ctx, "[WeCom] 渲染程工模板失败", "err", err)
		return
	}

	//可删区域，debug用

	jsonBytes, _ := json.Marshal(cardMsg)
	slog.InfoContext(ctx, "👉 最终发给企微的完整请求体", "payload", string(jsonBytes))

	// 5. 补上接收人并发送
	cardMsg.ToUser = userID
	if err := h.client.SendMessage(ctx, cardMsg); err != nil {
		slog.ErrorContext(ctx, "[WeCom] 发送消息失败", "userID", userID, "err", err)
		return
	}
	slog.InfoContext(ctx, "[WeCom] 发送消息成功", "userID", userID)

}

// parseIncomingMessage 私有提取器：专门将企微特有消息映射为 LLM 通用格式 (图/文/音/档)
func (h *WeComHandler) parseIncomingMessage(ctx context.Context, msg *PlainXMLMsg) (string, []llm.Attachment, error) {
	var userText string
	var attachments []llm.Attachment

	switch msg.MsgType {
	case "text":
		userText = msg.Content

	case "image":
		imgURL := msg.PicURL
		if imgURL == "" && msg.MediaID != "" {
			var err error
			imgURL, err = h.client.GetMediaURL(ctx, msg.MediaID)
			if err != nil {
				return "", nil, fmt.Errorf("获取图片素材失败: %w", err)
			}
		}
		if imgURL != "" {
			attachments = append(attachments, llm.Attachment{Type: "image_url", URL: imgURL})
		}

	// 🌟 开启微信语音记账通道！
	case "voice":
		if msg.MediaID == "" {
			return "", nil, fmt.Errorf("语音消息缺少 MediaId")
		}
		voiceURL, err := h.client.GetMediaURL(ctx, msg.MediaID)
		if err != nil {
			return "", nil, fmt.Errorf("获取语音下载链接失败: %w", err)
		}
		attachments = append(attachments, llm.Attachment{Type: "audio", URL: voiceURL})

	// 🌟 开启 PDF 电子发票通道！
	case "file":
		if msg.MediaID == "" {
			return "", nil, fmt.Errorf("文件消息缺少 MediaId")
		}
		fileURL, err := h.client.GetMediaURL(ctx, msg.MediaID)
		if err != nil {
			return "", nil, fmt.Errorf("获取文件下载链接失败: %w", err)
		}
		if msg.Title != "" {
			userText = fmt.Sprintf("发票名称: %s", msg.Title)
		}
		attachments = append(attachments, llm.Attachment{Type: "document", URL: fileURL})

	case "location":
		userText = fmt.Sprintf("用户在位置打卡记账：%s (坐标: %s, %s)", msg.Label, msg.LocationX, msg.LocationY)

	default:
		return "", nil, fmt.Errorf("暂不支持 [%s] 类型消息", msg.MsgType)
	}

	return userText, attachments, nil
}
