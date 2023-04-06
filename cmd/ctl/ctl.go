package main

import (
	"PowerX/cmd/ctl/gen"
	"fmt"
	"github.com/urfave/cli/v2"
	"os"
)

func main() {
	app := &cli.App{
		Name:  "powerxctl",
		Usage: "PowerX CLI",
		Commands: []*cli.Command{
			{
				Name:  "apigen",
				Usage: "Generate API CSV",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "dir",
						Aliases: []string{"r"},
						Value:   "./",
						Usage:   "file dir",
					},
				},
				Action: func(c *cli.Context) error {
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
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}
