package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache 是基于 Go Redis 的 Cache 实现
type RedisCache struct {
	client    *redis.Client
	namespace string
}

func (r *RedisCache) key(key string) string { return r.namespace + key }

// NewRedisCache 创建一个新的 RedisCache 实例
func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{
		client: client,
	}
}

// Get 实现 Cache 接口的 Get 方法
func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.client.Get(ctx, r.key(key)).Bytes()
	if err == redis.Nil {
		return nil, nil // 键不存在
	}
	return val, err
}

// Set 实现 Cache 接口的 Set 方法
func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return r.client.Set(ctx, r.key(key), value, expiration).Err()
}

// Delete 实现 Cache 接口的 Delete 方法
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, r.key(key)).Err()
}

// Exists 实现 Cache 接口的 Exists 方法
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	exists, err := r.client.Exists(ctx, r.key(key)).Result()
	return exists > 0, err
}

// Increment 实现 Cache 接口的 Increment 方法
func (r *RedisCache) Increment(ctx context.Context, key string, value int64) (int64, error) {
	return r.client.IncrBy(ctx, r.key(key), value).Result()
}

// Decrement 实现 Cache 接口的 Decrement 方法
func (r *RedisCache) Decrement(ctx context.Context, key string, value int64) (int64, error) {
	return r.client.DecrBy(ctx, r.key(key), value).Result()
}

// Expire 实现 Cache 接口的 Expire 方法
func (r *RedisCache) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return r.client.Expire(ctx, r.key(key), expiration).Err()
}
