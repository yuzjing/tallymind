package reporter

import (
	"cmp"
	"fmt"
	"strings"
	"tallymind/internal/ledger"
)

// ⭐️ 统一图标定义（引入 Gemini 经典 AI 星芒 ✨，保持极简灵动）
const (
	IconGemini = "✨"  // Gemini 核心智能星芒
	IconIncome = "🎉"  // 收入
	IconRefund = "↩️" // 退款
	IconBullet = "✦"  // 内部项目点缀符号
)

// BuildReplyData 纯函数：提炼领域实体为纯净的展示数据模型
func BuildReplyData(batch *ledger.BatchTransactions, jumpURL, imageURL string) ReplyData {
	if batch == nil {
		return ReplyData{}
	}

	// -------------------------------------------------------------
	// 场景 A：常规消费/收入/退款/开户初始化交易流水
	// -------------------------------------------------------------
	if len(batch.Transactions) > 0 {
		count := len(batch.Transactions)
		var totalAmount float64
		items := make([]TransactionItem, 0, count)

		// ⭐️ 协同判定：检查是否为开户建账流水
		isGenesis := false

		for _, tx := range batch.Transactions {
			// 退款冲减总额，其余累加
			if tx.Type == "refund" {
				totalAmount -= tx.Amount
			} else {
				totalAmount += tx.Amount
			}

			// 如果是 Equity 注入，归为期初建账
			if strings.HasPrefix(tx.Category, "Equity:") {
				isGenesis = true
			}

			displayName := cmp.Or(tx.Payee, tx.Narration, "日常消费")
			displayCat := formatCategory(tx.Category, 2)
			if isGenesis {
				displayCat = "期初建账"
			}

			items = append(items, TransactionItem{
				Type:            tx.Type, // expense / income / refund
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
				Tags:            tx.Tags,
			})
		}

		primaryCategory := items[0].DisplayCategory
		if isGenesis {
			primaryCategory = "期初建账"
		}

		return ReplyData{
			Count:           count,
			TotalAmount:     roundFloat(totalAmount, 2),
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
	// 场景 B：资产断言/对账场景（日常对账）
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
	// 1. 资产场景（期初建账 VS 日常核对）
	if r.PrimaryCategory == "期初建账" {
		if r.IsSingle {
			return fmt.Sprintf("%s 资产建账完成：%s 初始金额 ￥%.2f %s", IconGemini, r.PrimaryName, r.TotalAmount, r.Currency)
		}
		return fmt.Sprintf("%s 资产建账完成：共建立 %d 处初始基准账户，合计 ￥%.2f", IconGemini, r.Count, r.TotalAmount)
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

		var expenseTotal, incomeTotal float64
		var hasExpense, hasIncome bool

		for _, item := range r.Items {
			switch item.Type {
			case "income":
				incomeTotal += item.Amount
				hasIncome = true
			case "refund":
				expenseTotal -= item.Amount
				hasExpense = true
			default:
				expenseTotal += item.Amount
				hasExpense = true
			}
		}

		if hasExpense && hasIncome {
			return fmt.Sprintf("%s 记账成功：共 %d 笔（支出 ￥%.2f，收入 +￥%.2f）",
				IconGemini, r.Count, roundFloat(expenseTotal, 2), roundFloat(incomeTotal, 2))
		}
		if hasIncome && !hasExpense {
			return fmt.Sprintf("%s 收入入账：共计入 %d 笔收入，合计 +￥%.2f",
				IconIncome, r.Count, roundFloat(incomeTotal, 2))
		}
		return fmt.Sprintf("%s 记账成功：共计入 %d 笔账单，合计 ￥%.2f", IconGemini, r.Count, r.TotalAmount)
	}

	return fmt.Sprintf("%s 操作已成功处理", IconGemini)
}
