// internal/cron/schedules.go
package cron

import (
	"strconv"
	"strings"
	"time"

	"tallymind/internal/config"
)

// BuildTasksFromConfig 根据 TOML 配置动态构建调度策略（带优雅兜底）
func BuildTasksFromConfig(cfg *config.Config) []CronTask {
	tasks := []CronTask{}

	// 1. 周报策略解析
	weeklyFunc := ParseWeeklySchedule(cfg.App.WeeklyReportTime)
	tasks = append(tasks, CronTask{Period: "weekly", NextTime: weeklyFunc})

	// 2. 月报策略解析
	monthlyFunc := ParseMonthlySchedule(cfg.App.MonthlyReportTime)
	tasks = append(tasks, CronTask{Period: "monthly", NextTime: monthlyFunc})

	return tasks
}

// ParseWeeklySchedule 解析如 "Monday 08:30"
func ParseWeeklySchedule(spec string) ScheduleFunc {
	weekday := time.Monday
	hour, min := 8, 30

	parts := strings.Fields(spec)
	if len(parts) >= 2 {
		switch strings.ToLower(parts[0]) {
		case "sunday", "sun", "周日", "星期日":
			weekday = time.Sunday
		case "monday", "mon", "周一", "星期一":
			weekday = time.Monday
		}
		timeParts := strings.Split(parts[1], ":")
		if len(timeParts) >= 2 {
			h, _ := strconv.Atoi(timeParts[0])
			m, _ := strconv.Atoi(timeParts[1])
			hour, min = h, m
		}
	}

	return func() time.Time {
		now := time.Now()
		days := int(weekday - now.Weekday())
		if days < 0 || (days == 0 && (now.Hour() > hour || (now.Hour() == hour && now.Minute() >= min))) {
			days += 7
		}
		return time.Date(now.Year(), now.Month(), now.Day()+days, hour, min, 0, 0, now.Location())
	}
}

// ParseMonthlySchedule 解析如 "1 09:00"
func ParseMonthlySchedule(spec string) ScheduleFunc {
	day := 1
	hour, min := 9, 0

	parts := strings.Fields(spec)
	if len(parts) >= 2 {
		d, _ := strconv.Atoi(parts[0])
		if d > 0 {
			day = d
		}
		timeParts := strings.Split(parts[1], ":")
		if len(timeParts) >= 2 {
			h, _ := strconv.Atoi(timeParts[0])
			m, _ := strconv.Atoi(timeParts[1])
			hour, min = h, m
		}
	}

	return func() time.Time {
		now := time.Now()
		targetThisMonth := time.Date(now.Year(), now.Month(), day, hour, min, 0, 0, now.Location())
		if now.Before(targetThisMonth) {
			return targetThisMonth
		}
		return time.Date(now.Year(), now.Month()+1, day, hour, min, 0, 0, now.Location())
	}
}
