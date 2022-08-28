package cache

import "github.com/ArtisanCloud/PowerLibs/v2/cache"

type RedisConfig struct {
	cache.CacheInterface
	CacheConfig    *CacheConfig
	Protocol       string
	Host           string
	Password       string
	DB             int
	SSLEnabled     bool
	TimeoutConnect int
}
