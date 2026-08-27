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

// BatchTransactions 结构化账单批次 (映射底层领域实体，屏蔽外层导入)
type BatchTransactions = ledger.BatchTransactions

// Attachment 通用多模态附件定义 (支持图片、文档、音频等各类媒体)
type Attachment struct {
	Type     string `json:"type"`                // 附件类型: "image_url", "audio", "document"
	URL      string `json:"url"`                 // 资源直链、DataURI 或本地路径
	MimeType string `json:"mime_type,omitempty"` // 媒体 MIME 类型 (可选)
}

// AccountingInput 统一记账输入载体 (通用数据传输对象 DTO，与具体渠道完全解耦)
type AccountingInput struct {
	// 1. 调用上下文与审计追踪元数据
	UserID        string            // 操作人 / 发信人标识
	SourceChannel string            // 入站渠道来源 (如 "wecom", "telegram", "rest_api")
	MessageID     string            // 消息或单据唯一标识 (用于幂等与小票溯源)
	MessageTime   time.Time         // 消息产生时间
	Location      string            // 地理位置信息 (可选)
	ExtraMetadata map[string]string // 渠道扩展元数据 (可选)

	// 2. 核心业务荷载
	UserText    string       // 用户输入的自然语言或附加说明
	Attachments []Attachment // 多模态附件列表
}

// AccountingService 核心记账业务应用服务 (系统主业务管道门面)
type AccountingService struct {
	cfg          *config.Config
	llmClient    *llm.Client
	ledgerConfig ledger.Config

	// 内存小票临时状态缓存 (并发安全)
	mu       sync.RWMutex
	receipts map[string]reporter.ReplyData
}

// NewAccountingService 实例化记账核心应用服务
func NewAccountingService(cfg *config.Config, llmClient *llm.Client) *AccountingService {
	return &AccountingService{
		cfg:          cfg,
		llmClient:    llmClient,
		ledgerConfig: cfg.Ledger,
		receipts:     make(map[string]reporter.ReplyData),
	}
}

// Process 多模态智能记账用例：AI 结构化解析 ➔ 实体反查与双重保底 ➔ 账本落盘 ➔ 生成小票并暂存
func (s *AccountingService) Process(ctx context.Context, in AccountingInput) (reporter.ReplyData, error) {
	// 1. 防腐层适配：将通用业务 Attachment 转换为大模型网关所需的 DTO 结构
	llmAttachments := make([]llm.Attachment, len(in.Attachments))
	for i, att := range in.Attachments {
		llmAttachments[i] = llm.Attachment{
			Type:     att.Type,
			URL:      att.URL,
			MimeType: att.MimeType,
		}
	}

	// 2. 身份归一化与业务上下文提取 (从 in.UserID 反查出标准 Key，如 ZiYuZhao -> zhaozhao)
	currentActor := resolveActor(in.UserID, s.ledgerConfig.Members, s.ledgerConfig.DefaultReporter)
	contextHints := formatContextHints(currentActor, s.ledgerConfig.Members)

	// 3. 调用大模型进行多模态语义解析
	batch, err := s.llmClient.ParseTransaction(ctx, in.UserText, contextHints, llmAttachments...)
	if err != nil {
		return reporter.ReplyData{}, fmt.Errorf("AI 解析账单失败: %w", err)
	}
	if batch == nil || len(batch.Transactions) == 0 {
		return reporter.ReplyData{}, fmt.Errorf("未识别出有效记账明细")
	}

	// ⭐️ 3.5 智能实体反查与双重保底 (彻底消除 husband、member_a 或空值漏洞)
	for i := range batch.Transactions {
		tx := &batch.Transactions[i]

		// 记账人 (reporter) 强制由通信协议层判定
		tx.Meta.Reporter = currentActor

		// 出资人 (owner)：空值保底为发信人；有值则反查字典 (表外自由推断如 parents 原样保留)
		if strings.TrimSpace(tx.Meta.Owner) == "" {
			tx.Meta.Owner = currentActor
		} else {
			tx.Meta.Owner = resolveActor(tx.Meta.Owner, s.ledgerConfig.Members, currentActor)
		}

		// 受益人 (beneficiary)：空值保底为发信人；有值则反查字典
		if strings.TrimSpace(tx.Meta.Beneficiary) == "" {
			tx.Meta.Beneficiary = currentActor
		} else {
			tx.Meta.Beneficiary = resolveActor(tx.Meta.Beneficiary, s.ledgerConfig.Members, currentActor)
		}
	}

	// 4. 组装审计上下文并持久化写入底层账本
	reqCtx := ledger.RequestContext{
		UserID:        currentActor,
		SourceChannel: cmp.Or(in.SourceChannel, "api"),
		MessageID:     in.MessageID,
		MessageTime:   in.MessageTime,
		Location:      in.Location,
	}
	if err := ledger.AppendBatchTransactions(s.ledgerConfig.FilePath, batch, s.ledgerConfig, reqCtx); err != nil {
		return reporter.ReplyData{}, fmt.Errorf("账本存盘失败: %w", err)
	}

	// 5. 若具备唯一消息 ID，生成带时效签名的 H5 电子小票并放入缓存
	var jumpURL string
	if in.MessageID != "" {
		jumpURL = s.BuildReceiptURL(in.MessageID)
		previewImage := extractFirstImageURL(in.Attachments)
		replyData := reporter.BuildReplyData(batch, jumpURL, previewImage)
		s.SaveReceipt(in.MessageID, replyData)
		return replyData, nil
	}

	// 免小票追踪场景直接返回基础展示数据
	return reporter.BuildReplyData(batch, "", ""), nil
}

