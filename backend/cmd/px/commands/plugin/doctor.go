package plugin

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	pluginbootstrap "github.com/ArtisanCloud/PowerX/internal/service/plugin_bootstrap"
	"github.com/spf13/cobra"
)

var doctorOpts = struct {
	apiBase    string
	token      string
	templateID string
}{
	apiBase: "http://localhost:8077/api",
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run environment diagnostics against plugin bootstrap requirements",
	RunE:  runPluginDoctor,
}

func init() {
	Command.AddCommand(doctorCmd)
	doctorCmd.Flags().StringVar(&doctorOpts.apiBase, "api", doctorOpts.apiBase, "PowerX Admin API base URL (e.g. http://localhost:8077/api)")
	doctorCmd.Flags().StringVar(&doctorOpts.token, "token", "", "Bearer token for Admin API authentication")
	doctorCmd.Flags().StringVar(&doctorOpts.templateID, "template", "", "Template identifier (optional)")
}

func runPluginDoctor(cmd *cobra.Command, _ []string) error {
	client := newAPIClient(doctorOpts.apiBase, doctorOpts.token)

	payload := pluginbootstrap.EnvironmentCheckInput{
		TemplateID:      strings.TrimSpace(doctorOpts.templateID),
		RuntimeVersions: collectRuntimeVersions(),
		Tools:           collectToolAvailability(),
	}

	var report pluginbootstrap.EnvironmentCheckReport
	if err := client.post("/internal/plugins/environments/check", payload, &report); err != nil {
		return err
	}

	printDoctorReport(cmd, report)
	if report.Passed {
		return nil
	}
	return fmt.Errorf("environment check failed")
}

func collectRuntimeVersions() map[string]string {
	runtimes := map[string]string{
		"go":  sanitizeVersion(runtime.Version()),
		"git": detectCommandVersion("git", "--version"),
	}
	if node := detectCommandVersion("node", "--version"); node != "" {
		runtimes["node"] = sanitizeVersion(node)
	}
	return runtimes
}

func collectToolAvailability() map[string]bool {
	tools := []string{"git", "go", "node", "npm", "pnpm", "docker"}
	result := make(map[string]bool, len(tools))
	for _, tool := range tools {
		result[tool] = commandAvailable(tool)
	}
	return result
}

func detectCommandVersion(name string, args ...string) string {
	if !commandAvailable(name) {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return sanitizeVersion(out.String())
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func sanitizeVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "go")
	raw = strings.TrimPrefix(raw, "node")
	raw = strings.TrimPrefix(raw, "v")
	if raw == "" {
		return ""
	}
	parts := strings.Fields(raw)
	return strings.TrimPrefix(parts[0], "v")
}

func printDoctorReport(cmd *cobra.Command, report pluginbootstrap.EnvironmentCheckReport) {
	status := "PASSED"
	if !report.Passed {
		status = "FAILED"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Template: %s [%s] — Doctor %s\n", report.Template.Name, report.Template.ID, status)

	if len(report.Issues) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No blocking issues detected.")
		return
	}

	fmt.Fprintln(cmd.OutOrStdout(), "\nIssues:")
	for _, issue := range report.Issues {
		fmt.Fprintf(cmd.OutOrStdout(), "  - [%s] %s: %s\n", strings.ToUpper(issue.Severity), issue.Code, issue.Message)
		if issue.Hint != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "      hint: %s\n", issue.Hint)
		}
	}
}
