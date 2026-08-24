// internal/reporter/weekly.go
package reporter

import (
	"fmt"
	"time"
)

// GenerateWeeklyReportData 计算过去 7 天 (周一至周日) 的周报数据模型
func GenerateWeeklyReportData(basePath string, now time.Time, jumpURL string) (*PeriodicReportData, error) {
	// 计算本周周一 00:00 与 周日 23:59
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, -weekday+1).Truncate(24 * time.Hour)
	sunday := monday.AddDate(0, 0, 6).Add(23*time.Hour + 59*time.Minute)

	data, err := scanAndAggregatePeriod(basePath, monday, sunday)
	if err != nil {
		return nil, err
	}

	data.PeriodType = "weekly"
	data.Title = "📊 本周财务消费周报"
	data.DateRange = fmt.Sprintf("%s ~ %s", monday.Format("01.02"), sunday.Format("01.02"))
	data.JumpURL = jumpURL

	return data, nil
}
