package cache

import (
	"context"
	"time"
)

// Cache 接口定义了缓存的基本操作
type ICache interface {
	// Get 获取缓存值
	Get(ctx context.Context, key string) ([]byte, error)

	// Set 设置缓存值
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error

	// Delete 删除缓存值
	Delete(ctx context.Context, key string) error

	// Exists 检查缓存是否存在
	Exists(ctx context.Context, key string) (bool, error)

	// Increment 对缓存值进行自增操作
	Increment(ctx context.Context, key string, value int64) (int64, error)

	// Decrement 对缓存值进行自减操作
	Decrement(ctx context.Context, key string, value int64) (int64, error)

	// Expire 设置缓存过期时间
	Expire(ctx context.Context, key string, expiration time.Duration) error
}
