package cache

import (
	"github.com/ArtisanCloud/PowerLibs/v2/cache"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	globalBootstrap "github.com/ArtisanCloud/PowerX/boostrap/cache/global"
	"github.com/ArtisanCloud/PowerX/configs/app"
	cacheConfig "github.com/ArtisanCloud/PowerX/configs/cache"
)

func SetupCache() (err error) {

	c := cacheConfig.G_RedisConfig

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
	strAppName := object.Snake(app.G_AppConfigure.Name, "_")
	return strAppName + "_database_" + strAppName + "_cache:"
}
