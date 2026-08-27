// internal/cron/schedules.go
package cron

import (
	"time"
)

// DefaultTasks 预置系统标准的默认定时任务策略
func DefaultTasks() []CronTask {
	return []CronTask{
		{Period: "weekly", NextTime: WeeklySundayAt(20, 0, 0)},    // 每周日 20:00
		{Period: "monthly", NextTime: MonthlyFirstDayAt(9, 0, 0)}, // 每月1号 09:00
	}
}

// WeeklySundayAt 生成“每周日指定时分秒”的时间策略
func WeeklySundayAt(hour, min, sec int) ScheduleFunc {
	return func() time.Time {
		now := time.Now()
		daysUntilSunday := int(time.Sunday - now.Weekday())
		if daysUntilSunday <= 0 {
			daysUntilSunday += 7
		}
		nextSunday := time.Date(now.Year(), now.Month(), now.Day()+daysUntilSunday, hour, min, sec, 0, now.Location())
		if now.Weekday() == time.Sunday && (now.Hour() < hour || (now.Hour() == hour && now.Minute() < min)) {
			nextSunday = time.Date(now.Year(), now.Month(), now.Day(), hour, min, sec, 0, now.Location())
		}
		return nextSunday
	}
}

// MonthlyFirstDayAt 生成“每月1号指定时分秒”的时间策略
func MonthlyFirstDayAt(hour, min, sec int) ScheduleFunc {
	return func() time.Time {
		now := time.Now()
		targetThisMonth := time.Date(now.Year(), now.Month(), 1, hour, min, sec, 0, now.Location())
		if now.Before(targetThisMonth) {
			return targetThisMonth
		}
		return targetThisMonth.AddDate(0, 1, 0)
	}
}

// DailyAt 生成“每天指定时分秒”的时间策略
func DailyAt(hour, min, sec int) ScheduleFunc {
	return func() time.Time {
		now := time.Now()
		targetToday := time.Date(now.Year(), now.Month(), now.Day(), hour, min, sec, 0, now.Location())
		if now.Before(targetToday) {
			return targetToday
		}
		return targetToday.AddDate(0, 0, 1)
	}
}
