package cache

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"
)

// MemoryCache 是基于内存的 Cache 实现
type MemoryCache struct {
	store sync.Map
}

type memoryItem struct {
	value     []byte
	expiresAt time.Time
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
	item := val.(memoryItem)
	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		m.store.Delete(key)
		return nil, nil
	}
	return item.value, nil
}

// Set 实现 Cache 接口的 Set 方法
func (m *MemoryCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	raw, err := memoryCacheBytes(value)
	if err != nil {
		return err
	}
	item := memoryItem{value: raw}
	if expiration > 0 {
		item.expiresAt = time.Now().Add(expiration)
	}
	m.store.Store(key, item)
	return nil
}

// Delete 实现 Cache 接口的 Delete 方法
func (m *MemoryCache) Delete(ctx context.Context, key string) error {
	m.store.Delete(key)
	return nil
}

// Exists 实现 Cache 接口的 Exists 方法
func (m *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	raw, err := m.Get(ctx, key)
	return len(raw) > 0, err
}

// Increment 实现 Cache 接口的 Increment 方法
func (m *MemoryCache) Increment(ctx context.Context, key string, value int64) (int64, error) {
	current, err := m.intValue(ctx, key)
	if err != nil {
		return 0, err
	}
	next := current + value
	if err := m.Set(ctx, key, strconv.FormatInt(next, 10), 0); err != nil {
		return 0, err
	}
	return next, nil
}

// Decrement 实现 Cache 接口的 Decrement 方法
func (m *MemoryCache) Decrement(ctx context.Context, key string, value int64) (int64, error) {
	current, err := m.intValue(ctx, key)
	if err != nil {
		return 0, err
	}
	next := current - value
	if err := m.Set(ctx, key, strconv.FormatInt(next, 10), 0); err != nil {
		return 0, err
	}
	return next, nil
}

// Expire 实现 Cache 接口的 Expire 方法
func (m *MemoryCache) Expire(ctx context.Context, key string, expiration time.Duration) error {
	val, ok := m.store.Load(key)
	if !ok {
		return nil
	}
	item := val.(memoryItem)
	if expiration > 0 {
		item.expiresAt = time.Now().Add(expiration)
	} else {
		item.expiresAt = time.Time{}
	}
	m.store.Store(key, item)
	return nil
}

func (m *MemoryCache) intValue(ctx context.Context, key string) (int64, error) {
	raw, err := m.Get(ctx, key)
	if err != nil || len(raw) == 0 {
		return 0, err
	}
	return strconv.ParseInt(string(raw), 10, 64)
}

func memoryCacheBytes(value interface{}) ([]byte, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return json.Marshal(v)
	}
}
