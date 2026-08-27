// internal/cron/scheduler.go
package cron

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"tallymind/internal/config"
	"tallymind/internal/service"
)

type Scheduler struct {
	cfg               *config.Config
	accountingService *service.AccountingService
	dispatchers       map[string][]ReportDispatcher
	tasks             []CronTask
	mu                sync.RWMutex
}

// NewScheduler 实例化纯调度引擎并自动加载默认策略
func NewScheduler(cfg *config.Config, accountingService *service.AccountingService) *Scheduler {
	s := &Scheduler{
		cfg:               cfg,
		accountingService: accountingService,
		dispatchers:       make(map[string][]ReportDispatcher),
		tasks:             DefaultTasks(), // 👈 统一由 schedules.go 提供默认策略
	}
	return s
}

// RegisterTask 动态注册或覆盖定时任务策略
func (s *Scheduler) RegisterTask(period string, schedule ScheduleFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = append(s.tasks, CronTask{Period: period, NextTime: schedule})
}

// OnReport 注册特定周期的推送分发器 (支持 "weekly", "monthly" 等)
func (s *Scheduler) OnReport(period string, dispatcher ReportDispatcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dispatchers[period] = append(s.dispatchers[period], dispatcher)
}

// Start 启动调度器
func (s *Scheduler) Start(ctx context.Context) {
	if !s.cfg.App.EnableReporter {
		return
	}

	slog.Info("⏰ 定时报表调度器已就绪", "tasks_count", len(s.tasks))
	for _, task := range s.tasks {
		go s.runPeriodLoop(ctx, task)
	}
}

func (s *Scheduler) runPeriodLoop(ctx context.Context, task CronTask) {
	for {
		next := task.NextTime()
		timer := time.NewTimer(time.Until(next))

		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.Trigger(ctx, task.Period)
		}
	}
}

func (s *Scheduler) Trigger(ctx context.Context, period string) {
	s.mu.RLock()
	handlers := append([]ReportDispatcher(nil), s.dispatchers[period]...)
	s.mu.RUnlock()

	if len(handlers) == 0 {
		return
	}

	slog.Info("📊 开始生成并分发主动推送报表...", "period", period, "handlers_count", len(handlers))

	reportData, err := s.accountingService.GetPeriodicReport(ctx, period, time.Now(), "", "")
	if err != nil {
		slog.ErrorContext(ctx, "生成推送报表失败", "period", period, "err", err)
		return
	}

	for _, handler := range handlers {
		if err := handler(ctx, reportData); err != nil {
			slog.ErrorContext(ctx, "分发报表失败", "period", period, "err", err)
		}
	}
}
