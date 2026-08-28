// internal/reporter/types.go
package reporter

// TransactionItem 单笔交易明细视图
type TransactionItem struct {
	Date            string   `json:"date"`
	Payee           string   `json:"payee"`
	Narration       string   `json:"narration"`
	Category        string   `json:"category"`
	DisplayCategory string   `json:"display_category"`
	Account         string   `json:"account"`
	Amount          float64  `json:"amount"`
	Currency        string   `json:"currency"`
	DisplayName     string   `json:"display_name"`             // 商户优先，备注兜底
	Reporter        string   `json:"reporter,omitempty"`       // 记账操作人
	Owner           string   `json:"owner,omitempty"`          // 出资归属人
	Beneficiary     string   `json:"beneficiary,omitempty"`    // 实际受益人
	SourceChannel   string   `json:"source_channel,omitempty"` // 渠道来源 (如 wecom_plugin)
	Link            string   `json:"link,omitempty"`           // 消息溯源 (^msg-xxxx)
	Tags            []string `json:"tags,omitempty"`           // 特征标签列表
}

// ReplyData 即时记账回执视图模型
type ReplyData struct {
	Count           int               `json:"count"`
	TotalAmount     float64           `json:"total_amount"`
	Currency        string            `json:"currency"`
	IsSingle        bool              `json:"is_single"`
	PrimaryName     string            `json:"primary_name"`
	PrimaryCategory string            `json:"primary_category"`
	Items           []TransactionItem `json:"items"`
	FirstItem       TransactionItem   `json:"first_item"`
	JumpURL         string            `json:"jump_url,omitempty"`
	ImageURL        string            `json:"image_url,omitempty"`
}

// CategoryStat 分类/成员开销占比项
type CategoryStat struct {
	Category    string  `json:"category"`     // 完整科目名 (如 Expenses:Food:Lunch)
	DisplayName string  `json:"display_name"` // 分类或成员名 (如 Food > Lunch 或 zhaozhao)
	Amount      float64 `json:"amount"`       // 发生总额
	Percentage  float64 `json:"percentage"`   // 占总支出的百分比 (0~100)
}

// ExpenseItem 单笔大额开销项
type ExpenseItem struct {
	Date        string  `json:"date"`
	DisplayName string  `json:"display_name"`
	Category    string  `json:"category"`
	Amount      float64 `json:"amount"`
}

// TrendItem 趋势项 (按日走势如 "2026-08-26"，按月走势如 "2026-08")
type TrendItem struct {
	Date   string  `json:"date"`   // 日期或月份标签
	Amount float64 `json:"amount"` // 支出金额
}

type PeriodicReportData struct {
	PeriodType string `json:"period_type"`
	Title      string `json:"title"`
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
	DateRange  string `json:"date_range"`
	TargetDate string `json:"target_date"`
	PrevDate   string `json:"prev_date"`
	NextDate   string `json:"next_date"`
	HasNext    bool   `json:"has_next"`
	Token      string `json:"token"`

	TotalExpense float64 `json:"total_expense"`
	TotalIncome  float64 `json:"total_income"`
	NetSavings   float64 `json:"net_savings"`
	SavingsRate  float64 `json:"savings_rate"`

	// ⭐️ 核心新增：全维度对比与差额指标
	HasPrevData       bool    `json:"has_prev_data"`       // 是否存在上期数据
	PrevExpense       float64 `json:"prev_expense"`        // 上期支出
	ExpenseChangeRate float64 `json:"expense_change_rate"` // 支出环比 %
	ExpenseChangeDiff float64 `json:"expense_change_diff"` // 支出增减差额 (本期 - 上期)

	PrevIncome       float64 `json:"prev_income"`        // 上期收入
	IncomeChangeDiff float64 `json:"income_change_diff"` // 收入增减差额

	PrevSavings       float64 `json:"prev_savings"`        // 上期结余
	SavingsChangeDiff float64 `json:"savings_change_diff"` // 结余改善/恶化差额

	TransactionCount  int            `json:"transaction_count"`
	CategoryBreakdown []CategoryStat `json:"category_breakdown"`
	MemberBreakdown   []CategoryStat `json:"member_breakdown"`
	Trends            []TrendItem    `json:"trends"`
	TopExpenses       []ExpenseItem  `json:"top_expenses"`
	JumpURL           string         `json:"jump_url,omitempty"`
}
