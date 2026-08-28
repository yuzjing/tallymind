// internal/ledger/types.go
package ledger

import "time"

// Config 账本领域专属配置
type Config struct {
	FilePath         string              `yaml:"file_path"`
	DefaultCurrency  string              `yaml:"default_currency"`
	DefaultReporter  string              `yaml:"default_reporter"`
	FallbackCategory string              `yaml:"fallback_category"`
	FallbackAccount  string              `yaml:"fallback_account"`
	FallbackPayee    string              `yaml:"fallback_payee"`
	Members          map[string][]string `yaml:"members"`
}

// RequestContext 传输层与系统上下文
type RequestContext struct {
	UserID        string            `json:"user_id"`
	SourceChannel string            `json:"source_channel"`
	MessageID     string            `json:"message_id,omitempty"`
	MessageTime   time.Time         `json:"message_time,omitempty"`
	Location      string            `json:"location,omitempty"`
	ChatType      string            `json:"chat_type,omitempty"`
	Extra         map[string]string `json:"extra,omitempty"`
}

// Metadata 扩展元数据 (包含固定字段 + Extra 动态扩展字典)
type Metadata struct {
	Reporter       string            `json:"reporter,omitempty"`        // 谁发的消息
	Owner          string            `json:"owner,omitempty"`           // 实际出资归属人 (如 zhaozhao / jingjing)
	Link           string            `json:"link,omitempty"`            // 关联单号 (写为 ^msg-12345)
	Time           string            `json:"time,omitempty"`            // 交易时间
	Location       string            `json:"location,omitempty"`        // 地点
	Beneficiary    string            `json:"beneficiary,omitempty"`     // 受益人 (如 runrun / parents)
	InvoiceStatus  string            `json:"invoice_status,omitempty"`  // 发票状态
	SourceChannel  string            `json:"source_channel,omitempty"`  // 渠道
	OriginalAmount string            `json:"original_amount,omitempty"` // 优惠前原价
	DiscountAmount string            `json:"discount_amount,omitempty"` // 优惠金额
	Extra          map[string]string `json:"extra,omitempty"`           // 动态扩展字典
}

// Transaction 核心交易结构体
type Transaction struct {
	Amount    float64   `json:"amount" validate:"required,gt=0"`
	Date      string    `json:"date" validate:"omitempty,datetime=2006-01-02"`
	Payee     string    `json:"payee" validate:"omitempty,max=500"`
	Narration string    `json:"narration" validate:"omitempty,max=1000"`
	Category  string    `json:"category"`
	Account   string    `json:"account"`
	Currency  string    `json:"currency,omitempty"`
	Type      string    `json:"type,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Meta      *Metadata `json:"metadata,omitempty"`
	Flag      string    `json:"flag,omitempty"`
}

// BalanceAssertion 资产断言与自动平账实体
type BalanceAssertion struct {
	Date       string  `json:"date"`                  // 对账日期 (YYYY-MM-DD)
	Account    string  `json:"account"`               // 目标资产/负债账户 (如 Assets:WeChat:Wallet)
	Amount     float64 `json:"amount"`                // 真实余额
	Currency   string  `json:"currency"`              // 货币
	Owner      string  `json:"owner,omitempty"`       // 账户所有人 (如 zhaozhao, jingjing)
	AutoPad    bool    `json:"auto_pad,omitempty"`    // 是否开启自动平账 (pad)
	PadAccount string  `json:"pad_account,omitempty"` // 自动平账差额归集科目
}

// BatchTransactions 批量交易与断言请求体
type BatchTransactions struct {
	Transactions      []Transaction      `json:"transactions" validate:"required,divby=1"`
	BalanceAssertions []BalanceAssertion `json:"balance_assertions,omitempty"`
}

// TransactionView 单笔交易的清洗展示视图
type TransactionView struct {
	Date          string  `json:"date"`
	Payee         string  `json:"payee"`
	Narration     string  `json:"narration"`
	Category      string  `json:"category"`
	ShortCategory string  `json:"short_category"`
	Account       string  `json:"account"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	DisplayName   string  `json:"display_name"`
}

// BatchSummary 整批交易的结构化统计摘要
type BatchSummary struct {
	Count       int               `json:"count"`
	TotalAmount float64           `json:"total_amount"`
	Currency    string            `json:"currency"`
	IsSingle    bool              `json:"is_single"`
	Reporter    string            `json:"reporter"`
	SenderID    string            `json:"sender_id"`
	Items       []TransactionView `json:"items"`
	FirstItem   TransactionView   `json:"first_item"`
}