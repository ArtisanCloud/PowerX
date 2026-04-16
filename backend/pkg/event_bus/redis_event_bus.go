package event_bus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/redis/go-redis/v9"
)

// redisEventBus Redis事件总线实现
type redisEventBus struct {
	client     *redis.Client
	handlersMu sync.RWMutex
	handlers   map[string][]Handler
	dedupeMu   sync.Mutex
	processed  map[string]time.Time
	pubsub     *redis.PubSub
	closed     bool
	closeCh    chan struct{}
}

// NewRedisEventBus 创建Redis事件总线
func NewRedisEventBus(opts *redis.Options) EventBus {
	reb := &redisEventBus{
		client:    redis.NewClient(opts),
		handlers:  make(map[string][]Handler),
		processed: make(map[string]time.Time),
		closeCh:   make(chan struct{}),
	}

	// 启动订阅循环
	go reb.startSubscriberLoop()

	// 启动清理过期记录的定时任务
	go reb.startCleanupLoop()

	return reb
}

// startSubscriberLoop 启动订阅循环
func (reb *redisEventBus) startSubscriberLoop() {
	reb.pubsub = reb.client.PSubscribe(context.Background(), "corex:event:*")
	ch := reb.pubsub.Channel()

	for {
		select {
		case msg := <-ch:
			if msg == nil {
				continue
			}
			reb.handleRedisMessage(msg)
		case <-reb.closeCh:
			return
		}
	}
}

// handleRedisMessage 处理Redis消息
func (reb *redisEventBus) handleRedisMessage(msg *redis.Message) {
	var evt Event
	if err := json.Unmarshal([]byte(msg.Payload), &evt); err != nil {
		logger.ErrorF(context.Background(), "反序列化事件失败: %v", err)
		return
	}

	// 幂等检查
	reb.dedupeMu.Lock()
	if exp, ok := reb.processed[evt.ID]; ok {
		if time.Now().Before(exp) {
			reb.dedupeMu.Unlock()
			return
		}
	}
	reb.processed[evt.ID] = time.Now().Add(30 * time.Second) // 缓存30秒
	reb.dedupeMu.Unlock()

	// 获取处理器
	reb.handlersMu.RLock()
	handlers := make([]Handler, len(reb.handlers[evt.Name]))
	copy(handlers, reb.handlers[evt.Name])
	reb.handlersMu.RUnlock()

	// 异步执行处理器
	for _, handler := range handlers {
		go func(h Handler, event Event) {
			defer func() {
				if r := recover(); r != nil {
					logger.ErrorF(context.Background(), "Redis事件处理器发生panic: %v", r)
				}
			}()

			if err := h(event); err != nil {
				logger.ErrorF(context.Background(), "[redis_event_bus] 事件 %s 处理失败: %v", event.Name, err)
			}
		}(handler, evt)
	}
}

// startCleanupLoop 启动清理循环
func (reb *redisEventBus) startCleanupLoop() {
	ticker := time.NewTicker(time.Minute * 5) // 每5分钟清理一次
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			reb.cleanupExpiredRecords()
		case <-reb.closeCh:
			return
		}
	}
}

// cleanupExpiredRecords 清理过期记录
func (reb *redisEventBus) cleanupExpiredRecords() {
	reb.dedupeMu.Lock()
	defer reb.dedupeMu.Unlock()

	now := time.Now()
	for id, exp := range reb.processed {
		if now.After(exp) {
			delete(reb.processed, id)
		}
	}
}

// Subscribe 订阅事件
func (reb *redisEventBus) Subscribe(eventType string, handler Handler) (unsubscribe func()) {
	reb.handlersMu.Lock()
	defer reb.handlersMu.Unlock()

	if reb.closed {
		logger.WarnF(context.Background(), "Redis事件总线已关闭，无法订阅事件 %s", eventType)
		return func() {}
	}

	reb.handlers[eventType] = append(reb.handlers[eventType], handler)
	logger.DebugF(context.Background(), "Redis事件 %s 订阅成功，当前订阅者数量: %d", eventType, len(reb.handlers[eventType]))

	return func() {
		reb.handlersMu.Lock()
		defer reb.handlersMu.Unlock()

		handlers := reb.handlers[eventType]
		for i, h := range handlers {
			if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
				reb.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
				logger.DebugF(context.Background(), "Redis事件 %s 取消订阅成功", eventType)
				break
			}
		}

		if len(reb.handlers[eventType]) == 0 {
			delete(reb.handlers, eventType)
		}
	}
}

// Publish 发布事件
func (reb *redisEventBus) Publish(eventType string, payload interface{}, ctx context.Context) {
	if reb.closed {
		logger.WarnF(context.Background(), "Redis事件总线已关闭，无法发布事件 %s", eventType)
		return
	}

	// 创建事件对象
	evt := Event{
		Name:    eventType,
		Payload: payload,
		Ctx:     ctx,
		ID:      fmt.Sprintf("%d-%s", time.Now().UnixNano(), eventType),
	}

	// 从上下文中提取追踪ID和租户ID
	if ctx != nil {
		if traceID, ok := ctx.Value("trace_id").(string); ok {
			evt.TraceID = traceID
		}
		if tenant := tenantUUIDFromContext(ctx); tenant != "" {
			evt.TenantUUID = tenant
		}
	}

	// 序列化事件
	b, err := json.Marshal(evt)
	if err != nil {
		logger.ErrorF(context.Background(), "序列化事件失败: %v", err)
		return
	}

	// 发布到Redis
	channel := fmt.Sprintf("corex:event:%s", eventType)
	if err := reb.client.Publish(context.Background(), channel, b).Err(); err != nil {
		logger.ErrorF(context.Background(), "发布Redis事件失败: %v", err)
		return
	}

	logger.DebugF(context.Background(), "Redis事件 %s 发布成功", eventType)
}

// Close 关闭事件总线
func (reb *redisEventBus) Close() error {
	if reb.closed {
		return nil
	}

	reb.closed = true
	close(reb.closeCh)

	if reb.pubsub != nil {
		reb.pubsub.Close()
	}

	if reb.client != nil {
		reb.client.Close()
	}

	reb.handlersMu.Lock()
	reb.handlers = make(map[string][]Handler)
	reb.handlersMu.Unlock()

	reb.dedupeMu.Lock()
	reb.processed = make(map[string]time.Time)
	reb.dedupeMu.Unlock()

	logger.Info(context.Background(), "Redis事件总线已关闭")
	return nil
}
