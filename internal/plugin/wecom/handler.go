// internal/plugin/wecom/handler.go
package wecom

import (
	"context"
	"fmt"
	"log/slog"

	"tallymind/internal/cron"
	"tallymind/internal/reporter"
	"tallymind/internal/service"

	"tallymind/internal/template"
	"time"
)

type WeComHandler struct {
	client          *Client
	accountService  *service.AccountingService
	templateDir     string
	publicURL       string
	successTemplate string
	failureTemplate string
	reportTemplate  string
}

func NewWeComHandler(
	accountService *service.AccountingService, client *Client, templateDir, successTemplate, failureTemplate, reportTemplate string) *WeComHandler {
	return &WeComHandler{
		accountService:  accountService,
		client:          client,
		templateDir:     templateDir,
		successTemplate: successTemplate,
		failureTemplate: failureTemplate,
		reportTemplate:  reportTemplate,
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

	msgTime := time.Unix(msg.CreateTime, 0)
	if msg.CreateTime == 0 {
		msgTime = time.Now()
	}

	// 2. 组装输入并调用核心服务

	input := service.AccountingInput{
		UserID:        userID,
		SourceChannel: "wecom_plugin",
		MessageID:     fmt.Sprintf("%d", msg.MsgID),
		MessageTime:   msgTime,
		Location:      msg.Label,
		UserText:      userText,
		Attachments:   attachments,
	}

	replyData, err := h.accountService.Process(ctx, input)
	if err != nil {
		slog.ErrorContext(ctx, "[WeCom] 记账处理失败", "userID", userID, "err", err)
		_ = h.client.SendMessage(ctx, NewTextMessage(userID, "❌ **记账失败**\n> "+err.Error()))
		return
	}

	// 3. 渲染微信模板并发送卡片
	cardMsg, err := template.Render[MessageRequest](h.templateDir, h.successTemplate, replyData)
	if err != nil {
		slog.ErrorContext(ctx, "[WeCom] 渲染模板失败", "err", err)
		return
	}

	cardMsg.ToUser = userID
	if err := h.client.SendMessage(ctx, cardMsg); err != nil {
		slog.ErrorContext(ctx, "[WeCom] 发送消息失败", "userID", userID, "err", err)
		return
	}
	slog.InfoContext(ctx, "[WeCom] 记账成功并回复", "userID", userID)

}

// parseIncomingMessage 私有提取器：专门将企微特有消息映射为 LLM 通用格式 (图/文/音/档)
func (h *WeComHandler) parseIncomingMessage(ctx context.Context, msg *PlainXMLMsg) (string, []service.Attachment, error) {
	var userText string
	var attachments []service.Attachment

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
			attachments = append(attachments, service.Attachment{Type: "image_url", URL: imgURL})
		}

	// 微信语音记账通道！
	case "voice":
		if msg.MediaID == "" {
			return "", nil, fmt.Errorf("语音消息缺少 MediaId")
		}
		voiceURL, err := h.client.GetMediaURL(ctx, msg.MediaID)
		if err != nil {
			return "", nil, fmt.Errorf("获取语音下载链接失败: %w", err)
		}
		attachments = append(attachments, service.Attachment{Type: "audio", URL: voiceURL})

	// PDF 电子发票通道！
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
		attachments = append(attachments, service.Attachment{Type: "document", URL: fileURL})

	case "location":
		userText = fmt.Sprintf("用户在位置打卡记账：%s (坐标: %s, %s)", msg.Label, msg.LocationX, msg.LocationY)

	default:
		return "", nil, fmt.Errorf("暂不支持 [%s] 类型消息", msg.MsgType)
	}

	return userText, attachments, nil
}

// RegisterCron 将企业微信的周报与月报分发器挂载到调度引擎中
func (h *WeComHandler) RegisterCron(scheduler *cron.Scheduler) {
	// 1. 注册周报推送卡片
	scheduler.OnReport("weekly", func(ctx context.Context, data *reporter.PeriodicReportData) error {
		cardMsg, err := template.Render[MessageRequest](h.templateDir, "wecom/weekly_report.yaml", data)
		if err != nil {
			return err
		}
		return h.client.SendMessage(ctx, cardMsg)
	})

	// 2. 注册月报推送卡片 (复用或使用独立模板)
	scheduler.OnReport("monthly", func(ctx context.Context, data *reporter.PeriodicReportData) error {
		cardMsg, err := template.Render[MessageRequest](h.templateDir, h.reportTemplate, data)
		if err != nil {
			return err
		}
		return h.client.SendMessage(ctx, cardMsg)
	})
}
