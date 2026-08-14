// internal/plugin/wecom/handler.go
package wecom

import (
	"context"
	"fmt"
	"log/slog"
	"tallymind/internal/handler"
	"tallymind/internal/llm"
)

type WeComHandler struct {
	txHandler *handler.TransactionHandler
	llmClient *llm.Client
	client    *Client
}

func NewWeComHandler(txHandler *handler.TransactionHandler, llmClient *llm.Client, client *Client) *WeComHandler {
	return &WeComHandler{
		txHandler: txHandler,
		llmClient: llmClient,
		client:    client,
	}
}

// HandleMessage 处理解密后的用户消息 (在 callback.go 的异步协程中运行)

func (h *WeComHandler) HandleMessage(ctx context.Context, msg *PlainXMLMsg) {
	userID := msg.FromUserName
	var userText string
	var attachments []llm.Attachment

	// 1. 根据企微的 MsgType 转换输入参数 (分流)
	switch msg.MsgType {
	case "text":
		userText = msg.Content
		slog.InfoContext(ctx, "[WeCom] 收到文本记账消息", "userID", userID, "text", userText)
	case "image":
		slog.InfoContext(ctx, "[WeCom] 收到图片记账消息", "userID", userID, "picURL", msg.PicURL)
		attachments = append(attachments, llm.Attachment{
			Type: "image_url",
			URL:  msg.PicURL,
		})
	default:
		slog.WarnContext(ctx, "[WeCom] 收到暂不支持的消息类型", "userID", userID, "msgType", msg.MsgType)
		_ = h.client.SendMessage(ctx, NewMarkdownMessage(userID, fmt.Sprintf("⚠️ 暂不支持 [%s] 类型消息记账", msg.MsgType)))
		return
	}

	// 2. 调用通用的 LLM 识图/文本记账管道
	batch, err := h.llmClient.ParseTransaction(ctx, userText, attachments...)
	if err != nil {
		slog.ErrorContext(ctx, "[WeCom] LLM 解析失败", "userID", userID, "err", err)
		_ = h.client.SendMessage(ctx, NewMarkdownMessage(userID, "❌ **记账识别失败**\n> "+err.Error()))
		return
	}
	if batch == nil || len(batch.Transactions) == 0 {
		slog.WarnContext(ctx, "[WeCom] 消息中未识别出有效交易", "userID", userID)
		_ = h.client.SendMessage(ctx, NewMarkdownMessage(userID, "⚠️ **未识别出有效账单**\n> 请输入具体的消费信息或清晰的小票截图。"))
		return
	}
	// 3. 调 transaction 包存盘，拿到摘要
	summary, err := h.txHandler.SaveBatch(ctx, userID, "wecom_plugin", batch)
	if err != nil {
		slog.ErrorContext(ctx, "[WeCom] 账单存盘失败", "userID", userID, "err", err)
		_ = h.client.SendMessage(ctx, NewMarkdownMessage(userID, "❌ **账单保存失败**\n> "+err.Error()))
		return
	}
	// C. 推送企微卡片
	h.sendSuccessCard(ctx, userID, summary)
}

func (h *WeComHandler) sendSuccessCard(ctx context.Context, userID string, summary string) {
	card := &TemplateCardContent{
		CardType: "text_notice",
	}
	card.MainTitle.Title = "✅ **记账成功**\n> "
	card.MainTitle.Desc = summary

	card.CardAction.Type = "1"
	card.CardAction.URL = ""

	if err := h.client.SendMessage(ctx, NewTemplateCardMessage(userID, card)); err != nil {
		slog.ErrorContext(ctx, "[WeCom] 推送卡片失败", "userID", userID, "err", err)
	} else {
		slog.InfoContext(ctx, "[WeCom] 推送卡片成功", "userID", userID)
	}
}
