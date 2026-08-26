// internal/service/accounting.go
package service

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"tallymind/internal/config"
	"tallymind/internal/crypto"
	"tallymind/internal/ledger"
	"tallymind/internal/llm"
	"tallymind/internal/reporter"
)

// BatchTransactions 结构化记账入参 (类型别名：让外层 Handler 无需 import ledger)
type BatchTransactions = ledger.BatchTransactions

// Attachment 业务层通用多模态附件 (图/音/档)
type Attachment = llm.Attachment

// AccountingInput 记账用例的统一输入
type AccountingInput struct {
	// ========================
	// 1. 核心业务荷载 (必须)
	// ========================
	UserText    string       // 用户输入文本
	Attachments []Attachment // 多模态附件列表 (图片/语音/文档)

	// ========================
	// 2. 调用上下文与审计 (全部可选，留空自动智能兜底)
	// ========================
	MessageID     string            // 消息/小票唯一ID (留空自动生成 UUID/时间戳)
	UserID        string            // 记账人 (留空自动取默认记账人)
	MessageTime   time.Time         // 消息时间 (留空自动取当前 time.Now())
	SourceChannel string            // 渠道标识
	Location      string            // 地理位置 (可选)
	ExtraMetadata map[string]string // 渠道自定义扩展键值对 (可选)

}

// AccountingService 记账核心业务管道
type AccountingService struct {
	cfg          *config.Config
	llmClient    *llm.Client
	ledgerConfig ledger.Config
	// 小票内存缓存与锁 (收归业务层管理)
	mu       sync.RWMutex
	receipts map[string]reporter.ReplyData
}

func NewAccountingService(cfg *config.Config, llmClient *llm.Client) *AccountingService {
	return &AccountingService{
		cfg:          cfg,
		llmClient:    llmClient,
		ledgerConfig: cfg.Ledger,
		receipts:     make(map[string]reporter.ReplyData),
	}
}

// Process 核心用例：AI提取 -> 账本存盘 -> 生成签名小票 -> 存入内存
func (s *AccountingService) Process(ctx context.Context, input AccountingInput) (reporter.ReplyData, error) {
	// 1. 将业务层 Attachment 转为 LLM 适配器所需的格式

	llmAttachment := make([]llm.Attachment, len(input.Attachments))
	for i, att := range input.Attachments {
		llmAttachment[i] = llm.Attachment{
			Type:     att.Type,
			URL:      att.URL,
			MimeType: att.MimeType,
		}
	}

	// 2. 调用 LLM 解析多模态账单

	batch, err := s.llmClient.ParseTransaction(ctx, input.UserText, llmAttachment...)
	if err != nil {
		return reporter.ReplyData{}, fmt.Errorf("AI 识别账单失败: %w", err)
	}
	if batch == nil || len(batch.Transactions) == 0 {
		return reporter.ReplyData{}, fmt.Errorf("未识别出有效记账明细")
	}

	// 3. 在 service 内部转换为 ledger 审计上下文并落盘 (外层零感知 ledger)
	reqCtx := ledger.RequestContext{
		UserID:        input.UserID,
		SourceChannel: input.SourceChannel,
		MessageID:     input.MessageID,
		MessageTime:   input.MessageTime,
		Location:      input.Location,
	}
	if err := ledger.AppendBatchTransactions(s.ledgerConfig.FilePath, batch, s.ledgerConfig, reqCtx); err != nil {
		return reporter.ReplyData{}, fmt.Errorf("账本存盘失败: %w", err)
	}

	// 3. 生成小票链接并存入内存供 H5 查看
	jumpURL := s.BuildReceiptURL(input.MessageID)
	previewImage := extractFirstImage(input.Attachments)
	replyData := reporter.BuildReplyData(batch, jumpURL, previewImage)
	s.SaveReceipt(input.MessageID, replyData)
	return replyData, nil

}

// BuildReceiptURL 生成 2 小时安全签名小票链接
func (s *AccountingService) BuildReceiptURL(receiptID string) string {
	token := crypto.GenerateSignedToken(
		s.cfg.App.ReceiptSignSecret,
		receiptID,
		2*time.Hour,
		crypto.DefaultTokenSignLen,
	)
	baseURL := strings.TrimRight(s.cfg.App.PublicURL, "/")
	return fmt.Sprintf("%s/receipt/%s?token=%s", baseURL, receiptID, token)
}

// SaveReceipt 保存小票展示数据 (容量超 50 自动淘汰最老数据)
func (s *AccountingService) SaveReceipt(id string, data reporter.ReplyData) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.receipts) >= 50 {
		for k := range s.receipts {
			delete(s.receipts, k)
			break
		}
	}
	s.receipts[id] = data
}

// GetReceipt 供 H5 控制器读取小票数据并验签

func (s *AccountingService) GetReceipt(id, token string) (reporter.ReplyData, bool) {
	if !crypto.VerifySignedToken(s.cfg.App.ReceiptSignSecret, id, token) {
		return reporter.ReplyData{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.receipts[id]
	return data, ok
}

// extractFirstImage 从通用附件中提取首张图片
func extractFirstImage(atts []Attachment) string {
	for _, a := range atts {
		if a.Type == "image" || a.Type == "image_url" {
			return a.URL
		}
	}
	return ""
}

// RecordDirect 直接结构化记账 (内部自动组装 RequestContext，调用方零感知 ledger)
func (s *AccountingService) RecordDirect(ctx context.Context, userID, sourceChannel string, req *BatchTransactions) (reporter.ReplyData, error) {
	if req == nil || len(req.Transactions) == 0 {
		return reporter.ReplyData{}, fmt.Errorf("交易批次为空，无需存盘")
	}

	// 在 service 内部组装审计上下文并落盘
	reqCtx := ledger.RequestContext{
		UserID:        cmp.Or(userID, s.ledgerConfig.DefaultReporter),
		SourceChannel: cmp.Or(sourceChannel, "rest_api"),
		MessageTime:   time.Now(),
	}

	if err := ledger.AppendBatchTransactions(s.ledgerConfig.FilePath, req, s.ledgerConfig, reqCtx); err != nil {
		return reporter.ReplyData{}, fmt.Errorf("账本存盘失败: %w", err)
	}

	return reporter.BuildReplyData(req, "", ""), nil
}
