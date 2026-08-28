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

// scanAndAggregatePeriod 扫描并聚合指定时间区间的全量财务流水 (支持跨年多文件安全扫描)
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

			// 1. 匹配交易主行 (日期、商户、摘要)
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
				// 2. 匹配元数据行 (owner / beneficiary)
				if mMatches := metaRegex.FindStringSubmatch(line); len(mMatches) > 0 {
					switch mMatches[1] {
					case "owner":
						currentTx.Owner = mMatches[2]
					case "beneficiary":
						currentTx.Beneficiary = mMatches[2]
					}
					continue
				}

				// 3. 匹配账户记账分录行
				if pMatches := postingRegex.FindStringSubmatch(line); len(pMatches) > 0 {
					account := pMatches[1]
					amt, _ := strconv.ParseFloat(pMatches[2], 64)

					// 支出统计 (以 Expenses: 开头的正数金额)
					if strings.HasPrefix(account, "Expenses:") && amt > 0 {
						currentTx.Category = account
						currentTx.Amount += amt
						totalExpense += amt

						// ⭐️ 核心聚合：使用 formatCategory(account, 1) 归拢为宏观一级大类 (如 "Food", "Transport")
						macroCat := formatCategory(account, 1)
						categoryMap[macroCat] += amt

						// 走势按月或按日归拢
						trendKey := currentTx.Date
						if isYearly && len(currentTx.Date) >= 7 {
							trendKey = currentTx.Date[:7]
						}
						trendMap[trendKey] += amt

						// 成员归属 (优先算给受益人，其次算出资人)
						targetMember := cmp.Or(currentTx.Beneficiary, currentTx.Owner, "家庭公用")
						memberExpenseMap[targetMember] += amt
					} else if strings.HasPrefix(account, "Income:") && amt < 0 {
						// 收入统计 (Beancount 中收入记录为负数，累加时转正数)
						totalIncome += -amt
					}
				}
			}

			// 空行代表一笔交易结束，写入切片
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

	// -------------------------------------------------------------
	// 1. 分类排行榜聚合与排序 (环形图使用)
	// -------------------------------------------------------------
	categoryStats := make([]CategoryStat, 0, len(categoryMap))
	for cat, sum := range categoryMap {
		pct := 0.0
		if totalExpense > 0 {
			pct = (sum / totalExpense) * 100
		}
		categoryStats = append(categoryStats, CategoryStat{
			Category:    cat,
			DisplayName: cat, // 此时 cat 已经是 formatCategory 提取后的纯大类 (如 "Food")
			Amount:      sum,
			Percentage:  pct,
		})
	}
	// ⭐️ 使用 sort.Slice 按金额从大到小倒序排序
	sort.Slice(categoryStats, func(i, j int) bool {
		return categoryStats[i].Amount > categoryStats[j].Amount
	})

	// -------------------------------------------------------------
	// 2. 成员开销排行榜聚合与排序 (柱状图使用)
	// -------------------------------------------------------------
	memberStats := make([]CategoryStat, 0, len(memberExpenseMap))
	for mem, sum := range memberExpenseMap {
		pct := 0.0
		if totalExpense > 0 {
			pct = (sum / totalExpense) * 100
		}
		memberStats = append(memberStats, CategoryStat{
			DisplayName: mem, // 成员名称 (如 "zhaozhao")
			Amount:      sum,
			Percentage:  pct,
		})
	}
	// ⭐️ 成员开销倒序排序
	sort.Slice(memberStats, func(i, j int) bool {
		return memberStats[i].Amount > memberStats[j].Amount
	})

	// -------------------------------------------------------------
	// 3. 筛选 Top 5 大额开销清单
	// -------------------------------------------------------------
	// ⭐️ 对所有交易按单笔金额倒序排序
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
			Category:    formatCategory(records[i].Category, 2), //  明细保留二级细分 (如 "Transport > Maintenance")
			Amount:      records[i].Amount,
		})
	}

	// -------------------------------------------------------------
	// 4. 趋势数据排序 (按日期升序)
	// -------------------------------------------------------------
	trends := make([]TrendItem, 0, len(trendMap))
	for d, sum := range trendMap {
		trends = append(trends, TrendItem{Date: d, Amount: sum})
	}
	sort.Slice(trends, func(i, j int) bool {
		return trends[i].Date < trends[j].Date
	})

	// -------------------------------------------------------------
	// 5. 结余与储蓄率指标计算
	// -------------------------------------------------------------
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
		Trends:            trends,
	}, nil
}

// formatCategory 剥离会计根前缀，并按指定最大层级深度 depth 截断格式化
// - maxDepth = 1  ➔ 仅取宏观大类 (如 "Food", 专供环形图聚合)
// - maxDepth = 2  ➔ 取两级层级 (如 "Food > Lunch", 专供小票与 Top5 明细)
// - maxDepth <= 0 ➔ 保留所有子层级 (如 "Food > Lunch > Hotpot")
func formatCategory(fullCategory string, maxDepth int) string {
	if fullCategory == "" {
		return "未分类"
	}

	// 1. 剥离 Expenses:/Income:/Assets:/Liabilities:/Equity: 根前缀
	trimmed := fullCategory
	for _, prefix := range []string{"Expenses:", "Income:", "Liabilities:", "Assets:", "Equity:"} {
		if strings.HasPrefix(trimmed, prefix) {
			trimmed = strings.TrimPrefix(trimmed, prefix)
			break
		}
	}

	// 2. 按冒号切分层级
	parts := strings.Split(trimmed, ":")
	if maxDepth > 0 && len(parts) > maxDepth {
		parts = parts[:maxDepth]
	}

	// 3. 用友好箭头拼接
	return strings.Join(parts, " > ")
}
