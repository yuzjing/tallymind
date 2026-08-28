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

// EnsureDefaults 单笔交易降级与默认值注入
func (t *Transaction) EnsureDefaults(cfg Config, ctx RequestContext) {
	if t.Meta == nil {
		t.Meta = &Metadata{}
	}

	// 1. 身份归属 (Reporter 优先取元数据，其次取通信上下文，最后取系统默认)
	t.Meta.Reporter = cmp.Or(t.Meta.Reporter, ctx.UserID, cfg.DefaultReporter, "User")
	t.Meta.Owner = cmp.Or(t.Meta.Owner, t.Meta.Reporter)

	// 2. 交易日期 (YYYY-MM-DD)
	defaultDate := time.Now().Format("2006-01-02")
	if !ctx.MessageTime.IsZero() {
		defaultDate = ctx.MessageTime.Format("2006-01-02")
	}
	t.Date = cmp.Or(t.Date, defaultDate)

	// 3. 币种与商户保底
	t.Currency = cmp.Or(t.Currency, cfg.DefaultCurrency, "CNY")
	t.Payee = cmp.Or(t.Payee, cfg.FallbackPayee, "日常消费")

	// 4. 自动关联消息单号
	if t.Meta.Link == "" && ctx.MessageID != "" {
		t.Meta.Link = fmt.Sprintf("msg-%s", ctx.MessageID)
	}

	// 5. 继承地点与渠道
	if t.Meta.Location == "" && ctx.Location != "" {
		t.Meta.Location = ctx.Location
	}
	t.Meta.SourceChannel = cmp.Or(t.Meta.SourceChannel, ctx.SourceChannel)

	// 6. 继承 Extra 自定义字典
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

	// 7. 科目检查与保底
	if !isValidCategory(t.Category) || t.Category == "" {
		slog.Debug("科目格式不合规，自动降级", "raw_category", t.Category, "fallback", cfg.FallbackCategory)
		t.Category = cfg.FallbackCategory
		needsReview = true
	}

	// 8. 账户检查与保底
	if !isValidAccount(t.Account) || t.Account == "" {
		t.Account = cfg.FallbackAccount
	}

	// ⭐️ 9. 核心注入：将资产/负债账户转换为流派 2 (Assets:zhaozhao:WeChat:Wallet)
	t.Account = formatAccountWithOwner(t.Account, t.Meta.Owner)

	// 10. 标记与标签处理
	if needsReview && !slices.Contains(t.Tags, "#needs-review") {
		t.Tags = append(t.Tags, "#needs-review")
	}
	if t.Flag == "" {
		t.Flag = "*"
	}
}

// EnsureDefaults 单笔资产断言与自动找平的默认值注入
func (b *BalanceAssertion) EnsureDefaults(cfg Config, ctx RequestContext) {
	if b.Date == "" {
		if !ctx.MessageTime.IsZero() {
			b.Date = ctx.MessageTime.Format("2006-01-02")
		} else {
			b.Date = time.Now().Format("2006-01-02")
		}
	}
	b.Currency = cmp.Or(b.Currency, cfg.DefaultCurrency, "CNY")
	b.Owner = cmp.Or(b.Owner, ctx.UserID, cfg.DefaultReporter, "User")
	
	// ⭐️ 核心注入：断言账户同样转换为流派 2
	b.Account = formatAccountWithOwner(b.Account, b.Owner)
}

// EnsureDefaults 批量交易与断言统一默认值注入
func (b *BatchTransactions) EnsureDefaults(cfg Config, ctx RequestContext) {
	for i := range b.Transactions {
		b.Transactions[i].EnsureDefaults(cfg, ctx)
	}
	for i := range b.BalanceAssertions {
		b.BalanceAssertions[i].EnsureDefaults(cfg, ctx)
	}
}

// isValidCategory 检查是否为合法的损益或权益类科目
func isValidCategory(category string) bool {
	return strings.HasPrefix(category, "Expenses:") ||
		strings.HasPrefix(category, "Income:") ||
		strings.HasPrefix(category, "Equity:")
}

// isValidAccount 检查是否为合法的资金流转账户
func isValidAccount(account string) bool {
	return strings.HasPrefix(account, "Assets:") ||
		strings.HasPrefix(account, "Liabilities:")
}