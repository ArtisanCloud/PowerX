package cache

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func setupRedis() *RedisCache {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // 真实 Redis 服务器地址
		Password: "",               // 如果 Redis 设置了密码，在这里填入
		DB:       0,                // 选择 Redis DB
	})
	return NewRedisCache(client)
}

func TestRedisCache_Get(t *testing.T) {
	cache := setupRedis()
	ctx := context.Background()

	// 先写入数据
	err := cache.Set(ctx, "key1", "value1", 10*time.Second)
	assert.NoError(t, err)

	// 测试 key 存在时返回值
	val, err := cache.Get(ctx, "key1")
	assert.NoError(t, err)
	assert.Equal(t, []byte("value1"), val)

	// 测试 key 不存在时返回 nil
	val, err = cache.Get(ctx, "key2")
	assert.NoError(t, err)
	assert.Nil(t, val)
}

func TestRedisCache_Set(t *testing.T) {
	cache := setupRedis()
	ctx := context.Background()

	err := cache.Set(ctx, "key1", "value1", 10*time.Second)
	assert.NoError(t, err)

	// 验证数据是否真的被写入 Redis
	val, err := cache.Get(ctx, "key1")
	assert.NoError(t, err)
	assert.Equal(t, []byte("value1"), val)
}

func TestRedisCache_SetObject(t *testing.T) {
	cache := setupRedis()
	ctx := context.Background()

	// 定义一个对象
	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	user := User{ID: 1, Name: "Alice"}

	// 序列化为 JSON
	userJSON, err := json.Marshal(user)
	assert.NoError(t, err)

	// 存入 Redis
	err = cache.Set(ctx, "user:1", userJSON, 10*time.Second)
	assert.NoError(t, err)

	// 从 Redis 获取
	val, err := cache.Get(ctx, "user:1")
	assert.NoError(t, err)

	// 反序列化 JSON
	var retrievedUser User
	err = json.Unmarshal(val, &retrievedUser)
	assert.NoError(t, err)

	// 验证数据
	assert.Equal(t, user, retrievedUser)
}

func TestRedisCache_Delete(t *testing.T) {
	cache := setupRedis()
	ctx := context.Background()

	// 先存入数据
	err := cache.Set(ctx, "key1", "value1", 10*time.Second)
	assert.NoError(t, err)

	// 删除数据
	err = cache.Delete(ctx, "key1")
	assert.NoError(t, err)

	// 验证数据是否被删除
	val, err := cache.Get(ctx, "key1")
	assert.NoError(t, err)
	assert.Nil(t, val)
}

func TestRedisCache_Exists(t *testing.T) {
	cache := setupRedis()
	ctx := context.Background()

	// 先存入数据
	err := cache.Set(ctx, "key1", "value1", 10*time.Second)
	assert.NoError(t, err)

	// 测试 key 是否存在
	exists, err := cache.Exists(ctx, "key1")
	assert.NoError(t, err)
	assert.True(t, exists)

	// 测试 key 不存在
	exists, err = cache.Exists(ctx, "key2")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestRedisCache_Increment(t *testing.T) {
	cache := setupRedis()
	ctx := context.Background()

	// 先删除 key 确保测试环境干净
	cache.Delete(ctx, "counter")

	// 递增
	val, err := cache.Increment(ctx, "counter", 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), val)
}

func TestRedisCache_Decrement(t *testing.T) {
	cache := setupRedis()
	ctx := context.Background()

	// 先确保计数器存在
	cache.Set(ctx, "counter", 10, 0)

	// 递减
	val, err := cache.Decrement(ctx, "counter", 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(9), val)
}

func TestRedisCache_Expire(t *testing.T) {
	cache := setupRedis()
	ctx := context.Background()

	// 先存入数据
	err := cache.Set(ctx, "key1", "value1", 0)
	assert.NoError(t, err)

	// 设置过期时间
	err = cache.Expire(ctx, "key1", 2*time.Second)
	assert.NoError(t, err)

	// 等待 3 秒，确保 key 过期
	time.Sleep(3 * time.Second)

	// 验证 key 是否已经过期
	val, err := cache.Get(ctx, "key1")
	assert.NoError(t, err)
	assert.Nil(t, val)
}
