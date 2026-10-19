// internal/reporter/periodic.go
package reporter

import (
	"fmt"
	"time"
)

// GeneratePeriodicReport 通用周期报表生成器
func GeneratePeriodicReport(basePath string, periodType string, targetTime time.Time, customStart, customEnd string, jumpURL string, targetMember ...string) (*PeriodicReportData, error) {
	var startDate, endDate, prevStartDate, prevEndDate time.Time
	var prevTarget, nextTarget time.Time
	var title, dateRange string
	isYearly := false

	loc := targetTime.Location()
	now := time.Now().In(loc)

	switch periodType {
	case "custom":
		s, err1 := time.ParseInLocation("2006-01-02", customStart, loc)
		e, err2 := time.ParseInLocation("2006-01-02", customEnd, loc)
		if err1 != nil || err2 != nil {
			s = now.AddDate(0, 0, -30)
			e = now
		}
		// ⭐️ 核心修复：严禁用 Truncate(24h)，必须用 time.Date 对齐本地时区 00:00:00
		startDate = time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, loc)
		endDate = time.Date(e.Year(), e.Month(), e.Day(), 23, 59, 59, 999999999, loc)
		title = "自定义周期 · 财务报表"
		dateRange = fmt.Sprintf("%s ~ %s", startDate.Format("2006.01.02"), endDate.Format("2006.01.02"))

	case "yearly":
		isYearly = true
		startDate = time.Date(targetTime.Year(), 1, 1, 0, 0, 0, 0, loc)
		endDate = time.Date(targetTime.Year(), 12, 31, 23, 59, 59, 999999999, loc)
		prevStartDate = startDate.AddDate(-1, 0, 0)
		prevEndDate = time.Date(startDate.Year()-1, 12, 31, 23, 59, 59, 999999999, loc)
		prevTarget = startDate.AddDate(-1, 0, 0)
		nextTarget = startDate.AddDate(1, 0, 0)
		title = fmt.Sprintf("%d年度 · 财务年报", targetTime.Year())
		dateRange = "01.01 ~ 12.31"

	case "quarterly":
		quarter := (int(targetTime.Month())-1)/3 + 1
		startMonth := time.Month((quarter-1)*3 + 1)
		startDate = time.Date(targetTime.Year(), startMonth, 1, 0, 0, 0, 0, loc)
		// 季末最后一天 23:59:59
		endDate = startDate.AddDate(0, 3, 0).Add(-time.Nanosecond)
		prevStartDate = startDate.AddDate(0, -3, 0)
		prevEndDate = startDate.Add(-time.Nanosecond)
		prevTarget = startDate.AddDate(0, -3, 0)
		nextTarget = startDate.AddDate(0, 3, 0)
		title = fmt.Sprintf("%d年 第%d季度 · 财务季报", targetTime.Year(), quarter)
		dateRange = fmt.Sprintf("%s ~ %s", startDate.Format("01.02"), endDate.Format("01.02"))

	case "monthly":
		startDate = time.Date(targetTime.Year(), targetTime.Month(), 1, 0, 0, 0, 0, loc)
		endDate = startDate.AddDate(0, 1, 0).Add(-time.Nanosecond)
		prevStartDate = startDate.AddDate(0, -1, 0)
		prevEndDate = startDate.Add(-time.Nanosecond)
		prevTarget = startDate.AddDate(0, -1, 0)
		nextTarget = startDate.AddDate(0, 1, 0)
		title = fmt.Sprintf("%d年 %d月 · 财务月报", targetTime.Year(), int(targetTime.Month()))
		dateRange = fmt.Sprintf("%s ~ %s", startDate.Format("01.02"), endDate.Format("01.02"))

	default: // "weekly"
		periodType = "weekly"
		weekday := int(targetTime.Weekday())
		if weekday == 0 {
			weekday = 7 // 周日归为第 7 天
		}
		// 锁定本周一 00:00:00
		startDate = time.Date(targetTime.Year(), targetTime.Month(), targetTime.Day()-weekday+1, 0, 0, 0, 0, loc)
		endDate = startDate.AddDate(0, 0, 7).Add(-time.Nanosecond)
		prevStartDate = startDate.AddDate(0, 0, -7)
		prevEndDate = startDate.Add(-time.Nanosecond)
		prevTarget = startDate.AddDate(0, 0, -7)
		nextTarget = startDate.AddDate(0, 0, 7)

		isoYear, isoWeek := startDate.ISOWeek()
		title = fmt.Sprintf("%d年 第%d周 · 财务周报", isoYear, isoWeek)
		dateRange = fmt.Sprintf("%s ~ %s", startDate.Format("01.02"), endDate.Format("01.02"))
	}

	member := ""
	if len(targetMember) > 0 {
		member = targetMember[0]
	}

	data, err := scanAndAggregatePeriod(basePath, startDate, endDate, isYearly, member)
	if err != nil {
		return nil, err
	}

	if periodType != "custom" {
		if prevData, err := scanAndAggregatePeriod(basePath, prevStartDate, prevEndDate, isYearly, member); err == nil {
			data.HasPrevData = true
			data.PrevExpense = prevData.TotalExpense
			data.PrevIncome = prevData.TotalIncome
			data.PrevSavings = prevData.NetSavings

			data.ExpenseChangeDiff = roundFloat(data.TotalExpense-prevData.TotalExpense, 2)
			if prevData.TotalExpense > 0 {
				data.ExpenseChangeRate = roundFloat((data.ExpenseChangeDiff/prevData.TotalExpense)*100, 1)
			} else if data.TotalExpense > 0 {
				data.ExpenseChangeRate = 100.0
			}

			data.IncomeChangeDiff = roundFloat(data.TotalIncome-prevData.TotalIncome, 2)
			data.SavingsChangeDiff = roundFloat(data.NetSavings-prevData.NetSavings, 2)
		}
	}

	// ⭐️ 计算真实天数与日均开销 (修复除零)
	days := int(endDate.Sub(startDate).Hours()/24) + 1
	if days <= 0 {
		days = 1
	}
	data.DailyAverage = roundFloat(data.TotalExpense/float64(days), 2)

	// ⭐️ 笔均消费零值防御
	if data.TransactionCount > 0 {
		data.AvgPerTransaction = roundFloat(data.TotalExpense/float64(data.TransactionCount), 2)
	} else {
		data.AvgPerTransaction = 0
	}

	data.PeriodType = periodType
	data.Title = title
	data.StartDate = startDate.Format("2006-01-02")
	data.EndDate = endDate.Format("2006-01-02")
	data.DateRange = dateRange
	data.JumpURL = jumpURL
	data.TargetDate = targetTime.Format("2006-01-02")
	data.PrevDate = prevTarget.Format("2006-01-02")
	data.NextDate = nextTarget.Format("2006-01-02")

	// ⭐️ 修复 HasNext：只有未来周期才禁用下一页，当前或历史周期均可往后翻
	data.HasNext = !nextTarget.After(now)

	return data, nil
}
