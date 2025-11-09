package instrumentation

import (
	"fmt"
	"strings"
	"time"
)

// AlertRule represents a Prometheus alert configured for plugin release SLAs.
type AlertRule struct {
	Name        string
	Expr        string
	Duration    time.Duration
	Severity    string
	Summary     string
	Description string
	Labels      map[string]string
}

// BuildDefaultAlertSuite returns opinionated alert rules for dashboards and CI smoke checks.
func BuildDefaultAlertSuite(prefix string, rollbackSLA, importSLA time.Duration) []AlertRule {
	ns := strings.TrimSpace(prefix)
	if ns == "" {
		ns = "plugin_release"
	}
	if rollbackSLA <= 0 {
		rollbackSLA = 5 * time.Minute
	}
	if importSLA <= 0 {
		importSLA = 10 * time.Minute
	}
	return []AlertRule{
		{
			Name:     fmt.Sprintf("%s_canary_rollback_sla_breach", ns),
			Expr:     fmt.Sprintf("rate(plugin_release_canary_rollback_total[5m]) > 0 or histogram_quantile(0.95, sum(rate(plugin_release_canary_phase_duration_seconds_bucket{phase=\"rolled_back\"}[5m])) by (le)) > %d", int64(rollbackSLA.Seconds())),
			Duration: 1 * time.Minute,
			Severity: "critical",
			Summary:  "Canary auto rollback triggered",
			Description: fmt.Sprintf("A canary batch exceeded error thresholds or rollback duration > %s. Investigate px publish deploy events and Prometheus dashboard.",
				rollbackSLA),
			Labels: map[string]string{
				"dashboard": "plugin-release",
				"service":   "plugin_release_runtime",
			},
		},
		{
			Name:     fmt.Sprintf("%s_hotload_latency_regression", ns),
			Expr:     "histogram_quantile(0.95, sum(rate(plugin_release_hotload_latency_ms_bucket[10m])) by (le)) > 900000",
			Duration: 5 * time.Minute,
			Severity: "warning",
			Summary:  "Local hotload latency p95 breached 15 minute target",
			Description: "Detects when px-plugin dev --watch workflows exceed the 15-minute SLA. " +
				"Developers may be blocked, check px-plugin logs and tenant cache resets.",
			Labels: map[string]string{
				"dashboard": "plugin-release",
				"service":   "plugin_release_local",
			},
		},
		{
			Name:     fmt.Sprintf("%s_offline_import_stuck", ns),
			Expr:     fmt.Sprintf("sum(plugin_release_distribution_sla_seconds{%s}) / sum(plugin_release_distribution_sla_seconds{status=\"completed\"}) > %d", "status=\"pending\"", int64(importSLA.Seconds())),
			Duration: 10 * time.Minute,
			Severity: "warning",
			Summary:  "Offline package import took longer than expected",
			Description: fmt.Sprintf("Offline imports should complete within %s, otherwise tenants wait for approvals. "+
				"Review offline-import logs and Marketplace listings.", importSLA),
			Labels: map[string]string{
				"dashboard": "plugin-release",
				"service":   "plugin_release_distribution",
			},
		},
	}
}
