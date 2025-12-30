package event_bus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLocalEventBus(t *testing.T) {
	bus := NewLocalEventBus()
	defer bus.Close()

	// 测试订阅和发布
	received := make(chan Event, 1)
	unsubscribe := bus.Subscribe("test_event", func(event Event) error {
		received <- event
		return nil
	})

	// 发布事件
	ctx := context.WithValue(context.Background(), "tenant_uuid", "test-tenant")
	bus.Publish("test_event", "test payload", ctx)

	// 验证事件接收
	select {
	case event := <-received:
		assert.Equal(t, "test_event", event.Name)
		assert.Equal(t, "test payload", event.Payload)
		assert.Equal(t, "test-tenant", event.TenantUUID)
	case <-time.After(time.Second):
		t.Fatal("事件未收到")
	}

	// 测试取消订阅
	unsubscribe()
	bus.Publish("test_event", "should not receive", ctx)

	select {
	case <-received:
		t.Fatal("取消订阅后不应该收到事件")
	case <-time.After(100 * time.Millisecond):
		// 正常，没有收到事件
	}
}

func TestEventBusFactory(t *testing.T) {
	// 测试本地事件总线创建
	config := &Config{
		Type: "local",
	}

	bus, err := NewEventBusWithConfig(config)
	assert.NoError(t, err)
	assert.NotNil(t, bus)
	defer bus.Close()

	// 测试默认配置
	bus2, err := NewEventBusWithConfig(nil)
	assert.NoError(t, err)
	assert.NotNil(t, bus2)
	defer bus2.Close()
}

func TestFromEnv(t *testing.T) {
	// 测试从环境变量创建（默认本地）
	bus, err := NewEventBusFromEnv()
	assert.NoError(t, err)
	assert.NotNil(t, bus)
	defer bus.Close()
}

func TestMultipleHandlers(t *testing.T) {
	bus := NewLocalEventBus()
	defer bus.Close()

	received1 := make(chan Event, 1)
	received2 := make(chan Event, 1)

	// 订阅同一事件的多个处理器
	bus.Subscribe("multi_event", func(event Event) error {
		received1 <- event
		return nil
	})

	bus.Subscribe("multi_event", func(event Event) error {
		received2 <- event
		return nil
	})

	// 发布事件
	ctx := context.Background()
	bus.Publish("multi_event", "multi payload", ctx)

	// 验证两个处理器都收到事件
	select {
	case event := <-received1:
		assert.Equal(t, "multi_event", event.Name)
		assert.Equal(t, "multi payload", event.Payload)
	case <-time.After(time.Second):
		t.Fatal("处理器1未收到事件")
	}

	select {
	case event := <-received2:
		assert.Equal(t, "multi_event", event.Name)
		assert.Equal(t, "multi payload", event.Payload)
	case <-time.After(time.Second):
		t.Fatal("处理器2未收到事件")
	}
}

func TestEventWithTraceID(t *testing.T) {
	bus := NewLocalEventBus()
	defer bus.Close()

	received := make(chan Event, 1)
	bus.Subscribe("trace_event", func(event Event) error {
		received <- event
		return nil
	})

	// 创建带追踪ID的上下文
	ctx := context.WithValue(context.Background(), "trace_id", "trace-123")
	ctx = context.WithValue(ctx, "tenant_uuid", "tenant-456")

	bus.Publish("trace_event", "trace payload", ctx)

	select {
	case event := <-received:
		assert.Equal(t, "trace_event", event.Name)
		assert.Equal(t, "trace payload", event.Payload)
		assert.Equal(t, "trace-123", event.TraceID)
		assert.Equal(t, "tenant-456", event.TenantUUID)
	case <-time.After(time.Second):
		t.Fatal("事件未收到")
	}
}

func BenchmarkLocalEventBus(b *testing.B) {
	bus := NewLocalEventBus()
	defer bus.Close()

	// 订阅事件
	bus.Subscribe("bench_event", func(event Event) error {
		return nil
	})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish("bench_event", i, ctx)
	}
}
