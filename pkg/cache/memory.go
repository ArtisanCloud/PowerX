package cache

import (
	"context"
	"sync"
	"time"
)

// MemoryCache 是基于内存的 Cache 实现
type MemoryCache struct {
	store sync.Map
}

// NewMemoryCache 创建一个新的 MemoryCache 实例
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{}
}

// Get 实现 Cache 接口的 Get 方法
func (m *MemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, ok := m.store.Load(key)
	if !ok {
		return nil, nil // 键不存在
	}
	return val.([]byte), nil
}

// Set 实现 Cache 接口的 Set 方法
func (m *MemoryCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	m.store.Store(key, value)
	return nil
}

// Delete 实现 Cache 接口的 Delete 方法
func (m *MemoryCache) Delete(ctx context.Context, key string) error {
	m.store.Delete(key)
	return nil
}

// Exists 实现 Cache 接口的 Exists 方法
func (m *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := m.store.Load(key)
	return ok, nil
}

// Increment 实现 Cache 接口的 Increment 方法
func (m *MemoryCache) Increment(ctx context.Context, key string, value int64) (int64, error) {
	// 这里需要实现自增逻辑
	return 0, nil
}

// Decrement 实现 Cache 接口的 Decrement 方法
func (m *MemoryCache) Decrement(ctx context.Context, key string, value int64) (int64, error) {
	// 这里需要实现自减逻辑
	return 0, nil
}

// Expire 实现 Cache 接口的 Expire 方法
func (m *MemoryCache) Expire(ctx context.Context, key string, expiration time.Duration) error {
	// 这里需要实现过期逻辑
	return nil
}
