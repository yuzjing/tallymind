// internal/reporter/reply.go
package reporter

import (
	"cmp"
	"fmt"
	"tallymind/internal/ledger"
)

// BuildReplyData 纯函数：将 ledger 领域实体提炼为纯净的 ReplyData 展示数据模型
func BuildReplyData(batch *ledger.BatchTransactions, jumpURL, imageURL string) ReplyData {
	if batch == nil {
		return ReplyData{}
	}

	count := len(batch.Transactions)
	var totalAmount float64
	items := make([]TransactionItem, 0, count)

	for _, tx := range batch.Transactions {
		totalAmount += tx.Amount

		displayName := cmp.Or(tx.Payee, tx.Narration, "日常消费")
		displayCat := formatCategory(tx.Category, 2)

		items = append(items, TransactionItem{
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

	primaryName := "日常消费"
	primaryCat := "未分类"
	currency := "CNY"
	var firstItem TransactionItem

	if count > 0 {
		primaryName = items[0].DisplayName
		primaryCat = items[0].DisplayCategory
		firstItem = items[0]
		currency = cmp.Or(items[0].Currency, "CNY")
	} else if len(batch.BalanceAssertions) > 0 {
		// ⭐️ 核心防崩溃修复：处理纯对账/断言场景，避免访问空切片 Transactions[0]
		firstAssertion := batch.BalanceAssertions[0]
		primaryName = firstAssertion.Account
		primaryCat = "资产对账"
		totalAmount = firstAssertion.Amount
		currency = cmp.Or(firstAssertion.Currency, "CNY")
	}

	return ReplyData{
		Count:           count,
		TotalAmount:     roundFloat(totalAmount, 2),
		Currency:        currency, // 👈 安全赋值，绝不越界
		IsSingle:        count == 1,
		PrimaryName:     primaryName,
		PrimaryCategory: primaryCat,
		Items:           items,
		FirstItem:       firstItem,
		JumpURL:         jumpURL,
		ImageURL:        imageURL,
	}
}

// SummaryHeadline 生成即时记账、资产断言与自动找平的一句话极简总结
func (r ReplyData) SummaryHeadline() string {
	// 场景 1：包含常规记账流水
	if r.Count > 0 {
		if r.IsSingle {
			return fmt.Sprintf("👌 已记好：%s ￥%.2f (%s)", r.PrimaryName, r.FirstItem.Amount, r.PrimaryCategory)
		}
		return fmt.Sprintf("👌 已记好：共计入 %d 笔账单，合计 ￥%.2f", r.Count, r.TotalAmount)
	}

	// 场景 2：纯对账与平账场景
	return "👌 资产对账与断言已同步记录 (Fava 已完成数学核对)"
}
