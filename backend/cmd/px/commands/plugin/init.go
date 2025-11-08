package plugin

import (
	"fmt"
	"strings"

	pluginbootstrap "github.com/ArtisanCloud/PowerX/internal/service/plugin_bootstrap"
	"github.com/spf13/cobra"
)

var initOpts = struct {
	apiBase    string
	token      string
	templateID string
	pluginID   string
	modulePath string
	cliVersion string
	gitHost    string
	owners     []string
}{
	apiBase:    "http://localhost:8077/api",
	cliVersion: "dev",
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Validate plugin bootstrap parameters via CoreX API",
	RunE:  runPluginInit,
}

func init() {
	Command.AddCommand(initCmd)

	initCmd.Flags().StringVar(&initOpts.apiBase, "api", initOpts.apiBase, "PowerX Admin API base URL (e.g. http://localhost:8077/api)")
	initCmd.Flags().StringVar(&initOpts.token, "token", "", "Bearer token for Admin API authentication")
	initCmd.Flags().StringVar(&initOpts.templateID, "template", "", "Template identifier (defaults to server configuration)")
	initCmd.Flags().StringVar(&initOpts.pluginID, "plugin-id", "", "Plugin identifier (e.g. com.powerx.demo)")
	initCmd.Flags().StringVar(&initOpts.modulePath, "module", "", "Override backend module path")
	initCmd.Flags().StringVar(&initOpts.cliVersion, "cli-version", initOpts.cliVersion, "px-plugin CLI version presented to validator")
	initCmd.Flags().StringVar(&initOpts.gitHost, "git-host", "", "Git host or VCS endpoint used for health checks")
	initCmd.Flags().StringSliceVar(&initOpts.owners, "owner", nil, "List of owner emails (optional, repeatable)")

	_ = initCmd.MarkFlagRequired("plugin-id")
}

func runPluginInit(cmd *cobra.Command, _ []string) error {
	client := newAPIClient(initOpts.apiBase, initOpts.token)
	payload := pluginbootstrap.BootstrapValidateInput{
		TemplateID: strings.TrimSpace(initOpts.templateID),
		PluginID:   strings.TrimSpace(initOpts.pluginID),
		CLIVersion: strings.TrimSpace(initOpts.cliVersion),
		ModulePath: strings.TrimSpace(initOpts.modulePath),
		GitHost:    strings.TrimSpace(initOpts.gitHost),
		Owners:     initOpts.owners,
	}

	var result pluginbootstrap.BootstrapValidateResult
	if err := client.post("/internal/plugins/bootstrap/validate", payload, &result); err != nil {
		return err
	}

	printBootstrapResult(cmd, result)

	if strings.EqualFold(result.Status, "ready") {
		return nil
	}
	return fmt.Errorf("bootstrap validation blocked; resolve the issues above before retrying")
}

func printBootstrapResult(cmd *cobra.Command, result pluginbootstrap.BootstrapValidateResult) {
	fmt.Fprintf(cmd.OutOrStdout(), "Template: %s (%s)\n", result.Template.Name, result.Template.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "Plugin ID: %s\nModule Path: %s\nStatus: %s\n", result.PluginID, result.ModulePath, strings.ToUpper(result.Status))

	if len(result.Issues) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nIssues:")
		for _, issue := range result.Issues {
			fmt.Fprintf(cmd.OutOrStdout(), "  - [%s] %s: %s\n", strings.ToUpper(issue.Severity), issue.Code, issue.Message)
			if issue.Hint != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "      hint: %s\n", issue.Hint)
			}
		}
	}

	if len(result.Recommendations) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nRecommendations:")
		for _, rec := range result.Recommendations {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", rec)
		}
	}
}
