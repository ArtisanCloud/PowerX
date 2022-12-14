package database

import (
	"github.com/ArtisanCloud/PowerLibs/v2/fmt"
	"github.com/ArtisanCloud/PowerX/boostrap"
	"github.com/ArtisanCloud/PowerX/cmd/database/migrations"
	"github.com/ArtisanCloud/PowerX/cmd/database/seeds"
	"github.com/ArtisanCloud/PowerX/config"
	database2 "github.com/ArtisanCloud/PowerX/database"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"os"
)

func init() {
	var err error
	err = boostrap.InitConfig()
	if err != nil {
		panic(err)
	}

	// Initialize the logger
	err = logger.SetupLog(&config.G_AppConfigure.LogConfig)
	if err != nil {
		panic(err)
	}

	if !config.G_AppConfigure.SystemConfig.Installed {
		return
	}

	err = InitDatabase()
	if err != nil {
		panic(err)
	}

}

func InitDatabase() error {
	var err error
	err = config.LoadDatabaseConfig()
	if err != nil {
		return err
	}

	// Initialize the database
	err = database2.SetupDatabase(config.G_DBConfig)
	if err != nil {
		return err
	}

	return err
}

func RunDatabase(cmd *cobra.Command, command string) {

	err := InitDatabase()
	if err != nil {
		panic(err)
	}

	switch command {
	case "migrate":
		RunMigrate(cmd)

		break
	case "seed":
		RunSeed(cmd)
		break
	default:

	}

}

func RunMigrate(cmd *cobra.Command) {

	//arrayTables := getFoundationTables()
	//arrayTables = appendIndustryTables("education", arrayTables)
	fmt.Dump("globalDatabase.G_DBConnection:", globalDatabase.G_DBConnection.Name())
	err := migrations.Run(globalDatabase.G_DBConnection)

	if err != nil {
		println("migrate error: ", err.Error())
		os.Exit(-1)
	}

	println("migrate done")

	return
}

func RunSeed(cmd *cobra.Command) {

	ctx := &gin.Context{}

	dbSeeder := seeds.NewDatabaseSeeder(ctx)

	err := dbSeeder.Run(ctx)
	if err != nil {
		println(err.Error())
		os.Exit(-1)
	}

	println("seed done")

	return
}
