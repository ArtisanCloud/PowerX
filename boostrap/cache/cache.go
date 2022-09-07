package cache

import (
	"github.com/ArtisanCloud/PowerLibs/v2/cache"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	globalBootstrap "github.com/ArtisanCloud/PowerX/boostrap/cache/global"
	"github.com/ArtisanCloud/PowerX/config"
)

func SetupCache(c *config.RedisConfig) (err error) {

	options := cache.RedisOptions{
		Addr:       c.Host,
		Password:   c.Password,
		DB:         c.DB,
		SSLEnabled: c.SSLEnabled,
	}

	// use redis as default cache connection
	globalBootstrap.G_CacheConnection = cache.NewGRedis(&options)

	return nil

}

func GetKeyPrefix() string {
	strAppName := object.Snake(config.G_AppConfigure.Name, "_")
	return strAppName + "_database_" + strAppName + "_cache:"
}
