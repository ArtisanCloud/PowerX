package main

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/cmd/database/migrations/migrate"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
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
