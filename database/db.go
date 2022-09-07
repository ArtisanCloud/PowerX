package database

import (
	"context"
	globalConfig "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/golang-module/carbon"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func SetupDatabase(c *globalConfig.PostgresConfig) (err error) {

	timezone := globalConfig.G_AppConfigure.Timezone
	if timezone == "" {
		timezone = carbon.UTC
	}
	dsn := "host=" + c.Host
	dsn += " user=" + c.Username
	dsn += " password=" + c.Password
	dsn += " dbname=" + c.Database
	dsn += " port=" + c.Port
	dsn += " sslmode=" + c.SSLMode
	dsn += " TimeZone=" + timezone

	logMode := logger.Default.LogMode(logger.Error)
	if c.Debug {
		logMode = logger.Default.LogMode(logger.Info)
	}
	global.G_DBConnection, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                                   logMode,
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	//G_DBConnection.Exec("SET search_path TO " + d.SearchPath)

	if err != nil {
		// throw a exception here
		return err
	}

	//// works with Take
	//result := map[string]interface{}{}
	//G_DBConnection.Table("migrations").Take(&result)
	//fmt.Printf("%+v\r\n", result)

	//println("setup with database")
	//G_DBConnection.DB().SetMaxIdleConns(10)
	//G_DBConnection.DB().SetMaxOpenConns(100)
	//G_DBConnection.DB().SetConnMaxLifetime(time.Hour)

	//G_DBConnection.Logger.LogMode()
	//G_DBConnection.Session(&gorm.Session{NewDB: true})
	//fmt.Printf("init database address:%p\r\n", G_DBConnection)

	return err

}

func GetDBWithContext(ctx context.Context) *gorm.DB {
	//var newCTX context.Context
	return global.G_DBConnection.WithContext(ctx)
}
