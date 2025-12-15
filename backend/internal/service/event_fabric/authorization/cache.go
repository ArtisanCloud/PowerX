package authorization

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

const (
	defaultRedisTTL  = 10 * time.Minute
	defaultLocalTTL  = 10 * time.Second
	defaultCacheSize = 256
)

// Cache 为授权域提供 Redis + 本地 LRU 的缓存封装。
type Cache interface {
	// Get 尝试读取缓存，若 expectedVersion > 0 且缓存版本不匹配，将视为未命中。
	Get(ctx context.Context, key GrantCacheKey, expectedVersion uint64) (*GrantCacheEntry, error)
	// Set 写入缓存，同时刷新 Redis 与本地副本。
	Set(ctx context.Context, key GrantCacheKey, value *GrantCacheEntry) error
	// Invalidate 主动失效缓存，并广播消息使其他实例同步清除。
	Invalidate(ctx context.Context, key GrantCacheKey) error
	// ListenInvalidations 订阅 Redis 失效通知，用于清理本地缓存。
	ListenInvalidations(ctx context.Context) error
}

// GrantCacheKey 组合租户、主体构建缓存键。
type GrantCacheKey struct {
	TenantUUID  string
	SubjectType string
	SubjectID   uuid.UUID
}

// String 返回统一的缓存 key（不含前缀）。
func (k GrantCacheKey) String() string {
	tenant := canonicalTenantKey(k.TenantUUID)
	subjectType := strings.TrimSpace(strings.ToLower(k.SubjectType))
	subjectID := strings.TrimSpace(strings.ToLower(k.SubjectID.String()))
	return fmt.Sprintf("grant:%s:%s:%s", tenant, subjectType, subjectID)
}

