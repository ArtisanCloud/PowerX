package testutil

import (
	"context"
	"sync"
	"time"

	discovery "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
)

// InMemoryDiscoveryCache 提供简单的内存缓存实现，模拟 Redis 行为。
type InMemoryDiscoveryCache struct {
	mu    sync.RWMutex
	items map[discovery.CacheKey]cacheItem
}

type cacheItem struct {
	snapshot discovery.Snapshot
	ttl      time.Duration
}

// NewInMemoryDiscoveryCache 创建内存缓存。
func NewInMemoryDiscoveryCache() *InMemoryDiscoveryCache {
	return &InMemoryDiscoveryCache{
		items: make(map[discovery.CacheKey]cacheItem),
	}
}

func (c *InMemoryDiscoveryCache) Get(_ context.Context, key discovery.CacheKey) (discovery.Snapshot, bool, error) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return discovery.Snapshot{}, false, nil
	}
	return item.snapshot, true, nil
}

func (c *InMemoryDiscoveryCache) Set(_ context.Context, snapshot discovery.Snapshot, ttl time.Duration) error {
	c.mu.Lock()
	c.items[snapshot.CacheKey()] = cacheItem{
		snapshot: snapshot,
		ttl:      ttl,
	}
	c.mu.Unlock()
	return nil
}

func (c *InMemoryDiscoveryCache) Delete(_ context.Context, key discovery.CacheKey) error {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
	return nil
}
