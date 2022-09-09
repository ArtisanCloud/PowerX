package config

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	osPowerLib "github.com/ArtisanCloud/PowerLibs/v2/os"
	"os"
)

const CONFIG_FILE_LOCATION = "config.yml"
const CONFIG_EXAMPLE_FILE_LOCATION = "config-example.yml"

const COMMAND_ROOT string = "powerx"

const CODE_SHUTDOWN = 1
const CODE_REBOOT = 2

type AppConfig struct {
	Name     string `yaml:"name" json:"name"`
	Env      string `yaml:"env" json:"env"`
	Locale   string `yaml:"locale" json:"locale"`
	Timezone string `yaml:"timezone" json:"timezone"`

	ServerConfig ServerConfig `yaml:"server" json:"server"`
	JWTConfig    JWTConfig    `yaml:"jwt" json:"jwt"`
	SystemConfig SystemConfig `yaml:"system" json:"system"`
	LogConfig    LogConfig    `yaml:"log" json:"log"`

	DatabaseConfig DatabaseConfig `yaml:"database" json:"database"`
	CacheConfig    CacheConfig    `yaml:"cache" json:"cache"`

	WXConfig            WXConfig            `yaml:"wx" json:"wx"`
	WecomConfig         WecomConfig         `yaml:"weCom" json:"weCom"`
	WXMiniProgramConfig WXMiniProgramConfig `yaml:"wx_miniprogram" json:"wx_miniprogram"`
}

type ServerConfig struct {
	Host string `yaml:"host" binding:"required"`
	Port string `yaml:"port" binding:"required"`
}

type JWTConfig struct {
	PublicKey  string `yaml:"public_key" json:"public_key" binding:"required"`
	PrivateKey string `yaml:"private_key" json:"private_key" binding:"required"`
}

type SystemConfig struct {
	Maintenance bool `yaml:"maintenance" json:"maintenance"`
	Installed   bool
}

type LogConfig struct {
	LogPath string `yaml:"log_path" json:"log_path" binding:"required"`
}

func LoadConfigFile(configPath string) (err error) {

	// 检查是否存在配置文件
	if _, err = os.Stat(configPath); os.IsNotExist(err) {

		//  检查config-example是否存在
		if _, err = os.Stat(CONFIG_EXAMPLE_FILE_LOCATION); os.IsNotExist(err) {
			return err
		} else if os.IsPermission(err) {
			return err
		}

		// 从config-example生成新的config文件
		err = osPowerLib.CopyFile(CONFIG_EXAMPLE_FILE_LOCATION, configPath)
		if err != nil {
			return err
		}

	} else if os.IsPermission(err) {
		return err
	}

	G_AppConfigure = &AppConfig{}
	err = object.OpenYMLFile(configPath, G_AppConfigure)
	if err != nil {
		return err
	}

	return err
}

func parseAppConfig() (err error) {

	return err
}
