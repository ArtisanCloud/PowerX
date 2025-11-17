package auth

import "github.com/spf13/cobra"

// Command is the root for px auth utilities.
var Command = &cobra.Command{
	Use:   "auth",
	Short: "Authentication helpers for px/px-plugin tooling",
}
