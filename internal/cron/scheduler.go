// internal/cron/scheduler.go
package cron

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"tallymind/internal/config"
	"tallymind/internal/notifier"
	"tallymind/internal/service"
	"tallymind/internal/template"
)

type Scheduler struct {
	cfg               *config.Config
	accountingService *service.AccountingService
	notifierMgr       *notifier.Manager
	dispatchers       map[string][]ReportDispatcher // ⭐️ 补回手动注册的分发器集合
	tasks             []CronTask
	mu                sync.RWMutex
}

func NewScheduler(cfg *config.Config, accountingService *service.AccountingService, notifierMgr *notifier.Manager) *Scheduler {
	return &Scheduler{
		cfg:               cfg,
		accountingService: accountingService,
		notifierMgr:       notifierMgr,
		tasks:             BuildTasksFromConfig(cfg),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	if !s.cfg.App.EnableReporter {
		slog.Info("⏰ 定时报表推送已被配置禁用")
		return
	}

	slog.Info("⏰ 定时报表调度引擎已启动", "tasks_count", len(s.tasks))
	for _, task := range s.tasks {
		go s.runPeriodLoop(ctx, task)
	}
}

func (s *Scheduler) runPeriodLoop(ctx context.Context, task CronTask) {
	for {
		next := task.NextTime()
		duration := time.Until(next)
		slog.Info("⏳ 计划下一次报表推送", "period", task.Period, "target_time", next.Format("2006-01-02 15:04:05"))

		timer := time.NewTimer(duration)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.Trigger(ctx, task.Period)
		}
	}
}

// Trigger 执行特定周期的全目标报表生成与分发
func (s *Scheduler) Trigger(ctx context.Context, period string) {
	now := time.Now()
	// 结算上一周期
	var targetTime time.Time
	switch period {
	case "weekly":
		targetTime = now.AddDate(0, 0, -7)
	case "monthly":
		targetTime = now.AddDate(0, -1, 0)
	default:
		targetTime = now
	}

	// -------------------------------------------------------------
	// 模式 2：遍历 config.toml 声明的 reporter_targets 矩阵分发
	// -------------------------------------------------------------
	targets := s.cfg.App.ReporterTargets
	if len(targets) == 0 {
		return
	}

	type WecomMsgPayload struct {
		MsgType string `json:"msgtype"`
		Text    struct {
			Content string `json:"content"`
		} `json:"text"`
	}

	for _, target := range targets {
		if len(target.Periods) > 0 && !slices.Contains(target.Periods, period) {
			continue
		}

		slog.InfoContext(ctx, "🎯 匹配到推送目标",
			"name", target.Name,
			"channel", target.Channel,
			"to_user", target.ToUser,
			"scope", target.Scope,
		)

		scope := strings.TrimSpace(target.Scope)
		reportData, err := s.accountingService.GetPeriodicReport(ctx, period, targetTime, "", "", scope)
		if err != nil {
			slog.ErrorContext(ctx, "生成指定口径报表失败", "target", target.Name, "scope", scope, "err", err)
			continue
		}

		msgPayload, err := template.Render[WecomMsgPayload](s.cfg.App.TemplateDir, s.cfg.WeCom.ReportTemplate, reportData)
		if err != nil {
			slog.ErrorContext(ctx, "渲染报表模板失败", "target", target.Name, "err", err)
			continue
		}

		msg := notifier.Message{
			Type:    notifier.TypeText,
			Content: msgPayload.Text.Content,
		}

		driver := strings.ToLower(strings.TrimSpace(target.Channel))
		if driver == "app" {
			driver = "wecom"
		}
		dest := cmp.Or(strings.TrimSpace(target.ToUser), strings.TrimSpace(target.URL))

		sendErr := s.notifierMgr.SendDirect(ctx, driver, dest, msg)

		if sendErr != nil {
			slog.ErrorContext(ctx, "推送目标失败", "target", target.Name, "err", sendErr)
		} else {
			slog.InfoContext(ctx, "✅ 定时报表已推送", "target", target.Name, "channel", target.Channel)
		}
	}
}
