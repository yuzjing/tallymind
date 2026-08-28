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

// ToBeancountFormat 转换为标准 Beancount 纯文本字符串 (使用 fmt.Fprintf 直写内存缓冲区，性能最高)
func (t *Transaction) ToBeancountFormat() string {
	var builder strings.Builder

	// 1. 处理 Tags 与 Link 拼接 (自动规范化 # 标签前缀)
	var tagsBuilder strings.Builder
	for _, tag := range t.Tags {
		cleanTag := strings.TrimSpace(tag)
		if cleanTag != "" {
			if !strings.HasPrefix(cleanTag, "#") {
				tagsBuilder.WriteString(" #" + cleanTag)
			} else {
				tagsBuilder.WriteString(" " + cleanTag)
			}
		}
	}
	tagString := tagsBuilder.String()

	linkString := ""
	if t.Meta != nil && t.Meta.Link != "" {
		linkString = " ^" + strings.TrimPrefix(t.Meta.Link, "^")
	}

	// 2. 备注处理：有备注才拼双引号，无备注直接留空
	narrationPart := ""
	if t.Narration != "" {
		narrationPart = fmt.Sprintf(" \"%s\"", t.Narration)
	}

	// 标记处理 (* 或 !)
	flag := "*"
	if t.Flag != "" {
		flag = t.Flag
	}

	// 首行：2026-08-06 * "商户" "备注" #标签 ^链接
	fmt.Fprintf(&builder, "%s %s \"%s\"%s%s%s\n", t.Date, flag, t.Payee, narrationPart, tagString, linkString)

	// 3. 反射：自动遍历 Metadata 中的所有预设结构体字段 (增加 nil 安全保护)
	if t.Meta != nil {
		metaVal := reflect.ValueOf(*t.Meta)
		metaType := reflect.TypeFor[Metadata]()

		for i := 0; i < metaVal.NumField(); i++ {
			structField := metaType.Field(i)
			fieldVal := metaVal.Field(i)

			// 跳过 Extra 字典和 Link 字段（Link 已在首行 ^link 处理）
			if structField.Name == "Extra" || structField.Name == "Link" {
				continue
			}

			// 从 json tag 提取键名
			jsonTag := structField.Tag.Get("json")
			keyName := strings.Split(jsonTag, ",")[0]
			if keyName == "" || keyName == "-" {
				keyName = strings.ToLower(structField.Name)
			}

			// 直接用 Fprintf 写入 builder，避免生成临时字符串
			if fieldVal.Kind() == reflect.String && fieldVal.String() != "" {
				fmt.Fprintf(&builder, "  %s: \"%s\"\n", keyName, fieldVal.String())
			}
		}

		// 4. 遍历 Extra 动态扩展字典 (直接用 Fprintf 写入 builder)
		for k, v := range t.Meta.Extra {
			if k != "" && v != "" {
				fmt.Fprintf(&builder, "  %s: \"%s\"\n", k, v)
			}
		}
	}

	// 5. 正负号处理
	absAmount := math.Abs(t.Amount)
	categoryAmount := absAmount
	accountAmount := -absAmount
	if t.Type == "refund" || t.Type == "income" {
		categoryAmount = -absAmount
		accountAmount = absAmount
	}

	// 6. 科目借贷行 (对齐输出)
	fmt.Fprintf(&builder, "  %-32s  %8.2f %s\n  %-32s  %8.2f %s\n\n",
		t.Category, categoryAmount, t.Currency,
		t.Account, accountAmount, t.Currency,
	)

	return builder.String()
}

// ToBeancountFormat 格式化输出资产断言与自动找平指令 (pad + balance)
func (b *BalanceAssertion) ToBeancountFormat(cfg Config) string {
	var builder strings.Builder

	currency := b.Currency
	if currency == "" {
		currency = cmp.Or(cfg.DefaultCurrency, "CNY")
	}

	// ⭐️ 核心格式化：统一转为前缀命名空间流派 2 (如 Assets:zhaozhao:WeChat:Wallet)
	targetAccount := formatAccountWithOwner(b.Account, b.Owner)

	// 1. 如果开启了自动平账 (auto_pad)，在断言前一天插入 pad 找平指令
	if b.AutoPad {
		padAcc := cmp.Or(b.PadAccount, cfg.FallbackCategory, "Expenses:Other:Uncategorized")

		// pad 日期取断言日期的前一天
		padDate := b.Date
		if t, err := time.Parse("2006-01-02", b.Date); err == nil {
			padDate = t.AddDate(0, 0, -1).Format("2006-01-02")
		}

		fmt.Fprintf(&builder, "; 🌟 自动找平差额至 %s\n", padAcc)
		fmt.Fprintf(&builder, "%s pad %-32s  %s\n", padDate, targetAccount, padAcc)
	}

	// 2. 格式化 balance 断言行
	fmt.Fprintf(&builder, "%s balance %-32s  %8.2f %s\n\n", b.Date, targetAccount, b.Amount, currency)
	return builder.String()
}

// formatAccountWithOwner 将资产/负债账户转换为现代前缀命名空间流派: Assets:<Owner>:<Channel>
func formatAccountWithOwner(account, owner string) string {
	acc := strings.TrimSpace(account)
	owner = strings.TrimSpace(owner)

	// 1. 只有资产 (Assets) 和负债 (Liabilities) 需要绑定人名，支出与收入不分
	if !strings.HasPrefix(acc, "Assets:") && !strings.HasPrefix(acc, "Liabilities:") {
		return acc
	}

	// 2. 占位待办账户 (如 Assets:Pending:Unknown) 保持原样，不绑定人名
	if strings.Contains(acc, "Pending") || strings.Contains(acc, "Unknown") || owner == "" {
		return acc
	}

	// 3. 拆分根类别: "Assets:WeChat:Wallet" -> root="Assets", rest="WeChat:Wallet"
	parts := strings.SplitN(acc, ":", 2)
	if len(parts) < 2 {
		return acc
	}

	root := parts[0]
	rest := parts[1]

	// 4. 幂等防重：如果已经包含了 owner 前缀，直接返回
	if strings.HasPrefix(rest, owner+":") {
		return acc
	}

	// ⭐️ 核心拼接：Assets:zhaozhao:WeChat:Wallet
	return fmt.Sprintf("%s:%s:%s", root, owner, rest)
}