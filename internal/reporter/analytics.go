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
	Date          string
	Payee         string
	Narration     string
	Category      string
	Amount        float64
	Owner         string
	Beneficiary   string
	maxItemAmount float64 // 用于锁定多品类合并订单中金额最大的一级主科目
}

// scanAndAggregatePeriod 扫描并聚合指定时间区间的全量财务流水
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

		// 闭包：收敛并沉淀一笔有效支出到切片中
		flushTx := func() {
			if currentTx != nil {
				// 仅有实际净支出大于 0 的交易才收录到排行榜候选池中
				if currentTx.Amount > 0 {
					records = append(records, *currentTx)
				}
				currentTx = nil
			}
		}

		for scanner.Scan() {
			line := scanner.Text()

			// 1. 匹配交易主行 (日期、商户、摘要)
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
				// 2. 匹配元数据行 (owner / beneficiary / reporter 保底)
				if mMatches := metaRegex.FindStringSubmatch(line); len(mMatches) > 0 {
					switch mMatches[1] {
					case "owner":
						currentTx.Owner = mMatches[2]
					case "beneficiary":
						currentTx.Beneficiary = mMatches[2]
					case "reporter":
						// 若缺省 owner，自动以 reporter 兜底出资人
						if currentTx.Owner == "" {
							currentTx.Owner = mMatches[2]
						}
					}
					continue
				}

				// 3. 匹配账户分录行
				if pMatches := postingRegex.FindStringSubmatch(line); len(pMatches) > 0 {
					account := pMatches[1]
					amt, _ := strconv.ParseFloat(pMatches[2], 64)

					// ⭐️ 核心判定：成员隔离过滤
					if !isFamilyScope {
						own := strings.ToLower(currentTx.Owner)
						ben := strings.ToLower(currentTx.Beneficiary)
						acc := strings.ToLower(account)
						matchMember := (own == targetMember) || (ben == targetMember) || strings.Contains(acc, ":"+targetMember+":")
						if !matchMember {
							continue
						}
					}

					// 支出统计 (正数支出累加，负数退款自然冲减)
					if strings.HasPrefix(account, "Expenses:") {
						// ⭐️ 锁定单笔交易中金额最大的子类目作为主展示类目，避免被购物车最后一个小物件覆盖
						if amt > currentTx.maxItemAmount || currentTx.Category == "" {
							currentTx.maxItemAmount = amt
							currentTx.Category = account
						}
						currentTx.Amount += amt
						totalExpense += amt

						// 宏观一级大类归集 (自动平滑历史旧类目)
						macroCat := formatCategory(account, 1)
						categoryMap[macroCat] += amt

						// 趋势聚合
						trendKey := currentTx.Date
						if isYearly && len(currentTx.Date) >= 7 {
							trendKey = currentTx.Date[:7]
						}
						trendMap[trendKey] += amt

						// 成员归属
						targetDisplayMember := cmp.Or(currentTx.Beneficiary, currentTx.Owner, "家庭公用")
						memberExpenseMap[targetDisplayMember] += amt

					} else if strings.HasPrefix(account, "Income:") && amt < 0 {
						// 收入与价保补贴统计
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

	// -------------------------------------------------------------
	// 1. 分类排行榜聚合与排序 (环形图)
	// -------------------------------------------------------------
	categoryStats := make([]CategoryStat, 0, len(categoryMap))
	for cat, sum := range categoryMap {
		if sum <= 0 {
			continue // 冲减后净额 <= 0 不在消费环形图中显示
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

	// -------------------------------------------------------------
	// 2. 成员开销排行榜聚合与排序 (柱状图)
	// -------------------------------------------------------------
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

	// -------------------------------------------------------------
	// 3. 筛选 Top 5 大额开销清单 (⭐️ 彻底修复：严格比对 i 和 j)
	// -------------------------------------------------------------
	sort.Slice(records, func(i, j int) bool {
		return records[i].Amount > records[j].Amount // 👈 修复前两边都是 i
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

	// -------------------------------------------------------------
	// 4. 趋势数据排序 (按日期正序)
	// -------------------------------------------------------------
	trends := make([]TrendItem, 0, len(trendMap))
	for d, sum := range trendMap {
		trends = append(trends, TrendItem{Date: d, Amount: roundFloat(sum, 2)})
	}
	sort.Slice(trends, func(i, j int) bool {
		return trends[i].Date < trends[j].Date
	})

	// -------------------------------------------------------------
	// 5. 结余与储蓄率指标
	// -------------------------------------------------------------
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

// formatCategory 剥离会计根前缀，并按指定最大层级深度 depth 截断格式化
func formatCategory(fullCategory string, maxDepth int) string {
	if fullCategory == "" {
		return "未分类"
	}

	// ⭐️ 核心美化 1：债权、垫付与预付资产转化为高可读业务语义
	if strings.Contains(fullCategory, "Receivables:Company") {
		return "因公垫付 > 待报销"
	}
	if strings.Contains(fullCategory, "Receivables:Family") {
		return "亲友垫付 > 待结清"
	}
	if strings.Contains(fullCategory, "Receivables:Friends") {
		return "亲友往来 > 待收款"
	}
	if strings.Contains(fullCategory, "Prepaid:Deposit") {
		return "预售定金 > 待核销"
	}

	// 1. 剥离 Expenses:/Income:/Liabilities:/Assets:/Equity: 根前缀
	trimmed := fullCategory
	for _, prefix := range []string{"Expenses:", "Income:", "Liabilities:", "Assets:", "Equity:"} {
		if after, ok := strings.CutPrefix(trimmed, prefix); ok {
			trimmed = after
			break
		}
	}

	parts := strings.Split(trimmed, ":")

	// ⭐️ 核心美化 2：若为资产/负债转账科目 (如 Assets:Zhaozhao:Bank:BOC)，跳过人名避免暴露 Zhaozhao > Bank
	if len(parts) >= 3 && (strings.HasPrefix(fullCategory, "Assets:") || strings.HasPrefix(fullCategory, "Liabilities:")) {
		parts = parts[1:]
	}

	// ⭐️ 核心平滑 3：历史旧分类平滑过渡到新 10 大标准分类 (解决饼图同时出现 Home 和 Housing)
	if len(parts) > 0 {
		if parts[0] == "Home" {
			parts[0] = "Housing"
		} else if parts[0] == "Medical" {
			parts[0] = "Health"
		}
	}

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
