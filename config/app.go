package config

import (
	"gopkg.in/yaml.v3"
	"io/ioutil"
)

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
	WecomConfig         WecomConfig         `yaml:"wecom" json:"wecom"`
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
