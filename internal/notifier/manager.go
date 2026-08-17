// internal/notifier/manager.go
package notifier

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

// Manager 多渠道通知调度中心
type Manager struct {
	mu sync.Mutex
	// 通知渠道列表
	notifiers map[string]Notifier
}

func NewManager() *Manager {
	return &Manager{
		notifiers: make(map[string]Notifier),
	}
}

// Register 注册任意渠道
func (m *Manager) Register(name string, n Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if n != nil && name != "" {
		m.notifiers[name] = n
		slog.Info("[Notifier] 成功注册通知渠道", "driver", name)
	}
}

// Broadcast 全局广播
func (m *Manager) Broadcast(ctx context.Context, msg Message) error {
	return m.SendTo(ctx, msg, "")
}

func (m *Manager) SendTo(ctx context.Context, msg Message, channels ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.notifiers) == 0 {
		slog.WarnContext(ctx, "[Notifier] 未注册任何有效通知渠道，跳过发送")
		return nil
	}

	// 如果没有指定 channels，默认广播到所有已注册渠道
	targetNotifiers := make(map[string]Notifier)
	if len(channels) == 0 {
		for name, n := range m.notifiers {
			targetNotifiers[name] = n
		}
	} else {
		for _, name := range channels {
			if n, ok := m.notifiers[name]; ok {
				targetNotifiers[name] = n
			} else {
				slog.WarnContext(ctx, "[Notifier] 请求推送未注册/未启用的渠道", "driver", name)
			}
		}
	}

	// 依次推送并聚合错误
	var errList []error
	for name, n := range targetNotifiers {
		if err := n.Push(ctx, msg); err != nil {
			slog.ErrorContext(ctx, "[Notifier] 渠道推送失败", "driver", name, "err", err)
			errList = append(errList, err)
		}
	}

	return errors.Join(errList...)
}
