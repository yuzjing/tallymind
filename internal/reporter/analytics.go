// internal/reporter/analytics.go
package reporter

import (
	"bufio"
	"cmp"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"tallymind/internal/ledger"
)

var (
	txHeaderRegex = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\s+[*!]\s+"([^"]+)"(?:\s+"([^"]+)")?`)
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
// ⭐️ 新增 targetMember 参数：为空或 "family" 代表全家；否则仅统计该成员
func scanAndAggregatePeriod(basePath string, startDate, endDate time.Time, isYearly bool, targetMember string) (*PeriodicReportData, error) {
	var records []rawTxRecord
	var totalExpense, totalIncome float64
	categoryMap := make(map[string]float64)
	memberExpenseMap := make(map[string]float64)
	trendMap := make(map[string]float64)

	targetMember = strings.ToLower(strings.TrimSpace(targetMember))
	isFamilyScope := targetMember == "" || targetMember == "family" || targetMember == "all"

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

		flushTx := func() {
			if currentTx != nil {
				if currentTx.Amount > 0 {
					records = append(records, *currentTx)
				}
				currentTx = nil
			}
		}

		for scanner.Scan() {
			line := scanner.Text()

			// 1. 匹配交易主行
			if matches := txHeaderRegex.FindStringSubmatch(line); len(matches) > 0 {
				flushTx()
				txDate, _ := time.ParseInLocation("2006-01-02", matches[1], startDate.Location())
				if !txDate.Before(startDate) && !txDate.After(endDate) {
					dateStr := matches[1]
					p1 := matches[2]
					p2 := ""
					if len(matches) > 3 {
						p2 = matches[3]
					}
					payee, narration := p1, p2
					if p2 == "" {
						payee, narration = "", p1
					}
					currentTx = &rawTxRecord{
						Date:      dateStr,
						Payee:     payee,
						Narration: narration,
					}
				} else {
					currentTx = nil
				}
				continue
			}

			if currentTx != nil {
				// 2. 匹配元数据行
				if mMatches := metaRegex.FindStringSubmatch(line); len(mMatches) > 0 {
					switch mMatches[1] {
					case "owner":
						currentTx.Owner = mMatches[2]
					case "beneficiary":
						currentTx.Beneficiary = mMatches[2]
					}
					continue
				}

				// 3. 匹配账户分录行
				if pMatches := postingRegex.FindStringSubmatch(line); len(pMatches) > 0 {
					account := pMatches[1]
					amt, _ := strconv.ParseFloat(pMatches[2], 64)

					// ⭐️ 核心判定：成员隔离过滤
					// 若指定了个人口径，检查：出资人、受益人或结算账户是否属于该成员
					if !isFamilyScope {
						own := strings.ToLower(currentTx.Owner)
						ben := strings.ToLower(currentTx.Beneficiary)
						acc := strings.ToLower(account)
						// 只要 owner、beneficiary 匹配，或者账户命名空间包含该成员（如 Assets:Zhaozhao）
						matchMember := (own == targetMember) || (ben == targetMember) || strings.Contains(acc, ":"+targetMember+":")
						if !matchMember {
							continue // 非本人口径直接跳过
						}
					}

					// 支出统计
					if strings.HasPrefix(account, "Expenses:") {
						currentTx.Category = account
						currentTx.Amount += amt
						totalExpense += amt

						macroCat := formatCategory(account, 1)
						categoryMap[macroCat] += amt

						trendKey := currentTx.Date
						if isYearly && len(currentTx.Date) >= 7 {
							trendKey = currentTx.Date[:7]
						}
						trendMap[trendKey] += amt

						targetDisplayMember := cmp.Or(currentTx.Beneficiary, currentTx.Owner, "家庭公用")
						memberExpenseMap[targetDisplayMember] += amt

					} else if strings.HasPrefix(account, "Income:") && amt < 0 {
						totalIncome += -amt
					}
				}
			}

			if strings.TrimSpace(line) == "" {
				flushTx()
			}
		}

		flushTx()

		if scanErr := scanner.Err(); scanErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("scan ledger file error: %w", scanErr)
		}
		_ = file.Close()
	}

	// 汇总排序等逻辑保持原样...
	categoryStats := make([]CategoryStat, 0, len(categoryMap))
	for cat, sum := range categoryMap {
		if sum <= 0 {
			continue
		}
		pct := 0.0
		if totalExpense > 0 {
			pct = (sum / totalExpense) * 100
		}
		categoryStats = append(categoryStats, CategoryStat{
			Category:    cat,
			DisplayName: cat,
			Amount:      roundFloat(sum, 2),
			Percentage:  roundFloat(pct, 1),
		})
	}
	sort.Slice(categoryStats, func(i, j int) bool {
		return categoryStats[i].Amount > categoryStats[j].Amount
	})

	memberStats := make([]CategoryStat, 0, len(memberExpenseMap))
	for mem, sum := range memberExpenseMap {
		if sum <= 0 {
			continue
		}
		pct := 0.0
		if totalExpense > 0 {
			pct = (sum / totalExpense) * 100
		}
		memberStats = append(memberStats, CategoryStat{
			DisplayName: mem,
			Amount:      roundFloat(sum, 2),
			Percentage:  roundFloat(pct, 1),
		})
	}
	sort.Slice(memberStats, func(i, j int) bool {
		return memberStats[i].Amount > memberStats[j].Amount
	})

	sort.Slice(records, func(i, j int) bool {
		return records[i].Amount > records[j].Amount
	})
	topCount := 5
	if len(records) < 5 {
		topCount = len(records)
	}
	topExpenses := make([]ExpenseItem, 0, topCount)
	for i := 0; i < topCount; i++ {
		topExpenses = append(topExpenses, ExpenseItem{
			Date:        records[i].Date,
			DisplayName: cmp.Or(records[i].Payee, records[i].Narration, "日常消费"),
			Category:    formatCategory(records[i].Category, 2),
			Amount:      roundFloat(records[i].Amount, 2),
		})
	}

	trends := make([]TrendItem, 0, len(trendMap))
	for d, sum := range trendMap {
		trends = append(trends, TrendItem{Date: d, Amount: roundFloat(sum, 2)})
	}
	sort.Slice(trends, func(i, j int) bool {
		return trends[i].Date < trends[j].Date
	})

	savings := totalIncome - totalExpense
	savingsRate := 0.0
	if totalIncome > 0 && savings > 0 {
		savingsRate = (savings / totalIncome) * 100
	}

	return &PeriodicReportData{
		TotalExpense:      roundFloat(totalExpense, 2),
		TotalIncome:       roundFloat(totalIncome, 2),
		NetSavings:        roundFloat(savings, 2),
		SavingsRate:       roundFloat(savingsRate, 1),
		TransactionCount:  len(records),
		CategoryBreakdown: categoryStats,
		MemberBreakdown:   memberStats,
		TopExpenses:       topExpenses,
		Trends:            trends,
	}, nil
}

func formatCategory(fullCategory string, maxDepth int) string {
	if fullCategory == "" {
		return "未分类"
	}
	trimmed := fullCategory
	for _, prefix := range []string{"Expenses:", "Income:", "Liabilities:", "Assets:", "Equity:"} {
		if after, ok := strings.CutPrefix(trimmed, prefix); ok {
			trimmed = after
			break
		}
	}
	parts := strings.Split(trimmed, ":")
	if maxDepth > 0 && len(parts) > maxDepth {
		parts = parts[:maxDepth]
	}
	return strings.Join(parts, " > ")
}

func roundFloat(val float64, precision ...int) float64 {
	p := 6
	if len(precision) > 0 {
		p = precision[0]
	}
	ratio := math.Pow(10, float64(p))
	return math.Round(val*ratio) / ratio
}
