# CoreX 事件总线模块

CoreX 事件总线模块提供了统一的事件发布订阅机制，支持本地内存和Redis两种实现方式，适用于单机和分布式场景。

## 特性

- 🚀 **统一接口**: 提供统一的EventBus接口，支持本地和Redis两种实现
- 🔄 **异步处理**: 事件处理器异步执行，不阻塞发布者
- 🏢 **多租户支持**: 内置租户隔离，支持多租户场景
- 🔍 **分布式追踪**: 支持追踪ID传递，便于问题排查
- 🛡️ **错误恢复**: 处理器panic自动恢复，不影响其他处理器
- ⚡ **高性能**: 本地实现基于内存，Redis实现支持跨进程通信
- 🔧 **易于扩展**: 支持动态订阅和取消订阅
- 🎯 **幂等保证**: Redis实现支持事件去重，避免重复处理

## 架构设计

```
┌─────────────────┐    ┌─────────────────┐
│   EventBus      │    │     Event       │
│   Interface     │    │   Structure     │
├─────────────────┤    ├─────────────────┤
│ Subscribe()     │    │ Name            │
│ Publish()       │    │ Payload         │
│ Close()         │    │ Ctx             │
└─────────────────┘    │ ID              │
         │              │ TraceID         │
         │              │ TenantUUID      │
         ▼              └─────────────────┘
┌─────────────────┐
│ Implementation  │
├─────────────────┤
│ LocalEventBus   │ ◄── 内存实现
│ RedisEventBus   │ ◄── Redis实现
└─────────────────┘
```

## 快速开始

### 1. 本地事件总线

```go
package main

import (
    "context"
    "github.com/ArtisanCloud/PowerX/pkg/event_bus"
    "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func main() {
    // 创建本地事件总线
    bus := event_bus.NewLocalEventBus()
    defer bus.Close()

    // 订阅事件
    unsubscribe := bus.Subscribe("user_login", func(event event_bus.Event) error {
        logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "event_bus.example"}), "用户登录: %+v", event)
        return nil
    })
    defer unsubscribe()

    // 发布事件
    ctx := context.WithValue(context.Background(), "tenant_uuid", "tenant-123")
    bus.Publish("user_login", map[string]interface{}{
        "user_id": "user-001",
        "ip":      "192.168.1.1",
    }, ctx)
}
```

### 2. Redis事件总线

```go
package main

import (
    "context"
	"time"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

func main() {
    // 创建Redis事件总线
    config := &event_bus.Config{
        Type: "redis",
        Redis: &event_bus.RedisConfig{
            Addr:     "localhost:6379",
            Password: "",
            DB:       0,
        },
    }

    bus, err := event_bus.NewEventBusWithConfig(config)
    if err != nil {
        panic(err)
    }
    defer bus.Close()

    // 使用方式与本地事件总线相同
    bus.Subscribe("order_created", func(event event_bus.Event) error {
        // 处理订单创建事件
        return nil
    })
}
```

### 3. 工厂模式

```go
package main

import (
    "os"
    "github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

func main() {
    // 设置环境变量
    os.Setenv("CORE_X_EVENT_BUS_TYPE", "local")
    
    // 从环境变量创建
    bus, err := event_bus.NewEventBusFromEnv()
    if err != nil {
        panic(err)
    }
    defer bus.Close()

    // 或者使用默认事件总线
    event_bus.InitDefaultEventBus(&event_bus.Config{Type: "local"})
    defer event_bus.Close()

    // 使用默认实例
    event_bus.Subscribe("global_event", func(event event_bus.Event) error {
        return nil
    })
}
```

## 配置说明

### 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `CORE_X_EVENT_BUS_TYPE` | 事件总线类型 (local/redis) | local |
| `CORE_X_REDIS_ADDR` | Redis地址 | localhost:6379 |
| `CORE_X_REDIS_PASSWORD` | Redis密码 | 空 |

### 配置结构

```go
type Config struct {
    Type  string      `json:"type"`  // "local" 或 "redis"
    Redis *RedisConfig `json:"redis,omitempty"`
}

type RedisConfig struct {
    Addr     string `json:"addr"`     // Redis地址
    Password string `json:"password"` // Redis密码
    DB       int    `json:"db"`       // Redis数据库
}
```

