package commands

import (
	"os"

	plugincmd "github.com/ArtisanCloud/PowerX/cmd/powerx/commands/plugin"
	publishcmd "github.com/ArtisanCloud/PowerX/cmd/powerx/commands/publish"
	"github.com/spf13/cobra"
)

// rootCmd is the base command for powerx CLI.
var rootCmd = &cobra.Command{
	Use:   "powerx",
	Short: "PowerX DevOps CLI",
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(publishcmd.Command)
	rootCmd.AddCommand(plugincmd.Command)
}
