package app

import (
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/spf13/viper"
)

type AppConfig struct {
	Name        string
	Env         string
	Locale      string
	Timezone    string
	Server      *ServerConfig
	SSH         *SSHConfig
	System      object.HashMap
	SOAServices map[string]interface{}
	Wechat      map[string]interface{}
}

type ServerConfig struct {
	Host string
	Port string
}

type SSHConfig struct {
	PublicKeyFile  string
	PrivateKeyFile string
}

func LoadEnvConfig(configPath *string, configName *string, configType *string) (err error) {
	//Info("load database config", nil)

	err = LoadConfigFile(configPath, configName, configType)

	parseAppConfig()

	//output := fmt.Sprintf("%+v", *(*DatabaseConn).Connection)
	//Info("current connection"+output, nil)
	return err
}

func LoadConfigFile(configPath *string, configName *string, configType *string) (err error) {
	//fmt2.Dump(configPath, configName)
	if configName != nil {
		viper.SetConfigName(*configName)
	} else {
		err = errors.New("config name is nil")
		return err
	}

	if configType == nil {
		viper.SetConfigType("yaml") // REQUIRED if the config file does not have the extension in the name
		viper.SetConfigType("yml")  // REQUIRED if the config file does not have the extension in the name
	} else {
		viper.SetConfigType(*configType)
	}

	viper.AddConfigPath("./configs") // loading config path
	viper.AddConfigPath("./")        // path to look for the config file in
	viper.AddConfigPath("$HOME/")    // call multiple times to add many search paths
	//viper.AddConfigPath(".")        // optionally look for config in the working directory
	if configPath != nil {
		viper.AddConfigPath(*configPath)
	}

	if err := viper.ReadInConfig(); err != nil {
		if err, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error if desired
			panic(fmt.Errorf("Config file not found; ignore error if desired: %s \n", err))

		} else {
			// Config file was found but another error was produced
			panic(fmt.Errorf("Config file was found but another error was produced: %s \n", err))
		}
	}
	//all := viper.AllSettings()
	//fmt2.Dump(all)
	return err
}

func parseAppConfig() {

	//log.Printf("default connection: %v", MapConnection["database"].(map[string]interface{})["default"])
	//Info("default connection:"+viper.GetString("database.default"), nil)

	G_AppConfigure = &AppConfig{
		//viper.GetStringSlice( "Schema"),
		viper.GetString("name"),
		viper.GetString("env"),
		viper.GetString("locale"),
		viper.GetString("timezone"),
		&ServerConfig{
			viper.GetString("server.host"),
			viper.GetString("server.port"),
		},
		&SSHConfig{
			viper.GetString("ssh.public_key_file"),
			viper.GetString("ssh.private_key_file"),
		},
		viper.GetStringMap("system"),
		viper.GetStringMap("soaServices"),
		viper.GetStringMap("wechat"),
	}
	//fmt.Printf("load config:%s \r\n", viper.AllSettings())
	//fmt2.Dump(AppConfigure)
	//fmt.Printf("soa list %s \r\n", AppConfigure.SOAServices["notification"].(map[string]interface{})["url"])

}
