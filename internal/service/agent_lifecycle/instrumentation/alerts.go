package instrumentation

import (
	"strings"
	"sync"
	"time"
)

// AlertTracker 负责对健康告警进行节流控制。
type AlertTracker struct {
	mu        sync.Mutex
	cooldown  time.Duration
	lastAlert map[string]time.Time
}

// NewAlertTracker 构造带默认状态的节流器。
func NewAlertTracker(cooldown time.Duration) *AlertTracker {
	if cooldown < 0 {
		cooldown = 0
	}
	return &AlertTracker{
		cooldown:  cooldown,
		lastAlert: make(map[string]time.Time),
	}
}

// Try 在冷却周期内判定是否可再次发送告警，返回 true 表示可发送且已更新最后触发时间。
func (t *AlertTracker) Try(agentID, status string, now time.Time) bool {
	key := t.key(agentID, status)
	t.mu.Lock()
	defer t.mu.Unlock()

	if last, ok := t.lastAlert[key]; ok && t.cooldown > 0 {
		if now.Sub(last) < t.cooldown {
			return false
		}
	}
	t.lastAlert[key] = now
	return true
}

func (t *AlertTracker) key(agentID, status string) string {
	return strings.ToLower(agentID) + ":" + strings.ToLower(status)
}
