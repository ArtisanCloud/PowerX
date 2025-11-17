package commands

import (
	"os"
	"strings"

	authcmd "github.com/ArtisanCloud/PowerX/cmd/px/commands/auth"
	hostcmd "github.com/ArtisanCloud/PowerX/cmd/px/commands/host"
	plugincmd "github.com/ArtisanCloud/PowerX/cmd/px/commands/plugin"
	publishcmd "github.com/ArtisanCloud/PowerX/cmd/px/commands/publish"
	versioncmd "github.com/ArtisanCloud/PowerX/cmd/px/commands/version"
	"github.com/spf13/cobra"
)

// rootCmd is the base command for px CLI.
var rootCmd = &cobra.Command{
	Use:   "px",
	Short: "PowerX DevOps CLI",
}

// Execute runs the root command.
func Execute(cliVersion string) {
	rootCmd.Version = normalizeVersion(cliVersion)
	rootCmd.SetVersionTemplate("px version {{.Version}}\n")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(publishcmd.Command)
	rootCmd.AddCommand(plugincmd.Command)
	rootCmd.AddCommand(hostcmd.Command)
	rootCmd.AddCommand(versioncmd.Command)
	rootCmd.AddCommand(authcmd.Command)
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "dev"
	}
	return v
}
