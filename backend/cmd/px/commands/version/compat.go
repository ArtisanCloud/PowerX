package version

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	compatCmd = &cobra.Command{
		Use:   "compat",
		Short: "Compatibility guardrail helpers",
	}

	compatCheckCmd = &cobra.Command{
		Use:   "check",
		Short: "Check whether a plugin version is compatible with the host",
		RunE:  runCompatCheck,
	}

	compatCheckOpts = struct {
		api           string
		token         string
		hostVersion   string
		pluginVersion string
		tenantUUID    string
		pluginID      string
	}{
		api: defaultAdminAPI,
	}

	compatExceptionCmd = &cobra.Command{
		Use:   "exception",
		Short: "Create a compatibility exception request",
		RunE:  runCompatException,
	}

	compatExceptionOpts = struct {
		api            string
		token          string
		tenantUUID     string
		pluginID       string
		currentVersion string
		targetVersion  string
		reason         string
	}{
		api: defaultAdminAPI,
	}

	compatApproveCmd = &cobra.Command{
		Use:   "approve",
		Short: "Approve or reject a compatibility exception",
		RunE:  runCompatApprove,
	}

	compatApproveOpts = struct {
		api      string
		token    string
		id       string
		status   string
		reviewer string
		notes    string
	}{
		api: defaultAdminAPI,
	}
)

type compatCheckResponse struct {
	Compatible bool   `json:"compatible"`
	Reason     string `json:"reason"`
	Suggested  string `json:"suggestedVersion"`
}

type compatException struct {
	UUID           string `json:"uuid"`
	TenantUUID     string `json:"tenant_uuid"`
	PluginID       string `json:"plugin_id"`
	CurrentVersion string `json:"current_version"`
	TargetVersion  string `json:"target_version"`
	Status         string `json:"status"`
	Reason         string `json:"reason"`
	Reviewer       string `json:"reviewer"`
	DecisionNotes  string `json:"decision_notes"`
}

func init() {
	compatCmd.AddCommand(compatCheckCmd)
	compatCmd.AddCommand(compatExceptionCmd)
	compatCmd.AddCommand(compatApproveCmd)

	compatCheckCmd.Flags().StringVar(&compatCheckOpts.api, "api", compatCheckOpts.api, "PowerX Admin API base URL")
	compatCheckCmd.Flags().StringVar(&compatCheckOpts.token, "token", "", "Bearer token for Admin API")
	compatCheckCmd.Flags().StringVar(&compatCheckOpts.hostVersion, "host-version", "", "Host manifest version (required)")
	compatCheckCmd.Flags().StringVar(&compatCheckOpts.pluginVersion, "plugin-version", "", "Plugin version to check (required)")
	compatCheckCmd.Flags().StringVar(&compatCheckOpts.tenantUUID, "tenant-uuid", "", "Optional tenant UUID for impersonation")
	compatCheckCmd.Flags().StringVar(&compatCheckOpts.pluginID, "plugin-id", "", "Optional plugin identifier for auditing")
	_ = compatCheckCmd.MarkFlagRequired("host-version")
	_ = compatCheckCmd.MarkFlagRequired("plugin-version")

	compatExceptionCmd.Flags().StringVar(&compatExceptionOpts.api, "api", compatExceptionOpts.api, "PowerX Admin API base URL")
	compatExceptionCmd.Flags().StringVar(&compatExceptionOpts.token, "token", "", "Bearer token for Admin API")
	compatExceptionCmd.Flags().StringVar(&compatExceptionOpts.tenantUUID, "tenant-uuid", "", "Tenant UUID (required; uses as_tenant_uuid)")
	compatExceptionCmd.Flags().StringVar(&compatExceptionOpts.pluginID, "plugin-id", "", "Plugin identifier (required)")
	compatExceptionCmd.Flags().StringVar(&compatExceptionOpts.currentVersion, "current-version", "", "Current version (required)")
	compatExceptionCmd.Flags().StringVar(&compatExceptionOpts.targetVersion, "target-version", "", "Target version requested (required)")
	compatExceptionCmd.Flags().StringVar(&compatExceptionOpts.reason, "reason", "", "Exception reason (required)")
	_ = compatExceptionCmd.MarkFlagRequired("tenant-uuid")
	_ = compatExceptionCmd.MarkFlagRequired("plugin-id")
	_ = compatExceptionCmd.MarkFlagRequired("current-version")
	_ = compatExceptionCmd.MarkFlagRequired("target-version")
	_ = compatExceptionCmd.MarkFlagRequired("reason")

	compatApproveCmd.Flags().StringVar(&compatApproveOpts.api, "api", compatApproveOpts.api, "PowerX Admin API base URL")
	compatApproveCmd.Flags().StringVar(&compatApproveOpts.token, "token", "", "Bearer token for Admin API")
	compatApproveCmd.Flags().StringVar(&compatApproveOpts.id, "id", "", "Exception UUID (required)")
	compatApproveCmd.Flags().StringVar(&compatApproveOpts.status, "status", "approved", "Decision status (approved/rejected)")
	compatApproveCmd.Flags().StringVar(&compatApproveOpts.reviewer, "reviewer", "", "Reviewer identifier")
	compatApproveCmd.Flags().StringVar(&compatApproveOpts.notes, "notes", "", "Decision notes")
	_ = compatApproveCmd.MarkFlagRequired("id")
}

