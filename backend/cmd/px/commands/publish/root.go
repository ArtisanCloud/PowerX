package publish

import "github.com/spf13/cobra"

// Command is the entrypoint for `px publish` namespace.
var Command = &cobra.Command{
	Use:   "publish",
	Short: "Manage plugin release publishing lifecycle",
	Long:  "Commands for creating, deploying and distributing plugin releases.",
}
