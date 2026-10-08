// internal/ledger/format.go
package ledger

import (
	"cmp"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

func (t *Transaction) ToBeancountFormat() string {
	var builder strings.Builder

	var tagsBuilder strings.Builder
	for _, tag := range t.Tags {
		cleanTag := strings.TrimSpace(tag)
		if cleanTag != "" {
			tagsBuilder.WriteByte(' ')
			if !strings.HasPrefix(cleanTag, "#") {
				tagsBuilder.WriteByte('#')
			}
			tagsBuilder.WriteString(cleanTag)
		}
	}
	tagString := tagsBuilder.String()

	linkString := ""
	if t.Meta != nil && t.Meta.Link != "" {
		linkString = " ^" + strings.TrimPrefix(t.Meta.Link, "^")
	}

	narrationPart := ""
	if t.Narration != "" {
		narrationPart = fmt.Sprintf(" \"%s\"", t.Narration)
	}

	flag := "*"
	if t.Flag != "" {
		flag = t.Flag
	}

	fmt.Fprintf(&builder, "%s %s \"%s\"%s%s%s\n", t.Date, flag, t.Payee, narrationPart, tagString, linkString)

	owner := ""
	if t.Meta != nil {
		owner = t.Meta.Owner
		metaVal := reflect.ValueOf(*t.Meta)
		metaType := reflect.TypeFor[Metadata]()

		for i := 0; i < metaVal.NumField(); i++ {
			structField := metaType.Field(i)
			fieldVal := metaVal.Field(i)

			if structField.Name == "Extra" || structField.Name == "Link" {
				continue
			}

			jsonTag := structField.Tag.Get("json")
			keyName := strings.Split(jsonTag, ",")[0]
			if keyName == "" || keyName == "-" {
				keyName = strings.ToLower(structField.Name)
			}

			if fieldVal.Kind() == reflect.String && fieldVal.String() != "" {
				fmt.Fprintf(&builder, "  %s: \"%s\"\n", keyName, fieldVal.String())
			}
		}

		for k, v := range t.Meta.Extra {
			if k != "" && v != "" {
				fmt.Fprintf(&builder, "  %s: \"%s\"\n", k, v)
			}
		}
	}

	// ⭐️ 核心改进 1：统一在格式化层进行 4 层命名空间注入（Account 与 转账 Category 双重保护）
	categoryAccount := formatAccountWithOwner(t.Category, owner)
	targetAccount := formatAccountWithOwner(t.Account, owner)

	absAmount := math.Abs(t.Amount)
	categoryAmount := absAmount
	accountAmount := -absAmount

	if strings.HasPrefix(t.Category, "Equity:") {
		// 负债建账（如信用卡初始欠款）：负债记负，权益对冲记正
		if strings.HasPrefix(t.Account, "Liabilities:") {
			categoryAmount = absAmount
			accountAmount = -absAmount
		} else {
			// 正常资产建账（储蓄卡初始余额）：资产记正，权益对冲记负
			categoryAmount = -absAmount
			accountAmount = absAmount
		}
	} else if t.Type == "refund" || t.Type == "income" {
		categoryAmount = -absAmount
		accountAmount = absAmount
	}
	// 注：当 t.Type == "transfer" (内部转账/还信用卡) 时，走默认分支：
	// categoryAmount = +absAmount (转入方增加), accountAmount = -absAmount (转出方扣减)，符号完全精确！

	fmt.Fprintf(&builder, "  %-32s  %8.2f %s\n  %-32s  %8.2f %s\n\n",
		categoryAccount, categoryAmount, t.Currency,
		targetAccount, accountAmount, t.Currency,
	)

	return builder.String()
}

func (b *BalanceAssertion) ToBeancountFormat(cfg Config) string {
	var builder strings.Builder

	currency := b.Currency
	if currency == "" {
		currency = cmp.Or(cfg.DefaultCurrency, "CNY")
	}

	targetAccount := formatAccountWithOwner(b.Account, b.Owner)

	tolerancePart := ""

	if b.AutoPad {
		tolerancePart = " ~ 0.00"

		padAcc := cmp.Or(b.PadAccount, "Equity:Opening-Balances")
		padDate := b.Date
		if t, err := time.Parse("2006-01-02", b.Date); err == nil {
			padDate = t.AddDate(0, 0, -1).Format("2006-01-02")
		}

		fmt.Fprintf(&builder, "; 🌟 智能自动平账：将差额平衡至 %s\n", padAcc)
		fmt.Fprintf(&builder, "%s pad %-32s  %s\n", padDate, targetAccount, padAcc)
	}

	fmt.Fprintf(&builder, "%s balance %-32s  %8.2f%s %s\n\n",
		b.Date, targetAccount, b.Amount, tolerancePart, currency)
	return builder.String()
}

// formatAccountWithOwner 转换为首字母大写的标准 Beancount 前缀流派: Assets:Zhaozhao:WeChat:Wallet
func formatAccountWithOwner(account, owner string) string {
	acc := strings.TrimSpace(account)
	owner = strings.TrimSpace(owner)

	if !strings.HasPrefix(acc, "Assets:") && !strings.HasPrefix(acc, "Liabilities:") {
		return acc
	}
	if strings.Contains(acc, "Pending") || strings.Contains(acc, "Unknown") || owner == "" {
		return acc
	}

	ownerTitle := capitalize(owner)

	parts := strings.SplitN(acc, ":", 2)
	if len(parts) < 2 {
		return acc
	}
	root, rest := parts[0], parts[1]

	if strings.HasPrefix(strings.ToLower(rest), strings.ToLower(owner)+":") {
		return acc
	}

	return fmt.Sprintf("%s:%s:%s", root, ownerTitle, rest)
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
