package testutil

import (
	"sync"
	"time"
)

// ManualClock 允许在测试中手动推进时间，便于验证 TTL 行为。
type ManualClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewManualClock 创建手动时钟实例。
func NewManualClock(start time.Time) *ManualClock {
	return &ManualClock{now: start}
}

// Now 返回当前时间。
func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance 推进当前时间。
func (c *ManualClock) Advance(delta time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(delta)
	c.mu.Unlock()
}
