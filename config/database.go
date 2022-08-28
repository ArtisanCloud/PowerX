package config

import (
	"errors"
)

type DatabaseConfig struct {
	Default             string              `yaml:"default"`
	Debug               bool                `yaml:"debug"`
	DatabaseConnections DatabaseConnections `yaml:"connections"`
}

type DatabaseConnections struct {
	PostgresConfig PostgresConfig `yaml:"pgsql"`
	MysqlConfig    MysqlConfig    `yaml:"mysql"`
}

type DatabaseBaseConfig struct {
	Driver        string `yaml:"driver"`
	Url           string `yaml:"url"`
	Host          string `yaml:"host"`
	Port          string `yaml:"port"`
	Database      string `yaml:"database"`
	Username      string `yaml:"username"`
	Password      string `yaml:"password"`
	Charset       string `yaml:"charset"`
	Prefix        string `yaml:"prefix"`
	PrefixIndexes string `yaml:"prefix_indexes"`
}

type MysqlConfig struct {
	Driver        string   `yaml:"driver"`
	Url           string   `yaml:"url"`
	Host          string   `yaml:"host"`
	Port          string   `yaml:"port"`
	Database      string   `yaml:"database"`
	Username      string   `yaml:"username"`
	Password      string   `yaml:"password"`
	Charset       string   `yaml:"charset"`
	Prefix        string   `yaml:"prefix"`
	PrefixIndexes string   `yaml:"prefix_indexes"`
	Collation     string   `yaml:"collation"`
	Strict        bool     `yaml:"strict"`
	Engine        string   `yaml:"engine"`
	Options       []string `yaml:"options"`
	Debug         bool     `yaml:"debug"`
}

type PostgresConfig struct {
	Driver        string `yaml:"driver"`
	Url           string `yaml:"url"`
	Host          string `yaml:"host"`
	Port          string `yaml:"port"`
	Database      string `yaml:"database"`
	Username      string `yaml:"username"`
	Password      string `yaml:"password"`
	Charset       string `yaml:"charset"`
	Prefix        string `yaml:"prefix"`
	PrefixIndexes string `yaml:"prefix_indexes"`
	Schemas       struct {
		Default string
		Option  string
	} `yaml:"schemas"`
	SearchPath string `yaml:"search_path"`
	SSLMode    string `yaml:"ssl_mode"`
	Debug      bool   `yaml:"debug"`
}

func init() {
	//fmt.Printf("init with database.go\r\n")
}

func LoadDatabaseConfig() (err error) {

	G_DBConfig = &G_AppConfigure.DatabaseConfig.DatabaseConnections.PostgresConfig

	err = parseDatabaseConfig()

	return err
}

func parseDatabaseConfig() (err error) {

	if G_DBConfig == nil {
		return errors.New("加载数据库配置对象失败")
	}

	if G_DBConfig.Host == "" {
		return errors.New("数据库缺失host配置")
	}

	if G_DBConfig.Port == "" {
		return errors.New("数据库缺失port配置")
	}

	if G_DBConfig.Database == "" {
		return errors.New("数据库缺失database配置")
	}

	if G_DBConfig.Username == "" {
		return errors.New("数据库缺失username配置")
	}

	if G_DBConfig.Password == "" {
		return errors.New("数据库缺失password配置")
	}

	return err
}
