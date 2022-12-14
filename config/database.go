package config

import (
	"errors"
)

type DatabaseConfig struct {
	Default             string              `yaml:"default" json:"default"`
	Debug               bool                `yaml:"debug" json:"debug"`
	DatabaseConnections DatabaseConnections `yaml:"connections" json:"connections"`
}

type DatabaseConnections struct {
	PostgresConfig PostgresConfig `yaml:"pgsql" json:"pgsql"`
	//MysqlConfig    MysqlConfig    `yaml:"mysql" json:"mysql"`
}

type DatabaseBaseConfig struct {
	Driver        string `yaml:"driver" json:"driver"`
	Url           string `yaml:"url" json:"url"`
	Host          string `yaml:"host" json:"host"`
	Port          string `yaml:"port" json:"port"`
	Database      string `yaml:"database" json:"database"`
	Username      string `yaml:"username" json:"username"`
	Password      string `yaml:"password" json:"password"`
	Charset       string `yaml:"charset" json:"charset"`
	Prefix        string `yaml:"prefix" json:"prefix"`
	PrefixIndexes string `yaml:"prefix_indexes" json:"prefix_indexes"`
}

type MysqlConfig struct {
	Driver        string   `yaml:"driver" json:"driver"`
	Url           string   `yaml:"url" json:"url"`
	Host          string   `yaml:"host" json:"host"`
	Port          string   `yaml:"port" json:"port"`
	Database      string   `yaml:"database" json:"database"`
	Username      string   `yaml:"username" json:"username"`
	Password      string   `yaml:"password" json:"password"`
	Charset       string   `yaml:"charset" json:"charset"`
	Prefix        string   `yaml:"prefix" json:"prefix"`
	PrefixIndexes string   `yaml:"prefix_indexes" json:"prefix_indexes"`
	Collation     string   `yaml:"collation" json:"collation"`
	Strict        bool     `yaml:"strict" json:"strict"`
	Engine        string   `yaml:"engine" json:"engine"`
	Options       []string `yaml:"options" json:"options"`
	Debug         bool     `yaml:"debug" json:"debug"`
}

type PostgresConfig struct {
	Driver        string `yaml:"driver" json:"driver"`
	Url           string `yaml:"url" json:"url"`
	Host          string `yaml:"host" json:"host" binding:"required"`
	Port          string `yaml:"port" json:"port" binding:"required"`
	Database      string `yaml:"database" json:"database" binding:"required"`
	Username      string `yaml:"username" json:"username" binding:"required"`
	Password      string `yaml:"password" json:"password"`
	Charset       string `yaml:"charset" json:"charset"`
	Prefix        string `yaml:"prefix" json:"prefix"`
	PrefixIndexes string `yaml:"prefix_indexes" json:"prefix_indexes"`
	Schemas       struct {
		Default string `yaml:"default" json:"default"`
		Option  string `yaml:"option" json:"option"`
	} `yaml:"schemas" json:"schemas"`
	SearchPath string `yaml:"search_path" json:"search_path"`
	SSLMode    string `yaml:"ssl_mode" json:"ssl_mode"`
	Debug      bool   `yaml:"debug" json:"debug"`
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
		return errors.New("?????????????????????????????????")
	}

	if G_DBConfig.Host == "" {
		return errors.New("???????????????host??????")
	}

	if G_DBConfig.Port == "" {
		return errors.New("???????????????port??????")
	}

	if G_DBConfig.Database == "" {
		return errors.New("???????????????database??????")
	}

	if G_DBConfig.Username == "" {
		return errors.New("???????????????username??????")
	}

	if G_DBConfig.Password == "" {
		return errors.New("???????????????password??????")
	}

	return err
}
