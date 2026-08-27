// internal/cron/types.go
package cron

import (
	"context"
	"time"

	"tallymind/internal/reporter"
)

// ReportDispatcher 周期报表分发器函数签名 (解耦具体通知渠道)
type ReportDispatcher func(ctx context.Context, report *reporter.PeriodicReportData) error

// ScheduleFunc 计算下一次触发时刻的策略函数签名
type ScheduleFunc func() time.Time

// CronTask 调度任务元数据实体
type CronTask struct {
	Period   string       // 任务周期标识: "weekly" | "monthly" | "quarterly" | "yearly"
	NextTime ScheduleFunc // 动态计算下一次运行时间的策略
}
