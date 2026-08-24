// internal/reporter/types.go
package reporter

import (
	"tallymind/internal/ledger"
)

// TransactionItem 单笔明细的纯数据清洗视图
type TransactionItem struct {
	Date          string  `json:"date"`
	Payee         string  `json:"payee"`
	Narration     string  `json:"narration"`
	Category      string  `json:"category"`
	ShortCategory string  `json:"short_category"` // 末级科目简称 (如 Dining)
	Account       string  `json:"account"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	DisplayName   string  `json:"display_name"` // 商户优先，备注兜底
	Reporter      string  `json:"reporter,omitempty"`
	Owner         string  `json:"owner,omitempty"`
}

// ReplyData 即时记账回执数据模型 (纯数据字段，零 UI 格式化字符串！)
type ReplyData struct {
	Count           int                       `json:"count"`
	TotalAmount     float64                   `json:"total_amount"`
	Currency        string                    `json:"currency"`
	IsSingle        bool                      `json:"is_single"`
	PrimaryName     string                    `json:"primary_name"`     // 主展示名称 (单笔时为商户，多笔时为首笔商户)
	PrimaryCategory string                    `json:"primary_category"` // 主分类简称
	Items           []TransactionItem         `json:"items"`            // 全量明细列表
	FirstItem       TransactionItem           `json:"first_item"`       // 单笔快捷访问
	JumpURL         string                    `json:"jump_url,omitempty"`
	ImageURL        string                    `json:"image_url,omitempty"`
	Batch           *ledger.BatchTransactions `json:"batch"`
}

// WeeklyReportData 周报纯数据统计模型
type WeeklyReportData struct {
	DateRange     string         `json:"date_range"`
	TotalExpense  float64        `json:"total_expense"`
	TotalIncome   float64        `json:"total_income"`
	NetSavings    float64        `json:"net_savings"`
	Count         int            `json:"count"`
	CategoryStats []CategoryStat `json:"category_stats"`
	TopExpenses   []ExpenseItem  `json:"top_expenses"`
	JumpURL       string         `json:"jump_url,omitempty"`
}

type CategoryStat struct {
	Category      string  `json:"category"`
	ShortCategory string  `json:"short_category"`
	Amount        float64 `json:"amount"`
	Percentage    float64 `json:"percentage"`
}

type ExpenseItem struct {
	Date        string  `json:"date"`
	DisplayName string  `json:"display_name"`
	Category    string  `json:"category"`
	Amount      float64 `json:"amount"`
}

// PeriodicReportData 周期性报表模型 (周报 / 月报共用)
type PeriodicReportData struct {
	PeriodType        string         `json:"period_type"` // "weekly" 或 "monthly"
	Title             string         `json:"title"`
	DateRange         string         `json:"date_range"`
	TotalExpense      float64        `json:"total_expense"`
	TotalIncome       float64        `json:"total_income"`
	NetSavings        float64        `json:"net_savings"` // 净储蓄 (收入 - 支出)
	TransactionCount  int            `json:"transaction_count"`
	CategoryBreakdown []CategoryStat `json:"category_breakdown"` // 分类开销排行榜
	TopExpenses       []ExpenseItem  `json:"top_expenses"`       // 最大几笔支出
	JumpURL           string         `json:"jump_url,omitempty"`
}
