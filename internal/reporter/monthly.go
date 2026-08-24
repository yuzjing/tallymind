// internal/reporter/monthly.go
package reporter

import (
	"fmt"
	"time"
)

// GenerateMonthlyReportData 计算目标月份第一天至最后一天的月报数据模型
func GenerateMonthlyReportData(basePath string, targetMonth time.Time, jumpURL string) (*PeriodicReportData, error) {
	firstDay := time.Date(targetMonth.Year(), targetMonth.Month(), 1, 0, 0, 0, 0, targetMonth.Location())
	lastDay := firstDay.AddDate(0, 1, -1).Add(23*time.Hour + 59*time.Minute)

	data, err := scanAndAggregatePeriod(basePath, firstDay, lastDay)
	if err != nil {
		return nil, err
	}

	data.PeriodType = "monthly"
	data.Title = fmt.Sprintf("📈 %d年%d月 财务月度盘点", targetMonth.Year(), targetMonth.Month())
	data.DateRange = fmt.Sprintf("%s ~ %s", firstDay.Format("2006.01.02"), lastDay.Format("01.02"))
	data.JumpURL = jumpURL

	return data, nil
}
