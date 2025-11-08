package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache 是基于 Go Redis 的 Cache 实现
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache 创建一个新的 RedisCache 实例
func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{
		client: client,
	}
}

// Get 实现 Cache 接口的 Get 方法
func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil // 键不存在
	}
	return val, err
}

// Set 实现 Cache 接口的 Set 方法
func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return r.client.Set(ctx, key, value, expiration).Err()
}

// Delete 实现 Cache 接口的 Delete 方法
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// Exists 实现 Cache 接口的 Exists 方法
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	exists, err := r.client.Exists(ctx, key).Result()
	return exists > 0, err
}

// Increment 实现 Cache 接口的 Increment 方法
func (r *RedisCache) Increment(ctx context.Context, key string, value int64) (int64, error) {
	return r.client.IncrBy(ctx, key, value).Result()
}

// Decrement 实现 Cache 接口的 Decrement 方法
func (r *RedisCache) Decrement(ctx context.Context, key string, value int64) (int64, error) {
	return r.client.DecrBy(ctx, key, value).Result()
}

// Expire 实现 Cache 接口的 Expire 方法
func (r *RedisCache) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return r.client.Expire(ctx, key, expiration).Err()
}