// RecordDirect 直接结构化记账用例 (供标准 REST API 等免 AI 场景直接持久化)
func (s *AccountingService) RecordDirect(ctx context.Context, userID, sourceChannel string, req *BatchTransactions) (reporter.ReplyData, error) {
	if req == nil || len(req.Transactions) == 0 {
		return reporter.ReplyData{}, fmt.Errorf("交易批次为空，无需存盘")
	}

	currentActor := normalizeActor(userID, s.ledgerConfig.Members, s.ledgerConfig.DefaultReporter)
	reqCtx := ledger.RequestContext{
		UserID:        currentActor,
		SourceChannel: cmp.Or(sourceChannel, "rest_api"),
		MessageTime:   time.Now(),
	}

	if err := ledger.AppendBatchTransactions(s.ledgerConfig.FilePath, req, s.ledgerConfig, reqCtx); err != nil {
		return reporter.ReplyData{}, fmt.Errorf("账本存盘失败: %w", err)
	}

	return reporter.BuildReplyData(req, "", ""), nil
}

// BuildReceiptURL 生成带安全签名与时效控制的小票访问 URL
func (s *AccountingService) BuildReceiptURL(id string) string {
	token := crypto.GenerateSignedToken(
		s.cfg.App.ReceiptSignSecret,
		id,
		2*time.Hour,
		crypto.DefaultTokenSignLen,
	)
	baseURL := strings.TrimRight(s.cfg.App.PublicURL, "/")
	return fmt.Sprintf("%s/receipt/%s?token=%s", baseURL, id, token)
}

// SaveReceipt 保存小票展示数据 (基于上限自动淘汰最老数据防内存泄漏)
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

// GetReceipt 供 H5 控制器读取小票数据并执行安全验签
func (s *AccountingService) GetReceipt(id, token string) (reporter.ReplyData, bool) {
	if !crypto.VerifySignedToken(s.cfg.App.ReceiptSignSecret, id, token) {
		return reporter.ReplyData{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.receipts[id]
	return data, ok
}

// normalizeActor 实体身份归一化：根据配置别名表将原始 UserID 映射为标准实体 Key
func normalizeActor(rawID string, members map[string][]string, fallback string) string {
	if rawID == "" {
		return cmp.Or(fallback, "unknown")
	}
	for standardKey, aliases := range members {
		if strings.EqualFold(rawID, standardKey) {
			return standardKey
		}
		for _, alias := range aliases {
			if strings.EqualFold(rawID, alias) {
				return standardKey
			}
		}
	}
	return rawID
}

// formatContextHints 构造注入给大模型的通用业务上下文与实体对照提示词
func formatContextHints(currentActor string, members map[string][]string) string {
	var sb strings.Builder
	sb.WriteString("【业务上下文与实体别名映射】：\n")
	sb.WriteString(fmt.Sprintf("- 当前操作人/发信人 (Reporter): %s\n", currentActor))

	if len(members) > 0 {
		sb.WriteString("- 实体标准Key与别名对照表：\n")
		for standardKey, aliases := range members {
			fmt.Fprintf(&sb, "  • %s: %s\n", standardKey, strings.Join(aliases, ", "))
		}
	}
	return sb.String()
}

// extractFirstImageURL 从多模态附件列表中提取首张可用图片作为快照
func extractFirstImageURL(atts []Attachment) string {
	for _, a := range atts {
		if a.Type == "image" || a.Type == "image_url" {
			return a.URL
		}
	}
	return ""
}

// resolveActor 根据 members 字典反查标准 Key
func resolveActor(rawInput string, members map[string][]string, fallback string) string {
	target := strings.TrimSpace(rawInput)
	if target == "" {
		return cmp.Or(fallback, "unknown")
	}

	// 1. 遍历 members 字典进行全量反查 (支持大小写不敏感匹配)
	for standardKey, aliases := range members {
		if strings.EqualFold(target, standardKey) {
			return standardKey
		}
		for _, alias := range aliases {
			if strings.EqualFold(target, alias) {
				return standardKey
			}
		}
	}

	// 2. 如果字典里没有 (如 AI 自由推断的 "parents", "friends", "colleague")，原样保留
	return target
}

// GetPeriodicReport 业务用例：获取指定周期的财务分析报表
func (s *AccountingService) GetPeriodicReport(ctx context.Context, periodType string, refTime time.Time) (*reporter.PeriodicReportData, error) {
	jumpURL := s.BuildReportURL(periodType)
	return reporter.GeneratePeriodicReport(s.ledgerConfig.FilePath, periodType, refTime, jumpURL)
}

// BuildReportURL 生成带 2 小时时效签名的周期报表安全链接
func (s *AccountingService) BuildReportURL(periodType string) string {
	token := crypto.GenerateSignedToken(
		s.cfg.App.ReceiptSignSecret,
		"report_view",
		2*time.Hour,
		crypto.DefaultTokenSignLen,
	)
	baseURL := strings.TrimRight(s.cfg.App.PublicURL, "/")
	return fmt.Sprintf("%s/report?period=%s&token=%s", baseURL, periodType, token)
}
