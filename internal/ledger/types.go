// internal/ledger/types.go
package ledger

import "time"

// Config 账本领域专属配置 (直接定义在 ledger 内部，消除循环导入！)
type Config struct {
	FilePath         string              // 账本路径 (如 "data/2026.bean")
	DefaultCurrency  string              // 默认货币 (如 "CNY")
	DefaultReporter  string              // 默认记账归属人 (如 "User")
	FallbackCategory string              // 兜底支出分类 (如 "Expenses:Uncategorized")
	FallbackAccount  string              // 兜底资金账户 (如 "Assets:Pending:Unknown")
	FallbackPayee    string              // 兜底商户名 (如 "日常消费")
	Members          map[string][]string // 列表
}

// RequestContext 传输层与系统上下文
type RequestContext struct {
	// ==================== 1. 核心通信身份与渠道 ====================
	UserID        string `json:"user_id"`        // 发送人身份 ID (如 "ZiYuZhao", "12345678")
	SourceChannel string `json:"source_channel"` // 渠道标识 (如 "wecom_plugin", "telegram", "web_api")

	// ==================== 2. 协议自带的高价值客观事实 ====================
	MessageID   string    `json:"message_id,omitempty"`   // 协议唯一消息单号 (如企微 MsgId，用于防重与 link 溯源)
	MessageTime time.Time `json:"message_time,omitempty"` // 消息发送的精确物理时间 (协议自带的 CreateTime 时间戳)
	Location    string    `json:"location,omitempty"`     // 物理地点 (若发送了定位或 Header 带了位置)
	ChatType    string    `json:"chat_type,omitempty"`    // 会话类型: "single"(私聊), "group"(群聊)

	// ==================== 3. 开放式万能扩展槽 ====================
	// 任何未来未知渠道的特殊私有字段，直接塞进 Extra
	Extra map[string]string `json:"extra,omitempty"`
}

// Metadata 扩展元数据 (包含固定字段 + Extra 无限动态扩展字典)
type Metadata struct {
	Reporter       string            `json:"reporter,omitempty"`        // 谁发的消息
	Owner          string            `json:"owner,omitempty"`           // 实际账户控制人 / 账目归属人 (如 husband / wife)
	Link           string            `json:"link,omitempty"`            // 关联单号 (写为 ^order-12345)
	Time           string            `json:"time,omitempty"`            // 交易时间 (如 18:20:05)
	Location       string            `json:"location,omitempty"`        // 地点
	Beneficiary    string            `json:"beneficiary,omitempty"`     // 受益人 (如 family/baby)
	InvoiceStatus  string            `json:"invoice_status,omitempty"`  // 发票状态 (pending/done)
	SourceChannel  string            `json:"source_channel,omitempty"`  // 渠道 (text/voice/image)
	OriginalAmount string            `json:"original_amount,omitempty"` // 优惠前原价
	DiscountAmount string            `json:"discount_amount,omitempty"` // 优惠金额
	Extra          map[string]string `json:"extra,omitempty"`           // 动态扩展字典，支持任意自定义元数据
}

// Transaction 核心交易结构体
type Transaction struct {
	Amount    float64   `json:"amount" validate:"required,gt=0"`               // 金额必填且必须 > 0
	Date      string    `json:"date" validate:"omitempty,datetime=2006-01-02"` // 交易日期 YYYY-MM-DD
	Payee     string    `json:"payee" validate:"omitempty,max=500"`            // 商户名 (上限 500 字符)
	Narration string    `json:"narration" validate:"omitempty,max=1000"`       // 详细备注 (上限 1000 字符，可为空)
	Category  string    `json:"category"`                                      // 支出/收入科目
	Account   string    `json:"account"`                                       // 支付/资产账户
	Currency  string    `json:"currency,omitempty"`                            // 货币 (默认 CNY)
	Type      string    `json:"type,omitempty"`                                // 交易类型: expense / income / refund
	Tags      []string  `json:"tags,omitempty"`                                // 标签 (如 ["#公司报销"])
	Meta      *Metadata `json:"metadata,omitempty"`                            // 扩展元数据
}

// BatchTransactions 批量交易请求体
type BatchTransactions struct {
	Transactions []Transaction `json:"transactions" validate:"required,divby=1"`
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

// BatchSummary 整批交易的结构化统计摘要 (供微信消息与 H5 页面共用)
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


// BalanceAssertion 资产断言与自动平账实体
type BalanceAssertion struct {
	Date        string  `json:"date"`                   // 对账日期 (YYYY-MM-DD)
	Account     string  `json:"account"`                // 目标资产/负债账户 (如 Assets:WeChat:Wallet)
	Amount      float64 `json:"amount"`                 // 真实余额 (当前水位的确切金额)
	Currency    string  `json:"currency"`               // 货币
	Owner       string  `json:"owner,omitempty"`        // 账户所有人 (如 zhaozhao, jingjing)
	AutoPad     bool    `json:"auto_pad,omitempty"`     // 是否开启自动平账 (若为 true 则先写入 pad 找平)
	PadAccount  string  `json:"pad_account,omitempty"`  // 自动平账差额归集科目 (默认为 Expenses:Other:Uncategorized)
}