// GrantCacheEntry 统一缓存载荷结构。
type GrantCacheEntry struct {
	Version   uint64          `json:"version"`
	ExpiresAt time.Time       `json:"expires_at"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// CacheOptions 控制缓存行为。
type CacheOptions struct {
	RedisClient       *redis.Client
	KeyPrefix         string
	RedisTTL          time.Duration
	LocalTTL          time.Duration
	LocalCapacity     int
	InvalidateChannel string
	Logger            *pxlog.Logger
	Now               func() time.Time
}

type cacheImpl struct {
	redis     *redis.Client
	prefix    string
	ttl       time.Duration
	localTTL  time.Duration
	channel   string
	local     *localCache
	logger    *pxlog.Logger
	now       func() time.Time
	once      sync.Once
	subscribe *redis.PubSub
}

// NewCache 构建缓存实例。
func NewCache(opts CacheOptions) Cache {
	ttl := opts.RedisTTL
	if ttl <= 0 {
		ttl = defaultRedisTTL
	}
	localTTL := opts.LocalTTL
	if localTTL <= 0 || localTTL > ttl {
		localTTL = defaultLocalTTL
	}
	capacity := opts.LocalCapacity
	if capacity <= 0 {
		capacity = defaultCacheSize
	}
	prefix := strings.TrimSpace(opts.KeyPrefix)
	if prefix == "" {
		prefix = "event_fabric:authorization"
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}

	return &cacheImpl{
		redis:    opts.RedisClient,
		prefix:   prefix,
		ttl:      ttl,
		localTTL: localTTL,
		channel:  opts.InvalidateChannel,
		local:    newLocalCache(capacity),
		logger:   logger,
		now:      now,
	}
}

func (c *cacheImpl) Get(ctx context.Context, key GrantCacheKey, expectedVersion uint64) (*GrantCacheEntry, error) {
	if entry := c.localLoad(key); entry != nil {
		if expectedVersion == 0 || entry.Version == expectedVersion {
			return entry, nil
		}
		return nil, nil
	}
	if c.redis == nil {
		return nil, nil
	}
	raw, err := c.redis.Get(ctx, c.fullKey(key)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var payload GrantCacheEntry
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload.ExpiresAt.Before(c.now()) {
		_ = c.redis.Del(ctx, c.fullKey(key)).Err()
		return nil, nil
	}
	if expectedVersion > 0 && payload.Version != expectedVersion {
		return nil, nil
	}
	c.localStore(key, &payload)
	return &payload, nil
}

func (c *cacheImpl) Set(ctx context.Context, key GrantCacheKey, value *GrantCacheEntry) error {
	if value == nil {
		return fmt.Errorf("cache value is nil")
	}

	entry := &GrantCacheEntry{
		Version: value.Version,
		Payload: append(json.RawMessage(nil), value.Payload...),
	}
	expiry := value.ExpiresAt
	if expiry.IsZero() {
		expiry = c.now().Add(c.ttl)
	}
	entry.ExpiresAt = expiry

	c.localStore(key, entry)

	if c.redis == nil {
		return nil
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return c.redis.Set(ctx, c.fullKey(key), data, c.ttl).Err()
}

func (c *cacheImpl) Invalidate(ctx context.Context, key GrantCacheKey) error {
	c.localDelete(key)
	if c.redis == nil {
		return nil
	}
	if err := c.redis.Del(ctx, c.fullKey(key)).Err(); err != nil && err != redis.Nil {
		return err
	}
	if channel := strings.TrimSpace(c.channel); channel != "" {
		payload := key.String()
		if err := c.redis.Publish(ctx, channel, payload).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (c *cacheImpl) ListenInvalidations(ctx context.Context) error {
	if c.redis == nil {
		return nil
	}
	channel := strings.TrimSpace(c.channel)
	if channel == "" {
		return nil
	}

	var err error
	c.once.Do(func() {
		sub := c.redis.Subscribe(ctx, channel)
		// 等待订阅确认，避免错过第一条消息。
		if sub != nil {
			if _, recvErr := sub.ReceiveTimeout(ctx, time.Second); recvErr != nil && recvErr != redis.Nil {
				err = recvErr
			}
		}
		c.subscribe = sub
	})
	if err != nil {
		return err
	}
	if c.subscribe == nil {
		return nil
	}

	ch := c.subscribe.Channel()
	for {
		select {
		case <-ctx.Done():
			return c.subscribe.Close()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			c.localDeleteRaw(msg.Payload)
		}
	}
}

func (c *cacheImpl) fullKey(key GrantCacheKey) string {
	return fmt.Sprintf("%s:%s", c.prefix, key.String())
}

func (c *cacheImpl) localKey(key GrantCacheKey) string {
	return key.String()
}

func (c *cacheImpl) localLoad(key GrantCacheKey) *GrantCacheEntry {
	entry := c.local.Get(c.localKey(key), c.now())
	if entry == nil {
		return nil
	}
	copy := *entry
	if entry.Payload != nil {
		copy.Payload = append(json.RawMessage(nil), entry.Payload...)
	}
	return &copy
}

func (c *cacheImpl) localStore(key GrantCacheKey, entry *GrantCacheEntry) {
	if entry == nil {
		return
	}
	c.local.Set(c.localKey(key), &GrantCacheEntry{
		Version:   entry.Version,
		ExpiresAt: c.now().Add(c.localTTL),
		Payload:   append(json.RawMessage(nil), entry.Payload...),
	})
}

func (c *cacheImpl) localDelete(key GrantCacheKey) {
	c.local.Delete(c.localKey(key))
}

func (c *cacheImpl) localDeleteRaw(raw string) {
	if strings.HasPrefix(raw, c.prefix+":") {
		raw = strings.TrimPrefix(raw, c.prefix+":")
	}
	c.local.Delete(raw)
}

// -------------------------- 本地 LRU 实现 -----------------------------------

type localCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*listNode
	head     *listNode
	tail     *listNode
}

type listNode struct {
	key   string
	value *GrantCacheEntry
	prev  *listNode
	next  *listNode
}

func newLocalCache(capacity int) *localCache {
	return &localCache{
		capacity: capacity,
		items:    make(map[string]*listNode, capacity),
	}
}

func (c *localCache) Get(key string, now time.Time) *GrantCacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, ok := c.items[key]
	if !ok {
		return nil
	}
	if node.value == nil || node.value.ExpiresAt.Before(now) {
		c.removeNode(node)
		delete(c.items, key)
		return nil
	}
	c.moveToFront(node)
	return node.value
}

func (c *localCache) Set(key string, value *GrantCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, ok := c.items[key]; ok {
		node.value = value
		c.moveToFront(node)
		return
	}
	node := &listNode{key: key, value: value}
	c.items[key] = node
	c.addToFront(node)

	if len(c.items) > c.capacity {
		if c.tail != nil {
			delete(c.items, c.tail.key)
			c.removeNode(c.tail)
		}
	}
}

func (c *localCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	node, ok := c.items[key]
	if !ok {
		return
	}
	delete(c.items, key)
	c.removeNode(node)
}

func (c *localCache) addToFront(node *listNode) {
	node.prev = nil
	node.next = c.head
	if c.head != nil {
		c.head.prev = node
	}
	c.head = node
	if c.tail == nil {
		c.tail = node
	}
}

func (c *localCache) moveToFront(node *listNode) {
	if c.head == node {
		return
	}
	c.removeNode(node)
	c.addToFront(node)
}

func (c *localCache) removeNode(node *listNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		c.head = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		c.tail = node.prev
	}
	node.next = nil
	node.prev = nil
}
