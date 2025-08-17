// Package event_bus 提供事件总线接口和实现
package event_bus

import "context"

// Event 事件结构
type Event struct {
	Name     string          `json:"name"`      // 事件名称
	Payload  interface{}     `json:"payload"`   // 事件数据
	Ctx      context.Context `json:"-"`         // 上下文（不序列化）
	ID       string          `json:"id"`        // 事件ID（用于幂等）
	TraceID  string          `json:"trace_id"`  // 追踪ID
	TenantID string          `json:"tenant_id"` // 租户ID
}

// Handler 事件处理器函数类型
type Handler func(Event) error

// EventBus 事件总线接口
type EventBus interface {
	// Subscribe 订阅事件，返回取消订阅函数
	Subscribe(eventType string, handler Handler) (unsubscribe func())

	// Publish 发布事件
	Publish(eventType string, payload interface{}, ctx context.Context)

	// Close 关闭事件总线
	Close() error
}
