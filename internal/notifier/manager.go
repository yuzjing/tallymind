// internal/notifier/manager.go
package notifier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// Manager 多渠道通知调度中心
type Manager struct {
	mu        sync.RWMutex
	notifiers map[string]Notifier
}

func NewManager() *Manager {
	m := &Manager{
		notifiers: make(map[string]Notifier),
	}
	// ⭐️ 核心集成：底座默认挂载通用 webhook 适配器
	m.Register("webhook", NewWebhookAdapter())
	return m
}

// Register 注册通知驱动渠道 (如 "webhook", "wecom", "feishu" 等)
func (m *Manager) Register(name string, n Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if n != nil && name != "" {
		m.notifiers[name] = n
		slog.Info("[Notifier] 成功注册通知渠道驱动", "driver", name)
	}
}

// SendDirect 发送到指定驱动渠道并指定接收目标 Target
func (m *Manager) SendDirect(ctx context.Context, driver string, target string, msg Message) error {
	m.mu.RLock()
	n, exists := m.notifiers[driver]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("[Notifier] 驱动渠道 [%s] 未注册或未启用", driver)
	}

	if target != "" {
		msg.Target = target
	}
	return n.Push(ctx, msg)
}

// Broadcast 广播到所有已启用的驱动渠道
func (m *Manager) Broadcast(ctx context.Context, msg Message) error {
	return m.SendTo(ctx, msg, "")
}

// SendTo 发送到指定的一个或多个渠道
func (m *Manager) SendTo(ctx context.Context, msg Message, channels ...string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.notifiers) == 0 {
		slog.WarnContext(ctx, "[Notifier] 未注册任何有效通知渠道，跳过发送")
		return nil
	}

	targetNotifiers := make(map[string]Notifier)
	if len(channels) == 0 || (len(channels) == 1 && channels[0] == "") {
		for name, n := range m.notifiers {
			targetNotifiers[name] = n
		}
	} else {
		for _, name := range channels {
			if n, ok := m.notifiers[name]; ok {
				targetNotifiers[name] = n
			} else {
				slog.WarnContext(ctx, "[Notifier] 请求推送未注册的渠道", "driver", name)
			}
		}
	}

	var errList []error
	for name, n := range targetNotifiers {
		if err := n.Push(ctx, msg); err != nil {
			slog.ErrorContext(ctx, "[Notifier] 渠道推送失败", "driver", name, "err", err)
			errList = append(errList, err)
		}
	}

	return errors.Join(errList...)
}
