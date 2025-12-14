package version

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

const defaultAdminAPI = "http://localhost:8077/api"

var (
	scanCmd = &cobra.Command{
		Use:   "scan",
		Short: "Trigger a version governance scan for a tenant & plugin",
		RunE:  runVersionScan,
	}

	scanOpts = struct {
		api            string
		token          string
		tenantUUID     string
		pluginID       string
		currentVersion string
		targetVersion  string
	}{
		api: defaultAdminAPI,
	}
)

type governanceReport struct {
	TenantUUID         string `json:"tenant_uuid"`
	PluginID           string `json:"plugin_id"`
	CurrentVersion     string `json:"current_version"`
	RecommendedVersion string `json:"recommended_version"`
	RiskLevel          string `json:"risk_level"`
	Status             string `json:"status"`
	GeneratedAt        string `json:"generated_at"`
}

func init() {
	scanCmd.Flags().StringVar(&scanOpts.api, "api", scanOpts.api, "PowerX Admin API base URL (e.g. http://localhost:8077/api)")
	scanCmd.Flags().StringVar(&scanOpts.token, "token", "", "Bearer token for Admin API")
	scanCmd.Flags().StringVar(&scanOpts.tenantUUID, "tenant-uuid", "", "Tenant UUID (required; uses as_tenant_uuid when set)")
	scanCmd.Flags().StringVar(&scanOpts.pluginID, "plugin-id", "", "Plugin identifier (required)")
	scanCmd.Flags().StringVar(&scanOpts.currentVersion, "current-version", "", "Override the current version detected for the tenant")
	scanCmd.Flags().StringVar(&scanOpts.targetVersion, "target-version", "", "Override the recommended target version")
	_ = scanCmd.MarkFlagRequired("tenant-uuid")
	_ = scanCmd.MarkFlagRequired("plugin-id")
}

func runVersionScan(cmd *cobra.Command, _ []string) error {
	client := newAPIClient(scanOpts.api, scanOpts.token)
	payload := map[string]string{
		"pluginId":       strings.TrimSpace(scanOpts.pluginID),
		"currentVersion": strings.TrimSpace(scanOpts.currentVersion),
		"targetVersion":  strings.TrimSpace(scanOpts.targetVersion),
	}
	var report governanceReport
	path := withTenantScope("/internal/version/governance/scan", scanOpts.tenantUUID)
	if err := client.do("POST", path, payload, &report); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Tenant %s plugin %s -> current %s recommended %s\n",
		report.TenantUUID, report.PluginID, emptyFallback(report.CurrentVersion, "n/a"), emptyFallback(report.RecommendedVersion, "n/a"))
	fmt.Fprintf(out, "Risk: %s | Status: %s | GeneratedAt: %s\n",
		emptyFallback(report.RiskLevel, "info"), emptyFallback(report.Status, "generated"), report.GeneratedAt)
	return nil
}

func emptyFallback(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
