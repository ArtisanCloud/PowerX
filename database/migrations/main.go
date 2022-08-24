package main

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
	"os"
)

func main() {

	arrayArgs := os.Args[1:]
	migrate.NeedRefresh = object.InArray("refresh", arrayArgs)

	//arrayTables := getFoundationTables()
	//arrayTables = appendIndustryTables("education", arrayTables)

	err := migrate.Run(globalDatabase.G_DBConnection)

	if err != nil {
		println("migrate error: ", err.Error())
		os.Exit(-1)
	}

	println("migrate done")

	return
}
