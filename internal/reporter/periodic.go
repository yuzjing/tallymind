// internal/reporter/periodic.go
package reporter

import (
	"fmt"
	"time"
)

// GeneratePeriodicReport 通用周期报表生成器
func GeneratePeriodicReport(basePath string, periodType string, targetTime time.Time, customStart, customEnd string, jumpURL string) (*PeriodicReportData, error) {
	var startDate, endDate, prevStartDate, prevEndDate time.Time
	var prevTarget, nextTarget time.Time
	var title, dateRange string
	isYearly := false

	now := time.Now()

	switch periodType {
	case "custom":
		s, err1 := time.Parse("2006-01-02", customStart)
		e, err2 := time.Parse("2006-01-02", customEnd)
		if err1 != nil || err2 != nil {
			s = now.AddDate(0, 0, -30)
			e = now
		}
		startDate = s.Truncate(24 * time.Hour)
		endDate = e.Add(23*time.Hour + 59*time.Minute)
		title = "自定义周期 · 财务报表"
		dateRange = fmt.Sprintf("%s ~ %s", startDate.Format("2006.01.02"), endDate.Format("2006.01.02"))

	case "yearly":
		isYearly = true
		startDate = time.Date(targetTime.Year(), 1, 1, 0, 0, 0, 0, targetTime.Location())
		endDate = time.Date(targetTime.Year(), 12, 31, 23, 59, 59, 0, targetTime.Location())
		prevStartDate = startDate.AddDate(-1, 0, 0)
		prevEndDate = endDate.AddDate(-1, 0, 0)
		prevTarget = startDate.AddDate(-1, 0, 0)
		nextTarget = startDate.AddDate(1, 0, 0)
		title = fmt.Sprintf("%d年度 · 财务年报", targetTime.Year())
		dateRange = "01.01 ~ 12.31" // 👈 去掉重复年份

	case "quarterly":
		quarter := (int(targetTime.Month())-1)/3 + 1
		startMonth := time.Month((quarter-1)*3 + 1)
		startDate = time.Date(targetTime.Year(), startMonth, 1, 0, 0, 0, 0, targetTime.Location())
		endDate = startDate.AddDate(0, 3, -1).Add(23*time.Hour + 59*time.Minute)
		prevStartDate = startDate.AddDate(0, -3, 0)
		prevEndDate = startDate.Add(-1 * time.Second)
		prevTarget = startDate.AddDate(0, -3, 0)
		nextTarget = startDate.AddDate(0, 3, 0)
		title = fmt.Sprintf("%d年 第%d季度 · 财务季报", targetTime.Year(), quarter)
		dateRange = fmt.Sprintf("%s ~ %s", startDate.Format("01.02"), endDate.Format("01.02"))

	case "monthly":
		startDate = time.Date(targetTime.Year(), targetTime.Month(), 1, 0, 0, 0, 0, targetTime.Location())
		endDate = startDate.AddDate(0, 1, -1).Add(23*time.Hour + 59*time.Minute)
		prevStartDate = startDate.AddDate(0, -1, 0)
		prevEndDate = startDate.Add(-1 * time.Second)
		prevTarget = startDate.AddDate(0, -1, 0)
		nextTarget = startDate.AddDate(0, 1, 0)
		title = fmt.Sprintf("%d年 %d月 · 财务月报", targetTime.Year(), int(targetTime.Month()))
		dateRange = fmt.Sprintf("%s ~ %s", startDate.Format("01.02"), endDate.Format("01.02")) // 👈 去掉重复年份

	default: // "weekly"
		periodType = "weekly"
		weekday := int(targetTime.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startDate = targetTime.AddDate(0, 0, -weekday+1).Truncate(24 * time.Hour)
		endDate = startDate.AddDate(0, 0, 6).Add(23*time.Hour + 59*time.Minute)
		prevStartDate = startDate.AddDate(0, 0, -7)
		prevEndDate = startDate.Add(-1 * time.Second)
		prevTarget = startDate.AddDate(0, 0, -7)
		nextTarget = startDate.AddDate(0, 0, 7)

		// 计算 ISO 周数 (如 2026年 第35周)
		isoYear, isoWeek := startDate.ISOWeek()
		title = fmt.Sprintf("%d年 第%d周 · 财务周报", isoYear, isoWeek)
		dateRange = fmt.Sprintf("%s ~ %s", startDate.Format("01.02"), endDate.Format("01.02"))
	}

	data, err := scanAndAggregatePeriod(basePath, startDate, endDate, isYearly)
	if err != nil {
		return nil, err
	}

	if periodType != "custom" {
		if prevData, err := scanAndAggregatePeriod(basePath, prevStartDate, prevEndDate, isYearly); err == nil && prevData.TotalExpense > 0 {
			data.PrevExpense = prevData.TotalExpense
			data.ExpenseChangeRate = ((data.TotalExpense - prevData.TotalExpense) / prevData.TotalExpense) * 100
		}
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
	data.HasNext = !startDate.After(now.Truncate(24*time.Hour)) && !nextTarget.After(now.AddDate(0, 0, 1))

	return data, nil
}
