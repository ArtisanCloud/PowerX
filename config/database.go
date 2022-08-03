package config

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/spf13/viper"
)

type DatabaseConnection struct {
	Driver        string
	Url           string
	Host          string
	Port          string
	Database      string
	Username      string
	Password      string
	Charset       string
	Prefix        string
	PrefixIndexes string
}

type MysqlConnection struct {
	Connection *DatabaseConnection
	Collation  string
	Strict     bool
	Engine     string
	Options    []string
	Debug      bool
}

type PostgresConnection struct {
	Connection *DatabaseConnection

	Schemas    object.StringMap
	SearchPath string
	SSLMode    string
	Debug      bool
}

var (
	DatabaseConn *PostgresConnection
)

func init() {
	//fmt.Printf("init with database.go\r\n")
}

func LoadDatabaseConfig(configPath *string, configName *string, configType *string) (err error) {
	//Info("load database config", nil)

	err = LoadConfigFile(configPath, configName, configType)
	parseDatabaseConfig()

	//output := fmt.Sprintf("%+v", *(*DatabaseConn).Connection)
	//Info("current connection"+output, nil)
	return err
}

func parseDatabaseConfig() {

	//log.Printf("default connection: %v", MapConnection["database"].(map[string]interface{})["default"])
	//Info("default connection:"+viper.GetString("database.default"), nil)

	strDatabase := "database."
	strDatabaseConn := strDatabase + "connections.pgsql."
	viper.SetDefault(strDatabase+"debug", false)

	DatabaseConn = &PostgresConnection{
		&DatabaseConnection{
			viper.GetString(strDatabaseConn + "driver"),
			viper.GetString(strDatabaseConn + "url"),
			viper.GetString(strDatabaseConn + "host"),
			viper.GetString(strDatabaseConn + "port"),
			viper.GetString(strDatabaseConn + "database"),
			viper.GetString(strDatabaseConn + "username"),
			viper.GetString(strDatabaseConn + "password"),
			viper.GetString(strDatabaseConn + "charset"),
			viper.GetString(strDatabaseConn + "prefix"),
			viper.GetString(strDatabaseConn + "prefix_indexes"),
		},
		//viper.GetStringSlice(strDatabaseConn + "Schema"),
		viper.GetStringMapString(strDatabaseConn + "schemas"),
		viper.GetString(strDatabaseConn + "search_path"),
		viper.GetString(strDatabaseConn + "sslmode"),
		viper.GetBool(strDatabase + "debug"),
	}
	//Info(viper.GetString(strDatabaseConn+"sslMode"), nil)
	//fmt.Printf("parsed connection: %+v\n", DatabaseConn)

}
