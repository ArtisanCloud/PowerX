package main

import (
	"PowerX/cmd/ctl/database/migrate"
	"PowerX/cmd/ctl/database/seed"
	"PowerX/cmd/ctl/gen"
	"PowerX/internal/config"
	"PowerX/internal/uc"
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/zeromicro/go-zero/core/conf"
	"os"
	"path/filepath"
)

func main() {

	app := &cli.App{
		Name:  "powerx",
		Usage: "PowerX CLI",
		Commands: []*cli.Command{
			{
				Name:  "api-gen",
				Usage: "Generate API CSV",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "dir",
						Aliases: []string{"r"},
						Value:   "./",
						Usage:   "file dir",
					},
				},
				Action: ActionAPIGen,
			},
			{
				Name:    "database",
				Aliases: []string{"db"},
				Usage:   "database usage",

				Subcommands: []*cli.Command{
					{
						Name:  "migrate",
						Usage: "migrate database tables",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "file", Aliases: []string{"f"}},
						},
						Action: ActionMigrate,
					},
					{
						Name:  "seed",
						Usage: "seed database tables",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "file", Aliases: []string{"f"}},
						},
						Action: ActionSeed,
					},
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}

func ActionAPIGen(c *cli.Context) error {
	dir := c.String("dir")
	files, err := gen.FindAPIFiles(dir)
	if err != nil {
		fmt.Println("Error finding .api files:", err)
		return err
	}

	for _, file := range files {
		defer file.Close()
	}

	gen.GenAPICsv(files)

	return nil

}

func ActionMigrate(cCtx *cli.Context) error {

	var configFile = cCtx.String("file")
	if configFile == "" {
		configFile = "etc/powerx.yaml"
	}

	var c config.Config
	conf.MustLoad(configFile, &c)
	c.EtcDir = filepath.Dir(configFile)

	// migrate tables
	m, _ := migrate.NewPowerMigrator(&c)
	m.AutoMigrate()
	powerx, _ := uc.NewPowerXUseCase(&c)
	powerx.AdminAuthorization.Init()

	return nil
}

func ActionSeed(cCtx *cli.Context) error {

	var configFile = cCtx.String("file")
	if configFile == "" {
		configFile = "etc/powerx.yaml"
	}
	var c config.Config
	conf.MustLoad(configFile, &c)
	c.EtcDir = filepath.Dir(configFile)

	// seed tables
	s, _ := seed.NewPowerSeeder(&c)
	_ = s.CreatePowerX()

	return nil
}
