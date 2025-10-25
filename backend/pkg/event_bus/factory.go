package event_bus

import (
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
)

// Config 事件总线配置
type Config struct {
	Type  string       `json:"type"` // "local" 或 "redis"
	Redis *RedisConfig `json:"redis,omitempty"`
}

// RedisConfig Redis配置
type RedisConfig struct {
	Addr     string `json:"addr"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

// NewEventBusWithConfig 根据配置创建事件总线
func NewEventBusWithConfig(config *Config) (EventBus, error) {
	if config == nil {
		// 默认使用本地事件总线
		return NewLocalEventBus(), nil
	}

	switch config.Type {
	case "redis":
		if config.Redis == nil {
			return nil, fmt.Errorf("Redis配置不能为空")
		}

		opts := &redis.Options{
			Addr:     config.Redis.Addr,
			Password: config.Redis.Password,
			DB:       config.Redis.DB,
		}

		return NewRedisEventBus(opts), nil

	case "local", "":
		return NewLocalEventBus(), nil

	default:
		return nil, fmt.Errorf("不支持的事件总线类型: %s", config.Type)
	}
}

// NewEventBusFromEnv 从环境变量创建事件总线
func NewEventBusFromEnv() (EventBus, error) {
	busType := os.Getenv("CORE_X_EVENT_BUS_TYPE")
	if busType == "" {
		busType = "local"
	}

	config := &Config{
		Type: busType,
	}

	if busType == "redis" {
		redisAddr := os.Getenv("CORE_X_REDIS_ADDR")
		if redisAddr == "" {
			redisAddr = "localhost:6379"
		}

		config.Redis = &RedisConfig{
			Addr:     redisAddr,
			Password: os.Getenv("CORE_X_REDIS_PASSWORD"),
			DB:       0, // 默认DB
		}
	}

	return NewEventBusWithConfig(config)
}

// DefaultEventBus 默认事件总线实例
var DefaultEventBus EventBus

// InitDefaultEventBus 初始化默认事件总线
func InitDefaultEventBus(config *Config) error {
	bus, err := NewEventBusWithConfig(config)
	if err != nil {
		return err
	}
	DefaultEventBus = bus
	return nil
}

// Subscribe 使用默认事件总线订阅事件
func Subscribe(eventType string, handler Handler) (unsubscribe func()) {
	if DefaultEventBus == nil {
		fmt.Println("警告: 默认事件总线未初始化")
		return func() {}
	}
	return DefaultEventBus.Subscribe(eventType, handler)
}

// Publish 使用默认事件总线发布事件
func Publish(event Event) {
	if DefaultEventBus == nil {
		fmt.Println("警告: 默认事件总线未初始化")
		return
	}
	DefaultEventBus.Publish(event.Name, event.Payload, event.Ctx)
}

// Close 关闭默认事件总线
func Close() error {
	if DefaultEventBus == nil {
		return nil
	}
	return DefaultEventBus.Close()
}
