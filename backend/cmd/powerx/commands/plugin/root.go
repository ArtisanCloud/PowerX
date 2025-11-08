package plugin

import "github.com/spf13/cobra"

// Command is the entrypoint for `powerx plugin` namespace.
var Command = &cobra.Command{
	Use:   "plugin",
	Short: "Developer tooling for plugin hotload and lifecycle operations",
	Long:  "Commands that help plugin developers interact with hotload sessions and runtime utilities.",
}
