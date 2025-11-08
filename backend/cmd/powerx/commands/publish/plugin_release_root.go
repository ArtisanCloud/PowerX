package publish

import (
	"fmt"

	"github.com/spf13/cobra"
)

// pluginReleaseCmd is a placeholder for upcoming plugin release commands.
var pluginReleaseCmd = &cobra.Command{
	Use:   "plugin-release",
	Short: "Interact with plugin release pipelines",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), "Plugin release CLI is under construction. Run with --help for upcoming commands.")
	},
}

func init() {
	Command.AddCommand(pluginReleaseCmd)
}
