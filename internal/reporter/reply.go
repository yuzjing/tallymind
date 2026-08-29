// internal/reporter/reply.go
package reporter

import (
	"cmp"
	"fmt"
	"tallymind/internal/ledger"
)

// BuildReplyData 纯函数：提炼领域实体为纯净的展示数据模型
func BuildReplyData(batch *ledger.BatchTransactions, jumpURL, imageURL string) ReplyData {
	if batch == nil {
		return ReplyData{}
	}

	// -------------------------------------------------------------
	// 场景 A：常规消费/收入交易流水
	// -------------------------------------------------------------
	if len(batch.Transactions) > 0 {
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

		return ReplyData{
			Count:           count,
			TotalAmount:     roundFloat(totalAmount, 2),
			Currency:        cmp.Or(batch.Transactions[0].Currency, "CNY"),
			IsSingle:        count == 1,
			PrimaryName:     items[0].DisplayName,
			PrimaryCategory: items[0].DisplayCategory,
			Items:           items,
			FirstItem:       items[0],
			JumpURL:         jumpURL,
			ImageURL:        imageURL,
		}
	}

	// -------------------------------------------------------------
	// ⭐️ 场景 B：纯资产对账/断言场景 (全量循环映射，支持多账户批量对账！)
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

// SummaryHeadline 极简总结
func (r ReplyData) SummaryHeadline() string {
	if r.PrimaryCategory == "资产对账" {
		if r.IsSingle {
			return fmt.Sprintf("👌 资产对账已记录：%s 余额 ￥%.2f %s", r.PrimaryName, r.TotalAmount, r.Currency)
		}
		return fmt.Sprintf("👌 资产对账已记录：共核准 %d 处账户余额", r.Count)
	}

	if r.Count > 0 {
		if r.IsSingle {
			return fmt.Sprintf("👌 已记好：%s ￥%.2f (%s)", r.PrimaryName, r.FirstItem.Amount, r.PrimaryCategory)
		}
		return fmt.Sprintf("👌 已记好：共计入 %d 笔账单，合计 ￥%.2f", r.Count, r.TotalAmount)
	}

	return "👌 操作已成功处理"
}
