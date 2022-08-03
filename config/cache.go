package config

import (
	"github.com/spf13/viper"
)

type CacheConnection struct {
	MaxIdle        int
	MaxActive      int
	Expiration     int
	TimeoutConnect int
	TimeoutRead    int
	TimeoutWrite   int
	TimeoutIdle    int
}

type RedisConnection struct {
	Connection *CacheConnection
	Protocol   string
	Host       string
	Password   string
	DB         int
	SSLEnabled  bool
	TimeoutConnect int
}

type MemoryConnection struct {
	Connection *CacheConnection
}

var (
	CacheConn *RedisConnection
)

func init() {
	//fmt.Printf("init with cache.go\r\n")
}

func LoadCacheConfig(configPath *string, configName *string, configType *string) (err error) {
	//Info("load cache config", nil)



	err = LoadConfigFile(configPath, configName, configType)
	parseCacheConfig()

	//output := fmt.Sprintf("%+v", *(*DatabaseConn).Connection)
	//Info("current connection"+output, nil)
	return err
}


func parseCacheConfig() {

	//log.Printf("default connection: %v", MapConnection["cache"].(map[string]interface{})["default"])
	//Info("default connection:"+viper.GetString("cache.default"), nil)


	strCacheConnection := "cache.connections.redis."
	viper.SetDefault(strCacheConnection + "timeoutConnect", 3000)
	CacheConn = &RedisConnection{
		&CacheConnection{
			viper.GetInt(strCacheConnection + "maxIdle"),
			viper.GetInt(strCacheConnection + "maxActive"),
			viper.GetInt(strCacheConnection + "expiration"),
			viper.GetInt(strCacheConnection + "timeoutConnect"),
			viper.GetInt(strCacheConnection + "timeoutRead"),
			viper.GetInt(strCacheConnection + "timeoutWrite"),
			viper.GetInt(strCacheConnection + "timeoutIdle"),
		},

		viper.GetString(strCacheConnection + "protocol"),
		viper.GetString(strCacheConnection + "host"),
		viper.GetString(strCacheConnection + "password"),
		viper.GetInt(strCacheConnection + "db"),
		viper.GetBool(strCacheConnection + "sslEnabled"),
		viper.GetInt(strCacheConnection + "timeoutConnect"),
	}
	//Info(viper.GetString(strCacheConnection+"sslMode"), nil)
	//fmt2.Dump("parsed cache connection:", CacheConn)

}
