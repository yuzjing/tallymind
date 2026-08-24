// internal/ledger/defaults.go
package ledger

import (
	"cmp"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
)

// EnsureDefaults 极简降级兜底 (采用 Go 1.21+ cmp.Or 与 slices.Contains)
func (t *Transaction) EnsureDefaults(cfg Config, ctx RequestContext) {
	// ⭐️ 第一行强效防空指针：如果 Meta 是 nil，立刻给它初始化一个空的 &Metadata{} 结构体！
	if t.Meta == nil {
		t.Meta = &Metadata{}
	}

	// 1. 身份归属
	t.Meta.Reporter = cmp.Or(t.Meta.Reporter, ctx.UserID, "User")
	t.Meta.Owner = cmp.Or(t.Meta.Owner, t.Meta.Reporter)

	// 2. 交易日期 (纯 YYYY-MM-DD，客观真实)
	defaultDate := time.Now().Format("2006-01-02")
	if !ctx.MessageTime.IsZero() {
		defaultDate = ctx.MessageTime.Format("2006-01-02")
	}
	t.Date = cmp.Or(t.Date, defaultDate)

	// 3.自动关联溯源单号 (如 link: "msg-123456789")
	if t.Meta.Link == "" && ctx.MessageID != "" {
		t.Meta.Link = fmt.Sprintf("msg-%s", ctx.MessageID)
	}

	// 4. 自动继承地点信息
	if t.Meta.Location == "" && ctx.Location != "" {
		t.Meta.Location = ctx.Location
	}

	// 5. 继承来自协议的所有自定义 Extra
	if len(ctx.Extra) > 0 {
		if t.Meta.Extra == nil {
			t.Meta.Extra = make(map[string]string)
		}
		for k, v := range ctx.Extra {
			if _, exists := t.Meta.Extra[k]; !exists {
				t.Meta.Extra[k] = v
			}
		}
	}

	needsReview := false

	// 6. 科目检查与降级
	if !isValidCategory(t.Category) {
		slog.Debug("科目格式不合规，自动降级", "raw_category", t.Category, "fallback", cfg.FallbackCategory)
		t.Category = cfg.FallbackCategory
		needsReview = true
	}

	// 7. 账户检查与降级
	if !isValidAccount(t.Account) {
		t.Account = cfg.FallbackAccount
	}

	// 8. 微信上下文元数据填充
	t.Meta.SourceChannel = cmp.Or(t.Meta.SourceChannel, ctx.SourceChannel)
	t.Meta.Reporter = cmp.Or(t.Meta.Reporter, cfg.DefaultReporter)

	// 9. 追加复核标签 (用 slices.Contains 简化，消除多行循环)
	if needsReview && !slices.Contains(t.Tags, "#needs-review") {
		t.Tags = append(t.Tags, "#needs-review")
	}
}

// 负责把盒子里的每笔交易拿出来调上面的方法
// =========================================================================
func (b *BatchTransactions) EnsureDefaults(cfg Config, ctx RequestContext) {
	for i := range b.Transactions {
		// 逐笔调用
		b.Transactions[i].EnsureDefaults(cfg, ctx)
	}
}

// isValidCategory 检查是否为合法的损益类分类科目 (Expenses 或 Income)
func isValidCategory(category string) bool {
	return strings.HasPrefix(category, "Expenses:") ||
		strings.HasPrefix(category, "Income:") ||
		strings.HasPrefix(category, "Equity:")
}

// isValidAccount 检查是否为合法的资金流转账户 (Assets, Liabilities 或 Equity)
func isValidAccount(account string) bool {
	return strings.HasPrefix(account, "Assets:") ||
		strings.HasPrefix(account, "Liabilities:")
}
