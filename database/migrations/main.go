package main

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	migrate2 "github.com/ArtisanCloud/PowerX/database/migrations/migrate"
	"os"
)

func main() {

	arrayArgs := os.Args[1:]
	migrate2.NeedRefresh = object.InArray("refresh", arrayArgs)

	//arrayTables := getFoundationTables()
	//arrayTables = appendIndustryTables("education", arrayTables)

	migrate := &migrate2.Migration{}

	err := migrate.Run()

	if err != nil {
		println("migrate error: ", err.Error())
		os.Exit(-1)
	}

	println("migrate done")

	return
}