## 事件结构

```go
type Event struct {
    Name       string          `json:"name"`       // 事件名称
    Payload    interface{}     `json:"payload"`    // 事件数据
    Ctx        context.Context `json:"-"`          // 上下文（不序列化）
    ID         string          `json:"id"`         // 事件ID（用于幂等）
    TraceID    string          `json:"trace_id"`   // 追踪ID
    TenantUUID string          `json:"tenant_uuid"`// 租户 UUID
}
```

## 最佳实践

### 1. 事件命名规范

```go
// 推荐使用动词过去式 + 对象的格式
"user_registered"    // 用户已注册
"order_created"      // 订单已创建
"payment_completed"  // 支付已完成
"email_sent"         // 邮件已发送
```

### 2. 错误处理

```go
bus.Subscribe("risky_event", func(event Event) error {
    defer func() {
        if r := recover(); r != nil {
            logger.ErrorF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "event_bus.example"}), "事件处理panic: %v", r)
        }
    }()
    
    // 业务逻辑
    if err := processEvent(event); err != nil {
        logger.ErrorF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "event_bus.example"}), "事件处理失败: %v", err)
        return err
    }
    
    return nil
})
```

### 3. 上下文传递

```go
// 发布事件时传递上下文信息
ctx := context.WithValue(context.Background(), "tenant_uuid", "tenant-123")
ctx = context.WithValue(ctx, "trace_id", "trace-456")
ctx = context.WithValue(ctx, "user_id", "user-789")

bus.Publish("user_action", actionData, ctx)

// 在处理器中获取上下文信息
bus.Subscribe("user_action", func(event Event) error {
    tenantUUID := event.TenantUUID
    traceID := event.TraceID
    
    // 使用上下文信息
    logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "event_bus.example"}), "租户 %s 的用户操作，追踪ID: %s", tenantUUID, traceID)
    return nil
})
```

### 4. 资源清理

```go
func setupEventBus() func() {
    bus := event_bus.NewLocalEventBus()
    
    // 订阅事件
    unsubscribe1 := bus.Subscribe("event1", handler1)
    unsubscribe2 := bus.Subscribe("event2", handler2)
    
    // 返回清理函数
    return func() {
        unsubscribe1()
        unsubscribe2()
        bus.Close()
    }
}

func main() {
    cleanup := setupEventBus()
    defer cleanup()
    
    // 业务逻辑
}
```

## 性能考虑

### 本地事件总线
- **优点**: 延迟极低，无网络开销
- **缺点**: 仅限单进程，重启后事件丢失
- **适用场景**: 单机应用，实时性要求高

### Redis事件总线
- **优点**: 支持跨进程，持久化存储
- **缺点**: 网络延迟，Redis依赖
- **适用场景**: 分布式系统，需要事件持久化

## 监控和调试

```go
// 获取事件总线状态（仅本地实现）
if localBus, ok := bus.(*event_bus.LocalEventBus); ok {
    stats := localBus.GetStats()
    logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "event_bus.example"}), "订阅者数量: %d", stats.SubscriberCount)
    logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "event_bus.example"}), "事件类型: %v", stats.EventTypes)
}
```

## 注意事项

1. **内存泄漏**: 记得调用取消订阅函数或Close()方法
2. **并发安全**: 所有操作都是线程安全的
3. **事件顺序**: 不保证事件处理顺序，如需顺序请使用同步处理
4. **错误处理**: 处理器返回错误不会影响其他处理器执行
5. **Redis连接**: Redis事件总线会自动重连，但需要处理连接失败的情况

## 扩展开发

如需实现其他类型的事件总线（如Kafka、RabbitMQ等），只需实现EventBus接口：

```go
type MyEventBus struct {
    // 自定义字段
}

func (m *MyEventBus) Subscribe(eventType string, handler Handler) (unsubscribe func()) {
    // 实现订阅逻辑
}

func (m *MyEventBus) Publish(eventType string, payload interface{}, ctx context.Context) {
    // 实现发布逻辑
}

func (m *MyEventBus) Close() error {
    // 实现清理逻辑
}
