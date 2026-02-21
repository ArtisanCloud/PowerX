package acl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/google/uuid"
)

const (
	aclCachePrefix         = "event:acl"
	defaultACLLocalCacheTTL = 90 * time.Second
	defaultACLRedisCacheTTL = 180 * time.Second
)

// ACLResultCache 定义 ACL 查询结果缓存接口。
type ACLResultCache interface {
	Get(ctx context.Context, key string) (allowed bool, hit bool, err error)
	Set(ctx context.Context, key string, allowed bool) error
	Delete(ctx context.Context, key string) error
}

// BuildACLResultCacheKey 返回统一 ACL 缓存 key：event:acl:{scope}:{topic_id}:{principal}:{action}。
func BuildACLResultCacheKey(scope string, topicID uuid.UUID, principalID string, action string) string {
	normalizedScope := strings.ToLower(strings.TrimSpace(scope))
	if normalizedScope == "" {
		normalizedScope = "unknown"
	}
	normalizedPrincipal := strings.ToLower(strings.TrimSpace(principalID))
	if normalizedPrincipal == "" {
		normalizedPrincipal = "anonymous"
	}
	normalizedAction := strings.ToLower(strings.TrimSpace(action))
	if normalizedAction == "" {
		normalizedAction = "unknown"
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s", aclCachePrefix, normalizedScope, topicID.String(), normalizedPrincipal, normalizedAction)
}

type aclLocalCacheEntry struct {
	allowed   bool
	expiresAt time.Time
}

// LocalACLResultCache 为进程内 ACL 缓存实现。
type LocalACLResultCache struct {
	ttl   time.Duration
	clock func() time.Time
	items sync.Map
}

// NewLocalACLResultCache 创建本地 ACL 缓存。
func NewLocalACLResultCache(ttl time.Duration, clock func() time.Time) *LocalACLResultCache {
	if ttl <= 0 {
		ttl = defaultACLLocalCacheTTL
	}
	if clock == nil {
		clock = time.Now
	}
	return &LocalACLResultCache{ttl: ttl, clock: clock}
}

func (c *LocalACLResultCache) Get(_ context.Context, key string) (bool, bool, error) {
	if c == nil || strings.TrimSpace(key) == "" {
		return false, false, nil
	}
	v, ok := c.items.Load(key)
	if !ok {
		return false, false, nil
	}
	entry, ok := v.(aclLocalCacheEntry)
	if !ok {
		c.items.Delete(key)
		return false, false, nil
	}
	if c.clock().UTC().After(entry.expiresAt) {
		c.items.Delete(key)
		return false, false, nil
	}
	return entry.allowed, true, nil
}

func (c *LocalACLResultCache) Set(_ context.Context, key string, allowed bool) error {
	if c == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	c.items.Store(key, aclLocalCacheEntry{
		allowed:   allowed,
		expiresAt: c.clock().UTC().Add(c.ttl),
	})
	return nil
}

func (c *LocalACLResultCache) Delete(_ context.Context, key string) error {
	if c == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	c.items.Delete(key)
	return nil
}

// RedisACLResultCache 为 Redis ACL 缓存实现。
type RedisACLResultCache struct {
	store cache.ICache
	ttl   time.Duration
}

// NewRedisACLResultCache 创建 Redis ACL 缓存。
func NewRedisACLResultCache(store cache.ICache, ttl time.Duration) *RedisACLResultCache {
	if ttl <= 0 {
		ttl = defaultACLRedisCacheTTL
	}
	return &RedisACLResultCache{store: store, ttl: ttl}
}

func (c *RedisACLResultCache) Get(ctx context.Context, key string) (bool, bool, error) {
	if c == nil || c.store == nil || strings.TrimSpace(key) == "" {
		return false, false, nil
	}
	data, err := c.store.Get(ctx, key)
	if err != nil {
		return false, false, err
	}
	if len(data) == 0 {
		return false, false, nil
	}
	if string(data) == "1" {
		return true, true, nil
	}
	if string(data) == "0" {
		return false, true, nil
	}
	return false, false, nil
}

func (c *RedisACLResultCache) Set(ctx context.Context, key string, allowed bool) error {
	if c == nil || c.store == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	value := "0"
	if allowed {
		value = "1"
	}
	return c.store.Set(ctx, key, value, c.ttl)
}

func (c *RedisACLResultCache) Delete(ctx context.Context, key string) error {
	if c == nil || c.store == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	return c.store.Delete(ctx, key)
}

// LayeredACLResultCache 提供本地 + Redis 两级缓存。
type LayeredACLResultCache struct {
	local  ACLResultCache
	remote ACLResultCache
}

// NewLayeredACLResultCache 创建两级 ACL 缓存。
func NewLayeredACLResultCache(local ACLResultCache, remote ACLResultCache) *LayeredACLResultCache {
	return &LayeredACLResultCache{local: local, remote: remote}
}

func (c *LayeredACLResultCache) Get(ctx context.Context, key string) (bool, bool, error) {
	if c == nil {
		return false, false, nil
	}
	if c.local != nil {
		if allowed, hit, err := c.local.Get(ctx, key); err != nil {
			return false, false, err
		} else if hit {
			return allowed, true, nil
		}
	}
	if c.remote == nil {
		return false, false, nil
	}
	allowed, hit, err := c.remote.Get(ctx, key)
	if err != nil {
		return false, false, err
	}
	if hit && c.local != nil {
		_ = c.local.Set(ctx, key, allowed)
	}
	return allowed, hit, nil
}

func (c *LayeredACLResultCache) Set(ctx context.Context, key string, allowed bool) error {
	if c == nil {
		return nil
	}
	var errs []error
	if c.local != nil {
		if err := c.local.Set(ctx, key, allowed); err != nil {
			errs = append(errs, err)
		}
	}
	if c.remote != nil {
		if err := c.remote.Set(ctx, key, allowed); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *LayeredACLResultCache) Delete(ctx context.Context, key string) error {
	if c == nil {
		return nil
	}
	var errs []error
	if c.local != nil {
		if err := c.local.Delete(ctx, key); err != nil {
			errs = append(errs, err)
		}
	}
	if c.remote != nil {
		if err := c.remote.Delete(ctx, key); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

