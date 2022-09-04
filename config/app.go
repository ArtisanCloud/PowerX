package config

import (
	"gopkg.in/yaml.v3"
	"io/ioutil"
)

type AppConfig struct {
	Name     string `yaml:"name"`
	Env      string `yaml:"env"`
	Locale   string `yaml:"locale"`
	Timezone string `yaml:"timezone"`

	ServerConfig ServerConfig `yaml:"server"`
	JWTConfig    JWTConfig    `yaml:"jwt"`
	SystemConfig SystemConfig `yaml:"system"`
	LogConfig    LogConfig    `yaml:"log"`

	DatabaseConfig DatabaseConfig `yaml:"database"`
	CacheConfig    CacheConfig    `yaml:"cache"`

	WXConfig            WXConfig            `yaml:"wx"`
	WecomConfig         WecomConfig         `yaml:"wecom"`
	WXMiniProgramConfig WXMiniProgramConfig `yaml:"wx_miniprogram"`
}

type ServerConfig struct {
	Host string `yaml:"host" binding:"required"`
	Port string `yaml:"port" binding:"required"`
}

type JWTConfig struct {
	PublicKeyFile  string `yaml:"public_key_file" binding:"required"`
	PrivateKeyFile string `yaml:"private_key_file" binding:"required"`
}

type SystemConfig struct {
	Maintenance bool
}

type LogConfig struct {
	LogPath string `yaml:"log_path" binding:"required"`
}

func LoadEnvConfig(configPath *string) (err error) {

	err = LoadConfigFile(configPath)
	if err != nil {
		return err
	}
	//fmt.Dump(G_AppConfigure)
	//err = parseAppConfig()

	return err
}

func LoadConfigFile(configPath *string) (err error) {

	yamlFile, err := ioutil.ReadFile(*configPath)
	if err != nil {
		return err
	}

	G_AppConfigure = &AppConfig{}
	err = yaml.Unmarshal(yamlFile, G_AppConfigure)
	if err != nil {
		return err
	}

	return err
}

func parseAppConfig() (err error) {

	return err
}
