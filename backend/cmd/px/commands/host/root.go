package host

import "github.com/spf13/cobra"

// Command is the entrypoint for host simulator helpers.
var Command = &cobra.Command{
	Use:   "host",
	Short: "Manage plugin host simulators",
}
