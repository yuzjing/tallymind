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

var (
	txHeaderRegex = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\s+\*\s+"([^"]*)"(?:\s+"([^"]*)")?`)
	postingRegex  = regexp.MustCompile(`^\s+([A-Za-z0-9:]+)\s+([-\d.]+)\s+([A-Za-z]+)`)
	metaRegex     = regexp.MustCompile(`^\s+([a-z_]+):\s+"([^"]+)"`)
)

type rawTxRecord struct {
	Date        string
	Payee       string
	Narration   string
	Category    string
	Amount      float64
	Owner       string
	Beneficiary string
}

// scanAndAggregatePeriod 扫描并聚合指定时间区间的全量财务流水
func scanAndAggregatePeriod(basePath string, startDate, endDate time.Time, isYearly bool) (*PeriodicReportData, error) {
	var records []rawTxRecord
	var totalExpense, totalIncome float64
	categoryMap := make(map[string]float64)
	memberExpenseMap := make(map[string]float64)
	trendMap := make(map[string]float64)

	startYear, endYear := startDate.Year(), endDate.Year()
	for y := startYear; y <= endYear; y++ {
		targetFile := ledger.GetYearlyFilePath(basePath, fmt.Sprintf("%d-01-01", y))
		file, err := os.Open(targetFile)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("open ledger file error: %w", err)
		}

		scanner := bufio.NewScanner(file)
		var currentTx *rawTxRecord

		for scanner.Scan() {
			line := scanner.Text()

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

			if currentTx != nil {
				if mMatches := metaRegex.FindStringSubmatch(line); len(mMatches) > 0 {
					switch mMatches[1] {
					case "owner":
						currentTx.Owner = mMatches[2]
					case "beneficiary":
						currentTx.Beneficiary = mMatches[2]
					}
					continue
				}

				if pMatches := postingRegex.FindStringSubmatch(line); len(pMatches) > 0 {
					account := pMatches[1]
					amt, _ := strconv.ParseFloat(pMatches[2], 64)

					if strings.HasPrefix(account, "Expenses:") && amt > 0 {
						currentTx.Category = account
						currentTx.Amount += amt
						totalExpense += amt
						categoryMap[account] += amt

						// 年报按月聚合趋势 (如 "2026-08")，其余按日聚合 (如 "2026-08-26")
						trendKey := currentTx.Date
						if isYearly && len(currentTx.Date) >= 7 {
							trendKey = currentTx.Date[:7]
						}
						trendMap[trendKey] += amt

						targetMember := cmp.Or(currentTx.Beneficiary, currentTx.Owner, "家庭公用")
						memberExpenseMap[targetMember] += amt
					} else if strings.HasPrefix(account, "Income:") && amt < 0 {
						totalIncome += -amt
					}
				}
			}

			if strings.TrimSpace(line) == "" && currentTx != nil && currentTx.Amount > 0 {
				records = append(records, *currentTx)
				currentTx = nil
			}
		}

		if scanErr := scanner.Err(); scanErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("scan ledger file error: %w", scanErr)
		}
		_ = file.Close()
	}

	// 1. 分类排行榜
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
	sort.Slice(categoryStats, func(i, j int) bool { return categoryStats[i].Amount > categoryStats[j].Amount })

	// 2. 成员开销排行榜
	memberStats := make([]CategoryStat, 0, len(memberExpenseMap))
	for mem, sum := range memberExpenseMap {
		pct := 0.0
		if totalExpense > 0 {
			pct = (sum / totalExpense) * 100
		}
		memberStats = append(memberStats, CategoryStat{
			ShortCategory: mem,
			Amount:        sum,
			Percentage:    pct,
		})
	}
	sort.Slice(memberStats, func(i, j int) bool { return memberStats[i].Amount > memberStats[j].Amount })

	// 3. Top 5 大额开销
	sort.Slice(records, func(i, j int) bool { return records[i].Amount > records[j].Amount })
	topCount := 5
	if len(records) < 5 {
		topCount = len(records)
	}
	topExpenses := make([]ExpenseItem, 0, topCount)
	for i := 0; i < topCount; i++ {
		topExpenses = append(topExpenses, ExpenseItem{
			Date:        records[i].Date,
			DisplayName: cmp.Or(records[i].Payee, records[i].Narration, "日常消费"),
			Category:    getShortCategory(records[i].Category),
			Amount:      records[i].Amount,
		})
	}

	// 4. 趋势数据 (统一使用 TrendItem)
	trends := make([]TrendItem, 0, len(trendMap))
	for d, sum := range trendMap {
		trends = append(trends, TrendItem{Date: d, Amount: sum})
	}
	sort.Slice(trends, func(i, j int) bool { return trends[i].Date < trends[j].Date })

	savings := totalIncome - totalExpense
	savingsRate := 0.0
	if totalIncome > 0 && savings > 0 {
		savingsRate = (savings / totalIncome) * 100
	}

	return &PeriodicReportData{
		TotalExpense:      totalExpense,
		TotalIncome:       totalIncome,
		NetSavings:        savings,
		SavingsRate:       savingsRate,
		TransactionCount:  len(records),
		CategoryBreakdown: categoryStats,
		MemberBreakdown:   memberStats,
		TopExpenses:       topExpenses,
		Trends:            trends, // 赋值给 Trends
	}, nil
}

func getShortCategory(fullCategory string) string {
	if fullCategory == "" {
		return "未分类"
	}

	// 1. 去除根前缀
	trimmed := fullCategory
	for _, prefix := range []string{"Expenses:", "Income:", "Liabilities:", "Assets:", "Equity:"} {
		if strings.HasPrefix(trimmed, prefix) {
			trimmed = strings.TrimPrefix(trimmed, prefix)
			break
		}
	}

	// 2. 截取最多两级
	parts := strings.Split(trimmed, ":")
	if len(parts) > 2 {
		parts = parts[:2] // 强制截断为前两级
	}

	return strings.Join(parts, " > ")
}
