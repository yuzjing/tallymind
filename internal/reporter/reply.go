// internal/reporter/reply.go
package reporter

import (
	"cmp"
	"fmt"
	"strings"

	"tallymind/internal/ledger"
)

// 统一图标定义
const (
	IconGemini = "✨"  // Gemini 核心智能星芒
	IconIncome = "🎉"  // 收入
	IconRefund = "↩️" // 退款
	IconBullet = "✦"  // 条目装饰符
)

// BuildReplyData 纯函数：提炼领域实体为纯净展示视图模型
func BuildReplyData(batch *ledger.BatchTransactions, jumpURL, imageURL string) ReplyData {
	if batch == nil {
		return ReplyData{}
	}

	// -------------------------------------------------------------
	// 场景 A：常规消费/收入/退款/开户初始化流水
	// -------------------------------------------------------------
	if len(batch.Transactions) > 0 {
		count := len(batch.Transactions)
		var expenseTotal, incomeTotal float64
		var hasExpense, hasIncome bool
		items := make([]TransactionItem, 0, count)

		isGenesis := false

		for _, tx := range batch.Transactions {
			// ⭐️ 核心修正：严格区分现金流向，退款和收入在当次结算中全部计入入账流
			switch tx.Type {
			case "income":
				incomeTotal += tx.Amount
				hasIncome = true
			case "refund":
				incomeTotal += tx.Amount
				hasIncome = true
			default: // expense / transfer
				expenseTotal += tx.Amount
				hasExpense = true
			}

			if strings.HasPrefix(tx.Category, "Equity:") {
				isGenesis = true
			}

			displayName := cmp.Or(tx.Payee, tx.Narration, "日常交易")
			displayCat := formatCategory(tx.Category, 2)
			if isGenesis {
				displayCat = "期初建账"
			}

			items = append(items, TransactionItem{
				Type:            tx.Type,
				Date:            tx.Date,
				Payee:           tx.Payee,
				Narration:       tx.Narration,
				Category:        tx.Category,
				DisplayCategory: displayCat,
				Account:         tx.Account,
				Amount:          tx.Amount,
				Currency:        cmp.Or(tx.Currency, "CNY"),
				DisplayName:     displayName,
				Reporter:        cmp.Or(tx.Meta.Reporter, "User"),
				Owner:           cmp.Or(tx.Meta.Owner, "User"),
				Beneficiary:     tx.Meta.Beneficiary,
				SourceChannel:   tx.Meta.SourceChannel,
				Link:            tx.Meta.Link,
				Tags:            tx.Tags,
			})
		}

		primaryCategory := items[0].DisplayCategory
		if isGenesis {
			primaryCategory = "期初建账"
		}

		// 主金额判定：纯入账显示总入账，纯支出显示总支出，混合时显示净额
		var totalAmount float64
		if hasIncome && !hasExpense {
			totalAmount = incomeTotal
		} else if hasExpense && !hasIncome {
			totalAmount = expenseTotal
		} else {
			totalAmount = expenseTotal - incomeTotal
		}

		return ReplyData{
			Count:           count,
			TotalAmount:     roundFloat(totalAmount, 2),
			ExpenseTotal:    roundFloat(expenseTotal, 2),
			IncomeTotal:     roundFloat(incomeTotal, 2),
			HasExpense:      hasExpense,
			HasIncome:       hasIncome,
			Currency:        cmp.Or(batch.Transactions[0].Currency, "CNY"),
			IsSingle:        count == 1,
			PrimaryName:     items[0].DisplayName,
			PrimaryCategory: primaryCategory,
			Items:           items,
			FirstItem:       items[0],
			JumpURL:         jumpURL,
			ImageURL:        imageURL,
		}
	}

	// -------------------------------------------------------------
	// 场景 B：资产断言/对账场景
	// -------------------------------------------------------------
	if len(batch.BalanceAssertions) > 0 {
		count := len(batch.BalanceAssertions)
		var totalAmount float64
		items := make([]TransactionItem, 0, count)

		for _, b := range batch.BalanceAssertions {
			totalAmount += b.Amount
			items = append(items, TransactionItem{
				Date:            b.Date,
				Payee:           b.Account,
				DisplayName:     b.Account,
				Category:        "资产对账",
				DisplayCategory: "资产对账",
				Account:         b.Account,
				Amount:          b.Amount,
				Currency:        cmp.Or(b.Currency, "CNY"),
				Reporter:        b.Owner,
				Owner:           b.Owner,
			})
		}

		return ReplyData{
			Count:           count,
			TotalAmount:     roundFloat(totalAmount, 2),
			Currency:        cmp.Or(batch.BalanceAssertions[0].Currency, "CNY"),
			IsSingle:        count == 1,
			PrimaryName:     items[0].Account,
			PrimaryCategory: "资产对账",
			Items:           items,
			FirstItem:       items[0],
			JumpURL:         jumpURL,
			ImageURL:        imageURL,
		}
	}

	return ReplyData{}
}

// SummaryHeadline 极简单行摘要
func (r ReplyData) SummaryHeadline() string {
	// 1. 资产场景
	if r.PrimaryCategory == "期初建账" {
		if r.IsSingle {
			return fmt.Sprintf("%s 资产建账完成：%s 初始金额 ￥%.2f %s", IconGemini, r.PrimaryName, r.TotalAmount, r.Currency)
		}
		return fmt.Sprintf("%s 资产建账完成：共建立 %d 处初始账户，合计 ￥%.2f", IconGemini, r.Count, r.TotalAmount)
	}

	if r.PrimaryCategory == "资产对账" {
		if r.IsSingle {
			return fmt.Sprintf("%s 资产核验完成：%s 当前余额 ￥%.2f %s", IconGemini, r.PrimaryName, r.TotalAmount, r.Currency)
		}
		return fmt.Sprintf("%s 资产核验完成：共核准 %d 处账户余额", IconGemini, r.Count)
	}

	// 2. 流水场景
	if r.Count > 0 {
		if r.IsSingle {
			first := r.FirstItem
			switch first.Type {
			case "income":
				return fmt.Sprintf("%s 已入账：%s +￥%.2f (%s)", IconIncome, r.PrimaryName, first.Amount, r.PrimaryCategory)
			case "refund":
				return fmt.Sprintf("%s 已记退款：%s +￥%.2f (%s)", IconRefund, r.PrimaryName, first.Amount, r.PrimaryCategory)
			default:
				return fmt.Sprintf("%s 记账成功：%s ￥%.2f (%s)", IconGemini, r.PrimaryName, first.Amount, r.PrimaryCategory)
			}
		}

		if r.HasExpense && r.HasIncome {
			return fmt.Sprintf("%s 记账成功：共 %d 笔（支出 ￥%.2f，入账 +￥%.2f）",
				IconGemini, r.Count, r.ExpenseTotal, r.IncomeTotal)
		}
		if r.HasIncome && !r.HasExpense {
			return fmt.Sprintf("%s 收入入账：共 %d 笔入账，合计 +￥%.2f",
				IconIncome, r.Count, r.IncomeTotal)
		}
		return fmt.Sprintf("%s 记账成功：共计入 %d 笔支出，合计 ￥%.2f", IconGemini, r.Count, r.ExpenseTotal)
	}

	return fmt.Sprintf("%s 操作已成功处理", IconGemini)
}
