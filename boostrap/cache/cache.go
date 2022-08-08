package cache

import (
	"github.com/ArtisanCloud/PowerLibs/v2/cache"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/boostrap/global"
	"github.com/ArtisanCloud/PowerX/config"
)

func SetupCache() (err error) {

	c := config.CacheConn

	options := cache.RedisOptions{
		Addr:       c.Host,
		Password:   c.Password,
		DB:         c.DB,
		SSLEnabled: c.SSLEnabled,
	}

	// use redis as default cache connection
	global.CacheConnection = cache.NewGRedis(&options)

	return nil

}

func GetKeyPrefix() string {
	strAppName := object.Snake(config.AppConfigure.Name, "_")
	return strAppName + "_database_" + strAppName + "_cache:"
}
