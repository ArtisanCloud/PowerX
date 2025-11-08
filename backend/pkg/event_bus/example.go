package event_bus

import (
	"context"
	"fmt"
	"time"
)

// ExampleUsage 展示事件总线的使用方法
func ExampleUsage() {
	// 1. 创建本地事件总线
	bus := NewLocalEventBus()
	defer bus.Close()

	// 2. 订阅用户注册事件
	unsubscribeUserReg := bus.Subscribe("user_registered", func(event Event) error {
		fmt.Printf("用户注册事件: %+v\n", event)

		// 从事件中获取用户信息
		if userInfo, ok := event.Payload.(map[string]interface{}); ok {
			fmt.Printf("新用户: %s, 邮箱: %s\n", userInfo["name"], userInfo["email"])
		}

		return nil
	})

	// 3. 订阅订单创建事件
	unsubscribeOrderCreated := bus.Subscribe("order_created", func(event Event) error {
		fmt.Printf("订单创建事件: 租户=%s, 追踪ID=%s\n", event.TenantID, event.TraceID)

		if orderInfo, ok := event.Payload.(map[string]interface{}); ok {
			fmt.Printf("订单ID: %s, 金额: %v\n", orderInfo["order_id"], orderInfo["amount"])
		}

		return nil
	})

	// 4. 发布用户注册事件
	ctx := context.WithValue(context.Background(), "tenant_id", "tenant-123")
	ctx = context.WithValue(ctx, "trace_id", "trace-456")

	bus.Publish("user_registered", map[string]interface{}{
		"name":  "张三",
		"email": "zhangsan@example.com",
		"id":    "user-001",
	}, ctx)

	// 5. 发布订单创建事件
	bus.Publish("order_created", map[string]interface{}{
		"order_id": "order-001",
		"amount":   99.99,
		"user_id":  "user-001",
	}, ctx)

	// 等待事件处理完成
	time.Sleep(100 * time.Millisecond)

	// 6. 取消订阅
	unsubscribeUserReg()
	unsubscribeOrderCreated()

	fmt.Println("事件总线示例完成")
}

// ExampleRedisEventBus 展示Redis事件总线的使用
func ExampleRedisEventBus() {
	// 创建Redis事件总线配置
	config := &Config{
		Type: "redis",
		Redis: &RedisConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		},
	}

	// 创建Redis事件总线
	bus, err := NewEventBusWithConfig(config)
	if err != nil {
		fmt.Printf("创建Redis事件总线失败: %v\n", err)
		return
	}
	defer bus.Close()

	// 订阅事件
	bus.Subscribe("redis_test", func(event Event) error {
		fmt.Printf("Redis事件: %+v\n", event)
		return nil
	})

	// 发布事件
	ctx := context.Background()
	bus.Publish("redis_test", "Hello Redis!", ctx)

	// 等待处理
	time.Sleep(time.Second)
	fmt.Println("Redis事件总线示例完成")
}

// ExampleFactoryPattern 展示工厂模式的使用
func ExampleFactoryPattern() {
	// 从环境变量创建事件总线
	bus, err := NewEventBusFromEnv()
	if err != nil {
		fmt.Printf("创建事件总线失败: %v\n", err)
		return
	}
	defer bus.Close()

	// 初始化默认事件总线
	if err := InitDefaultEventBus(&Config{Type: "local"}); err != nil {
		fmt.Printf("初始化默认事件总线失败: %v\n", err)
		return
	}
	defer Close()

	// 使用默认事件总线
	Subscribe("global_event", func(event Event) error {
		fmt.Printf("全局事件: %+v\n", event)
		return nil
	})

	// 发布全局事件
	Publish(Event{
		Name:    "global_event",
		Payload: "全局消息",
		Ctx:     context.Background(),
	})

	time.Sleep(100 * time.Millisecond)
	fmt.Println("工厂模式示例完成")
}

// ExampleErrorHandling 展示错误处理
func ExampleErrorHandling() {
	bus := NewLocalEventBus()
	defer bus.Close()

	// 订阅会出错的处理器
	bus.Subscribe("error_event", func(event Event) error {
		fmt.Printf("处理事件: %+v\n", event)
		return fmt.Errorf("模拟处理错误")
	})

	// 订阅正常的处理器
	bus.Subscribe("error_event", func(event Event) error {
		fmt.Printf("正常处理事件: %+v\n", event)
		return nil
	})

	// 发布事件
	ctx := context.Background()
	bus.Publish("error_event", "测试错误处理", ctx)

	time.Sleep(100 * time.Millisecond)
	fmt.Println("错误处理示例完成")
}

// ExampleMultiTenant 展示多租户场景
func ExampleMultiTenant() {
	bus := NewLocalEventBus()
	defer bus.Close()

	// 订阅所有租户的事件
	bus.Subscribe("tenant_action", func(event Event) error {
		fmt.Printf("租户 %s 执行了操作: %v\n", event.TenantID, event.Payload)
		return nil
	})

	// 租户A的操作
	ctxA := context.WithValue(context.Background(), "tenant_id", "tenant-A")
	bus.Publish("tenant_action", "创建用户", ctxA)

	// 租户B的操作
	ctxB := context.WithValue(context.Background(), "tenant_id", "tenant-B")
	bus.Publish("tenant_action", "删除订单", ctxB)

	time.Sleep(100 * time.Millisecond)
	fmt.Println("多租户示例完成")
}
