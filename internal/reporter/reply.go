// internal/reporter/reply.go
package reporter

import (
	"cmp"
	"fmt"
	"tallymind/internal/ledger"
)

// BuildReplyData 纯函数：将 ledger 领域实体提炼为纯净的 ReplyData 数据模型
func BuildReplyData(batch *ledger.BatchTransactions, jumpURL, imageURL string) ReplyData {
	if batch == nil || len(batch.Transactions) == 0 {
		return ReplyData{}
	}

	count := len(batch.Transactions)
	var totalAmount float64
	items := make([]TransactionItem, 0, count)

	for _, tx := range batch.Transactions {
		totalAmount += tx.Amount

		// 提取展示名
		displayName := cmp.Or(tx.Payee, tx.Narration, "日常消费")

		items = append(items, TransactionItem{
			Date:            tx.Date,
			Payee:           tx.Payee,
			Narration:       tx.Narration,
			Category:        tx.Category,
			DisplayCategory: formatCategory(tx.Category, 2),
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
		TotalAmount:     totalAmount,
		Currency:        items[0].Currency,
		IsSingle:        count == 1,
		PrimaryName:     items[0].DisplayName,
		PrimaryCategory: formatCategory(items[0].Category, 2),
		Items:           items,
		FirstItem:       items[0],
		JumpURL:         jumpURL,
		ImageURL:        imageURL,
	}
}

// SummaryHeadline 生成标准的一句话极简总结 (单笔/多笔自适应，全系统通用)
func (r ReplyData) SummaryHeadline() string {
	if r.Count == 0 {
		return "暂无有效记账数据"
	}

	if r.IsSingle {
		return fmt.Sprintf("已记好：%s ￥%.2f (%s)", r.PrimaryName, r.FirstItem.Amount, r.PrimaryCategory)
	}

	return fmt.Sprintf("已记好：共计入 %d 笔账单，合计 ￥%.2f", r.Count, r.TotalAmount)
}
