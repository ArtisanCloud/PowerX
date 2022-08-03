package cache

import (
	"github.com/ArtisanCloud/PowerLibs/v2/cache"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/config"
)

var (
	CacheConnection *cache.GRedis
)

func SetupCache() (err error) {

	c := config.CacheConn
	//fmt.Dump(c)

	options := cache.RedisOptions{
		Addr:       c.Host,
		Password:   c.Password,
		DB:         c.DB,
		SSLEnabled: c.SSLEnabled,
	}

	CacheConnection = cache.NewGRedis(&options)
	//fmt2.Printf("CacheConnection:%+v \r\n", CacheConnection.Pool.String())

	//CacheMapLockers = make(map[string]*sync.Mutex)

	return nil

}

func GetKeyPrefix() string {
	strAppName := object.Snake(config.AppConfigure.Name, "_")
	return strAppName + "_database_" + strAppName + "_cache:"
}
