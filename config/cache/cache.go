package cache

import (
	"github.com/ArtisanCloud/PowerX/config/app"
	"github.com/spf13/viper"
)

type CacheConfig struct {
	MaxIdle        int
	MaxActive      int
	Expiration     int
	TimeoutConnect int
	TimeoutRead    int
	TimeoutWrite   int
	TimeoutIdle    int
}

func LoadCacheConfig(configPath *string, configName *string, configType *string) (err error) {
	//Info("load cache config", nil)

	err = app.LoadConfigFile(configPath, configName, configType)
	parseCacheConfig()

	//output := fmt.Sprintf("%+v", *(*DatabaseConn).Connection)
	//Info("current connection"+output, nil)
	return err
}

func parseCacheConfig() {

	//log.Printf("default connection: %v", MapConnection["cache"].(map[string]interface{})["default"])
	//Info("default connection:"+viper.GetString("cache.default"), nil)

	strCacheConnection := "cache.connections.redis."
	viper.SetDefault(strCacheConnection+"timeoutConnect", 3000)
	G_RedisConfig = &RedisConfig{
		CacheConfig: &CacheConfig{
			viper.GetInt(strCacheConnection + "maxIdle"),
			viper.GetInt(strCacheConnection + "maxActive"),
			viper.GetInt(strCacheConnection + "expiration"),
			viper.GetInt(strCacheConnection + "timeoutConnect"),
			viper.GetInt(strCacheConnection + "timeoutRead"),
			viper.GetInt(strCacheConnection + "timeoutWrite"),
			viper.GetInt(strCacheConnection + "timeoutIdle"),
		},
		Protocol:       viper.GetString(strCacheConnection + "protocol"),
		Host:           viper.GetString(strCacheConnection + "host"),
		Password:       viper.GetString(strCacheConnection + "password"),
		DB:             viper.GetInt(strCacheConnection + "db"),
		SSLEnabled:     viper.GetBool(strCacheConnection + "sslEnabled"),
		TimeoutConnect: viper.GetInt(strCacheConnection + "timeoutConnect"),
	}
	//Info(viper.GetString(strCacheConnection+"sslMode"), nil)
	//fmt2.Dump("parsed cache connection:", CacheConn)

}
