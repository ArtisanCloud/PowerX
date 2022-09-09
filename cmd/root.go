/*
Copyright © 2022 Artisan Cloud <matrix-x@artisan-cloud.com>

*/
package cmd

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/cmd/authorization"
	"github.com/ArtisanCloud/PowerX/cmd/database"
	"github.com/ArtisanCloud/PowerX/cmd/server"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/spf13/cobra"
	"os"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   config.COMMAND_ROOT,
	Short: "powerx-cli 工具包",
	Long: `powerx-cli 工具包: powerx-cli命令行工具包可以提供各种应用，来增强用户的任务需求。
`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//Run: func(cmd *cobra.Command, args []string) {},
}

// versionCmd检查当前版本号
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "获取版本号",
	Long:  `获取所有的版本好`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("v1")
	},
}

// serverCmd 启动web服务
var serverCmd = &cobra.Command{
	Use:   "serve",
	Short: "启动web服务",
	Long:  `启动web服务，默认地址localhost, 默认端口8080, 可在安装服务或者后台系统功能中修改`,
	Run: func(cmd *cobra.Command, args []string) {
		server.LaunchServer(cmd, args)
	},
}

// databaseCmd 数据库功能
var databaseCmd = &cobra.Command{
	Use:   "db",
	Short: "数据库",
	Long:  `数据库执行命令，migrate，dump`,
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) > 1 {
			cmd.Printf("Too many arguments. You can only have one which is the name of your reminder")
			return
		}

		switch args[0] {
		case "migrate":
			database.RunDatabase(cmd, args[0])
			break
		default:

		}

	},
}

// authorizationCmd 授权配置
var authorizationCmd = &cobra.Command{
	Use:   "rbac",
	Short: "角色权限规则配置",
	Long:  `角色权限规则配置，初始化系统，配置系统规则`,
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) > 1 {
			cmd.Printf("Too many arguments. You can only have one which is the name of your reminder")
			return
		}
		authorization.RunAuthorization(cmd, args[0])

	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.PowerX.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(databaseCmd)
	rootCmd.AddCommand(authorizationCmd)

}
