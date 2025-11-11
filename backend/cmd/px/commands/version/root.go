package version

import "github.com/spf13/cobra"

// Command is the root for px version utilities.
var Command = &cobra.Command{
	Use:   "version",
	Short: "Version governance & compatibility utilities",
}

func init() {
	Command.AddCommand(scanCmd)
	Command.AddCommand(boardCmd)
	Command.AddCommand(compatCmd)
}
