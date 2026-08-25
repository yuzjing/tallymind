// internal/reporter/analytics.go
package reporter

import (
	"bufio"
	"cmp"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"tallymind/internal/ledger"
)

// 预编译正则，快速从 .bean 纯文本提取交易行
var (
	txHeaderRegex = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\s+\*\s+"([^"]*)"(?:\s+"([^"]*)")?`)
	postingRegex  = regexp.MustCompile(`^\s+([A-Za-z0-9:]+)\s+([-\d.]+)\s+([A-Za-z]+)`)
)

type rawTxRecord struct {
	Date      string
	Payee     string
	Narration string
	Category  string
	Amount    float64
}

// scanAndAggregatePeriod 扫描并聚合指定时间范围内的财务流水
func scanAndAggregatePeriod(basePath string, startDate, endDate time.Time) (*PeriodicReportData, error) {
	// 推导目标年份账本路径 (支持跨年)
	targetFile := ledger.GetYearlyFilePath(basePath, startDate.Format("2006-01-02"))
	file, err := os.Open(targetFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &PeriodicReportData{}, nil
		}
		return nil, fmt.Errorf("open ledger file error: %w", err)
	}
	defer file.Close()

	var records []rawTxRecord
	var totalExpense, totalIncome float64
	categoryMap := make(map[string]float64)

	scanner := bufio.NewScanner(file)
	var currentTx *rawTxRecord

	for scanner.Scan() {
		line := scanner.Text()

		// 匹配交易主行: 2026-08-18 * "商户" "备注"
		if matches := txHeaderRegex.FindStringSubmatch(line); len(matches) > 0 {
			txDate, _ := time.Parse("2006-01-02", matches[1])
			if !txDate.Before(startDate) && !txDate.After(endDate) {
				currentTx = &rawTxRecord{
					Date:      matches[1],
					Payee:     matches[2],
					Narration: matches[3],
				}
			} else {
				currentTx = nil
			}
			continue
		}

		// 匹配账户明细行
		if currentTx != nil {
			if pMatches := postingRegex.FindStringSubmatch(line); len(pMatches) > 0 {
				account := pMatches[1]
				amt, _ := strconv.ParseFloat(pMatches[2], 64)

				if strings.HasPrefix(account, "Expenses:") && amt > 0 {
					currentTx.Category = account
					currentTx.Amount = amt
					totalExpense += amt
					categoryMap[account] += amt
					records = append(records, *currentTx)
				} else if strings.HasPrefix(account, "Income:") && amt < 0 {
					totalIncome += -amt
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan ledger file error: %w", err)
	}

	// 1. 分类占比排行榜
	categoryStats := make([]CategoryStat, 0, len(categoryMap))
	for cat, sum := range categoryMap {
		pct := 0.0
		if totalExpense > 0 {
			pct = (sum / totalExpense) * 100
		}
		categoryStats = append(categoryStats, CategoryStat{
			Category:      cat,
			ShortCategory: getShortCategory(cat),
			Amount:        sum,
			Percentage:    pct,
		})
	}
	// 按消费金额倒序排列
	sort.Slice(categoryStats, func(i, j int) bool {
		return categoryStats[i].Amount > categoryStats[j].Amount
	})

	// 2. 筛选 Top 3 大额开销
	sort.Slice(records, func(i, j int) bool {
		return records[i].Amount > records[j].Amount
	})
	topExpenses := make([]ExpenseItem, 0, 3)
	for i := 0; i < len(records) && i < 3; i++ {
		topExpenses = append(topExpenses, ExpenseItem{
			Date:        records[i].Date,
			DisplayName: cmp.Or(records[i].Payee, records[i].Narration, "日常消费"),
			Category:    getShortCategory(records[i].Category),
			Amount:      records[i].Amount,
		})
	}

	return &PeriodicReportData{
		TotalExpense:      totalExpense,
		TotalIncome:       totalIncome,
		NetSavings:        totalIncome - totalExpense,
		TransactionCount:  len(records),
		CategoryBreakdown: categoryStats,
		TopExpenses:       topExpenses,
	}, nil
}
