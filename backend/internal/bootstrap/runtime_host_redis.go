package bootstrap

import (
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/redis/go-redis/v9"
)

// runtimeHostRedisClient 显式装配主缓存连接，不推测地址或另建客户端。
func runtimeHostRedisClient(store cache.ICache) redis.UniversalClient {
	provider, ok := store.(interface{ RedisClient() redis.UniversalClient })
	if !ok {
		return nil
	}
	return provider.RedisClient()
}
