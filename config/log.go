package config

import (
	UBT "github.com/ArtisanCloud/ubt-go"
	"github.com/spf13/viper"
)

var (
	UBTConfig *UBT.ClientOptions
)

func init() {
	//fmt.Printf("init with database.go\r\n")
}

func LoadLogConfig(configPath *string, configName *string, configType *string) (err error) {
	//Info("load database config", nil)

	err = LoadConfigFile(configPath, configName, configType)
	parseLogConfig()

	//output := fmt.Sprintf("%+v", *(*LogConn).Connection)
	//Info("current connection"+output, nil)
	return err
}

func parseLogConfig() {

	//log.Printf("default connection: %v", MapConnection["database"].(map[string]interface{})["default"])
	//Info("default connection:"+viper.GetString("database.default"), nil)

	strLogConn := "log.drivers.elastic_search."

	UBTConfig = &UBT.ClientOptions{
		UBTServer:  viper.GetString(strLogConn + "ubtServer"),
		AppName:    APP_NAME,
		AppVersion: APP_VERSION,
		DebugMode:  viper.GetBool(strLogConn + "debugMode"),
	}

	//Info(viper.GetString(strLogConn+"sslMode"), nil)
	//fmt.Printf("parsed connection: %+v\n", LogConn)

}
