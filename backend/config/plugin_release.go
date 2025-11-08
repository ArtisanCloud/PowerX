package config

// PluginReleaseConfig captures feature gates and runtime defaults for the plugin release lifecycle.
type PluginReleaseConfig struct {
	FeatureFlags  PluginReleaseFeatureFlagsConfig  `yaml:"feature_flags"`
	LocalInstall  PluginReleaseLocalInstallConfig  `yaml:"local_install"`
	Pipeline      PluginReleasePipelineConfig      `yaml:"pipeline"`
	Canary        PluginReleaseCanaryConfig        `yaml:"canary"`
	Distribution  PluginReleaseDistributionConfig  `yaml:"distribution"`
	Observability PluginReleaseObservabilityConfig `yaml:"observability"`
}

// PluginReleaseFeatureFlagsConfig governs high-level capability toggles.
type PluginReleaseFeatureFlagsConfig struct {
	EnableLocalInstall        bool `yaml:"enable_local_install"`
	EnablePipelineDeployment  bool `yaml:"enable_pipeline_deployment"`
	EnableOfflineDistribution bool `yaml:"enable_offline_distribution"`
}

// PluginReleaseLocalInstallConfig controls local hotload behaviour.
type PluginReleaseLocalInstallConfig struct {
	SessionTTLMinutes int `yaml:"session_ttl_minutes"`
	MaxArtifactSizeMB int `yaml:"max_artifact_size_mb"`
}

// PluginReleasePipelineConfig defines release pipeline baselines.
type PluginReleasePipelineConfig struct {
	ApprovalSLAHours      int `yaml:"approval_sla_hours"`
	MaxParallelReleases   int `yaml:"max_parallel_releases"`
	DefaultRollbackNotice int `yaml:"default_rollback_notice_minutes"`
}

// PluginReleaseCanaryConfig provides default thresholds for canary execution.
type PluginReleaseCanaryConfig struct {
	RollbackTimeoutSeconds int `yaml:"rollback_timeout_seconds"`
	DefaultBatchSize       int `yaml:"default_batch_size"`
	MaxBatches             int `yaml:"max_batches"`
}

// PluginReleaseDistributionConfig stores offline distribution defaults.
type PluginReleaseDistributionConfig struct {
	OfflineBucket         string `yaml:"offline_bucket"`
	OfflinePrefix         string `yaml:"offline_prefix"`
	EscalationThreshold   int    `yaml:"escalation_threshold"`
	ArtifactRetentionDays int    `yaml:"artifact_retention_days"`
}

// PluginReleaseObservabilityConfig captures dashboard and alert baselines.
type PluginReleaseObservabilityConfig struct {
	DashboardUID    string                                  `yaml:"dashboard_uid"`
	AlertRulePrefix string                                  `yaml:"alert_rule_prefix"`
	KPITargets      PluginReleaseObservabilityTargetsConfig `yaml:"kpi_targets"`
}

// PluginReleaseObservabilityTargetsConfig defines measurable objectives used by alerting.
type PluginReleaseObservabilityTargetsConfig struct {
	CanRollbackWithinSeconds int `yaml:"can_rollback_within_seconds"`
	HotloadLatencyP95Ms      int `yaml:"hotload_latency_p95_ms"`
}

// DefaultPluginReleaseConfig returns opinionated defaults aligned with the specification.
func DefaultPluginReleaseConfig() PluginReleaseConfig {
	return PluginReleaseConfig{
		FeatureFlags: PluginReleaseFeatureFlagsConfig{
			EnableLocalInstall:        true,
			EnablePipelineDeployment:  true,
			EnableOfflineDistribution: true,
		},
		LocalInstall: PluginReleaseLocalInstallConfig{
			SessionTTLMinutes: 30,
			MaxArtifactSizeMB: 512,
		},
		Pipeline: PluginReleasePipelineConfig{
			ApprovalSLAHours:      24,
			MaxParallelReleases:   4,
			DefaultRollbackNotice: 15,
		},
		Canary: PluginReleaseCanaryConfig{
			RollbackTimeoutSeconds: 300,
			DefaultBatchSize:       10,
			MaxBatches:             5,
		},
		Distribution: PluginReleaseDistributionConfig{
			OfflineBucket:         "plugin-release-artifacts",
			OfflinePrefix:         "packages",
			EscalationThreshold:   2,
			ArtifactRetentionDays: 30,
		},
		Observability: PluginReleaseObservabilityConfig{
			DashboardUID:    "plugin-release",
			AlertRulePrefix: "plugin_release",
			KPITargets: PluginReleaseObservabilityTargetsConfig{
				CanRollbackWithinSeconds: 300,
				HotloadLatencyP95Ms:      600,
			},
		},
	}
}
