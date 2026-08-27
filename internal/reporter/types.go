// internal/reporter/types.go
package reporter

// TransactionItem 单笔交易明细视图
type TransactionItem struct {
	Date          string   `json:"date"`
	Payee         string   `json:"payee"`
	Narration     string   `json:"narration"`
	Category      string   `json:"category"`
	ShortCategory string   `json:"short_category"` // 末级科目 (如 Food > Lunch)
	Account       string   `json:"account"`
	Amount        float64  `json:"amount"`
	Currency      string   `json:"currency"`
	DisplayName   string   `json:"display_name"`             // 商户优先，备注兜底
	Reporter      string   `json:"reporter,omitempty"`       // 记账操作人
	Owner         string   `json:"owner,omitempty"`          // 出资归属人
	Beneficiary   string   `json:"beneficiary,omitempty"`    // 实际受益人
	SourceChannel string   `json:"source_channel,omitempty"` // 渠道来源 (如 wecom_plugin)
	Link          string   `json:"link,omitempty"`           // 消息溯源 (^msg-xxxx)
	Tags          []string `json:"tags,omitempty"`           // 特征标签列表
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
	Category      string  `json:"category"`       // 完整科目名 (如 Expenses:Food:Lunch)
	ShortCategory string  `json:"short_category"` // 短分类或成员名 (如 Food > Lunch 或 zhaozhao)
	Amount        float64 `json:"amount"`         // 发生总额
	Percentage    float64 `json:"percentage"`     // 占总支出的百分比 (0~100)
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

// PeriodicReportData 全周期统计通用模型 (周/月/季/年 100% 共用)
type PeriodicReportData struct {
	PeriodType string `json:"period_type"` // "weekly" | "monthly" | "quarterly" | "yearly"
	Title      string `json:"title"`       // 报表标题
	StartDate  string `json:"start_date"`  // 起始日期 "2026-08-01"
	EndDate    string `json:"end_date"`    // 结束日期 "2026-08-31"
	DateRange  string `json:"date_range"`  // 页面展示区间 "08.01 ~ 08.31"

	// 时间导航锚点
	TargetDate string `json:"target_date"` // 当前基准日期 "2026-08-26"
	PrevDate   string `json:"prev_date"`   // 上一周期基准日期 "2026-08-19"
	NextDate   string `json:"next_date"`   // 下一周期基准日期 "2026-09-02"
	HasNext    bool   `json:"has_next"`    // 是否存在下一周期 (未来则为 false)

	TotalExpense      float64        `json:"total_expense"`       // 本期总支出
	TotalIncome       float64        `json:"total_income"`        // 本期总收入
	NetSavings        float64        `json:"net_savings"`         // 本期净结余 (收入 - 支出)
	SavingsRate       float64        `json:"savings_rate"`        // 储蓄率百分比 (0~100)
	PrevExpense       float64        `json:"prev_expense"`        // 上期总支出 (用于环比)
	ExpenseChangeRate float64        `json:"expense_change_rate"` // 支出环比增减率 (负数代表节省)
	TransactionCount  int            `json:"transaction_count"`   // 总交易笔数
	CategoryBreakdown []CategoryStat `json:"category_breakdown"`  // 支出分类排行榜
	MemberBreakdown   []CategoryStat `json:"member_breakdown"`    // 成员开销排行榜
	Trends            []TrendItem    `json:"trends"`              // 统一使用 Trends []TrendItem
	TopExpenses       []ExpenseItem  `json:"top_expenses"`        // 最大几笔大额开销
	JumpURL           string         `json:"jump_url,omitempty"`  // H5 详情跳转链接
}