func runCompatCheck(cmd *cobra.Command, _ []string) error {
	client := newAPIClient(compatCheckOpts.api, compatCheckOpts.token)
	payload := map[string]string{
		"hostVersion":   strings.TrimSpace(compatCheckOpts.hostVersion),
		"pluginVersion": strings.TrimSpace(compatCheckOpts.pluginVersion),
		"pluginId":      strings.TrimSpace(compatCheckOpts.pluginID),
	}
	var resp compatCheckResponse
	path := withTenantScope("/internal/version/compat/check", compatCheckOpts.tenantUUID)
	if err := client.do("POST", path, payload, &resp); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if resp.Compatible {
		fmt.Fprintf(out, "Compatible ✅\n")
	} else {
		fmt.Fprintf(out, "Not compatible ❌: %s\n", emptyFallback(resp.Reason, "unknown"))
		if s := strings.TrimSpace(resp.Suggested); s != "" {
			fmt.Fprintf(out, "Suggested version: %s\n", s)
		}
	}
	return nil
}

func runCompatException(cmd *cobra.Command, _ []string) error {
	client := newAPIClient(compatExceptionOpts.api, compatExceptionOpts.token)
	payload := map[string]string{
		"pluginId":       strings.TrimSpace(compatExceptionOpts.pluginID),
		"currentVersion": strings.TrimSpace(compatExceptionOpts.currentVersion),
		"targetVersion":  strings.TrimSpace(compatExceptionOpts.targetVersion),
		"reason":         strings.TrimSpace(compatExceptionOpts.reason),
	}
	var entity compatException
	path := withTenantScope("/internal/version/compat/exception", compatExceptionOpts.tenantUUID)
	if err := client.do("POST", path, payload, &entity); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Exception %s created for %s -> %s (%s)\n", entity.UUID, entity.CurrentVersion, entity.TargetVersion, entity.Status)
	return nil
}

func runCompatApprove(cmd *cobra.Command, _ []string) error {
	client := newAPIClient(compatApproveOpts.api, compatApproveOpts.token)
	payload := map[string]string{
		"id":            strings.TrimSpace(compatApproveOpts.id),
		"status":        strings.TrimSpace(compatApproveOpts.status),
		"reviewer":      strings.TrimSpace(compatApproveOpts.reviewer),
		"decisionNotes": strings.TrimSpace(compatApproveOpts.notes),
	}
	var entity compatException
	if err := client.do("POST", "/internal/version/compat/approve", payload, &entity); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Exception %s updated to %s by %s\n", entity.UUID, entity.Status, emptyFallback(entity.Reviewer, "n/a"))
	if note := strings.TrimSpace(entity.DecisionNotes); note != "" {
		fmt.Fprintf(out, "Notes: %s\n", note)
	}
	return nil
}
