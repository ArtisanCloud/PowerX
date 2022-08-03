package database

import (
	"context"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/golang-module/carbon"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
)

var (
	DBConnection *gorm.DB
)

func SetupDatabase() (err error) {

	d := config.DatabaseConn
	c := d.Connection
	timezone := config.AppConfigure.Timezone
	if timezone == "" {
		timezone = carbon.UTC
	}
	dsn := "host=" + c.Host
	dsn += " user=" + c.Username
	dsn += " password=" + c.Password
	dsn += " dbname=" + c.Database
	dsn += " port=" + c.Port
	dsn += " sslmode=" + d.SSLMode
	dsn += " TimeZone=" + timezone

	logMode := logger.Default.LogMode(logger.Error)
	if config.DatabaseConn.Debug {
		logMode = logger.Default.LogMode(logger.Info)
	}
	DBConnection, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                                   logMode,
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	//DBConnection.Exec("SET search_path TO " + d.SearchPath)

	if err != nil {
		// throw a exception here
		log.Fatal("Database init error: ", err)
		return
	}

	//// works with Take
	//result := map[string]interface{}{}
	//DBConnection.Table("migrations").Take(&result)
	//fmt.Printf("%+v\r\n", result)

	//println("setup with database")
	//DBConnection.DB().SetMaxIdleConns(10)
	//DBConnection.DB().SetMaxOpenConns(100)
	//DBConnection.DB().SetConnMaxLifetime(time.Hour)

	//DBConnection.Logger.LogMode()
	//DBConnection.Session(&gorm.Session{NewDB: true})
	//fmt.Printf("init database address:%p\r\n", DBConnection)

	return err

}

//func NewContext()  context.Context {
//	return context.Context{}
//}
func GetDBWithContext(ctx context.Context) *gorm.DB {
	//var newCTX context.Context
	return DBConnection.WithContext(ctx)
}
