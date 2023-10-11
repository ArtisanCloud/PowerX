package main

import (
	"PowerX/cmd/devctl/plugin"
	"PowerX/pkg/pluginx"
	"fmt"
	"github.com/urfave/cli/v2"
	"github.com/zeromicro/go-zero/tools/goctl/api/parser"
	"log"
	"os"
)

func main() {
	app := &cli.App{
		Name:  "powerd",
		Usage: "Manage plugins for PowerXDashboard",
		Commands: []*cli.Command{
			{
				Name:  "plugin",
				Usage: "plugin usage",
				Subcommands: []*cli.Command{
					{
						Name:  "build",
						Usage: "build plugin frontend",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "dir", Aliases: []string{"d"}, Value: "./plugins", Usage: "file dir"},
							&cli.StringFlag{Name: "api", Aliases: []string{"a"}, Value: "/api/plugin", Usage: "api base url"},
						},
						Action: func(c *cli.Context) error {
							loader := pluginx.NewLoader(c.String("dir"), &pluginx.BuildLoaderConfig{
								MainAPIEndpoint: c.String("api"),
							})
							err := loader.CheckEnvDependency()
							if err != nil {
								return err
							}
							err = loader.UnArchives()
							if err != nil {
								return err
							}
							err = loader.BuildPluginFrontend(pluginx.BuildPluginFrontendOptions{
								ReDownload: true,
							})
							if err != nil {
								return err
							}
							return nil
						},
					},
					{
						Name:  "gen-client-api",
						Usage: "generate client api",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "dir", Aliases: []string{"d"}, Value: "./plugin-client", Usage: "target dir"},
						},
						Action: func(c *cli.Context) error {
							filePath := "api/powerx.api"
							api, err := parser.Parse(filePath)
							if err != nil {
								fmt.Println("Error parsing .api file:", err)
								return err
							}

							err = plugin.GenerateClientCode(api, c.String("dir"))
							if err != nil {
								fmt.Println("Error generating client code:", err)
								return err
							}
							return nil
						},
					},
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
