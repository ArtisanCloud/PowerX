package event_bus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// localEventBus 本地内存事件总线实现
type localEventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]Handler
	closed      bool
}

// NewLocalEventBus 创建本地事件总线
func NewLocalEventBus() EventBus {
	return &localEventBus{
		subscribers: make(map[string][]Handler),
	}
}

// Subscribe 订阅事件
func (leb *localEventBus) Subscribe(eventType string, handler Handler) (unsubscribe func()) {
	logCtx := logger.WithLogFields(context.Background(), map[string]interface{}{"module": "event_bus.local"})
	leb.mu.Lock()
	defer leb.mu.Unlock()

	if leb.closed {
		logger.WarnF(logCtx, "事件总线已关闭，无法订阅事件 %s", eventType)
		return func() {}
	}

	leb.subscribers[eventType] = append(leb.subscribers[eventType], handler)
	logger.DebugF(logCtx, "事件 %s 订阅成功，当前订阅者数量: %d", eventType, len(leb.subscribers[eventType]))

	// 返回取消订阅函数
	return func() {
		leb.mu.Lock()
		defer leb.mu.Unlock()

		handlers := leb.subscribers[eventType]
		for i, h := range handlers {
			// 通过函数指针地址比较来找到要删除的handler
			if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
				leb.subscribers[eventType] = append(handlers[:i], handlers[i+1:]...)
				logger.DebugF(logCtx, "事件 %s 取消订阅成功", eventType)
				break
			}
		}

		// 如果没有订阅者了，删除该事件类型
		if len(leb.subscribers[eventType]) == 0 {
			delete(leb.subscribers, eventType)
		}
	}
}

// Publish 发布事件
func (leb *localEventBus) Publish(eventType string, payload interface{}, ctx context.Context) {
	logCtx := ctx
	if logCtx == nil {
		logCtx = logger.WithLogFields(context.Background(), map[string]interface{}{"module": "event_bus.local"})
	}
	leb.mu.RLock()
	defer leb.mu.RUnlock()

	if leb.closed {
		logger.WarnF(logCtx, "事件总线已关闭，无法发布事件 %s", eventType)
		return
	}

	handlers := leb.subscribers[eventType]
	if len(handlers) == 0 {
		logger.DebugF(logCtx, "事件 %s 没有订阅者", eventType)
		return
	}

	// 创建事件对象
	event := Event{
		Name:    eventType,
		Payload: payload,
		Ctx:     ctx,
		ID:      fmt.Sprintf("%d-%s", time.Now().UnixNano(), eventType),
	}

	// 从上下文中提取追踪ID和租户ID
	if ctx != nil {
		if traceID, ok := ctx.Value("trace_id").(string); ok {
			event.TraceID = traceID
		}
		if tenant := tenantUUIDFromContext(ctx); tenant != "" {
			event.TenantUUID = tenant
		}
	}

	logger.DebugF(logCtx, "发布事件 %s，通知 %d 个订阅者", eventType, len(handlers))

	// 异步处理所有handler
	for _, handler := range handlers {
		go func(h Handler, evt Event) {
			defer func() {
				if r := recover(); r != nil {
					logger.ErrorF(logCtx, "事件处理器发生panic: %v", r)
				}
			}()

			if err := h(evt); err != nil {
				logger.ErrorF(logCtx, "事件 %s 处理失败: %v", eventType, err)
			}
		}(handler, event)
	}
}

// Close 关闭事件总线
func (leb *localEventBus) Close() error {
	logCtx := logger.WithLogFields(context.Background(), map[string]interface{}{"module": "event_bus.local"})
	leb.mu.Lock()
	defer leb.mu.Unlock()

	if leb.closed {
		return nil
	}

	leb.closed = true
	leb.subscribers = make(map[string][]Handler)
	logger.Info(logCtx, "本地事件总线已关闭")
	return nil
}

// GetSubscriberCount 获取订阅者数量（用于监控）
func (leb *localEventBus) GetSubscriberCount(eventType string) int {
	leb.mu.RLock()
	defer leb.mu.RUnlock()
	return len(leb.subscribers[eventType])
}

// GetAllEventTypes 获取所有事件类型（用于监控）
func (leb *localEventBus) GetAllEventTypes() []string {
	leb.mu.RLock()
	defer leb.mu.RUnlock()

	var eventTypes []string
	for eventType := range leb.subscribers {
		eventTypes = append(eventTypes, eventType)
	}
	return eventTypes
}
