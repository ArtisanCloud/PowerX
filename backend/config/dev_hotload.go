package config

// DevHotloadConfig captures configuration for the Dev API hotload gateway.
type DevHotloadConfig struct {
	FeatureFlags  DevHotloadFeatureFlagsConfig  `yaml:"feature_flags"`
	Sessions      DevHotloadSessionConfig       `yaml:"sessions"`
	Sandbox       DevHotloadSandboxConfig       `yaml:"sandbox"`
	Security      DevHotloadSecurityConfig      `yaml:"security"`
	Observability DevHotloadObservabilityConfig `yaml:"observability"`
}

// DevHotloadFeatureFlagsConfig toggles Dev API exposure and auditing.
type DevHotloadFeatureFlagsConfig struct {
	Enabled          bool   `yaml:"enabled"`
	GatewayFlag      string `yaml:"gateway_flag"`
	SessionAuditFlag string `yaml:"session_audit_flag"`
}

// DevHotloadSessionConfig governs session lifecycle boundaries.
type DevHotloadSessionConfig struct {
	TTLMinutes             int `yaml:"ttl_minutes"`
	MaxConcurrentSessions  int `yaml:"max_concurrent_sessions"`
	CleanupIntervalSeconds int `yaml:"cleanup_interval_seconds"`
}

// DevHotloadSandboxConfig describes host simulator / sandbox resource caps.
type DevHotloadSandboxConfig struct {
	Image          string `yaml:"image"`
	MaxCPUPercent  int    `yaml:"max_cpu_percent"`
	MaxMemoryMB    int    `yaml:"max_memory_mb"`
	WatchFileLimit int    `yaml:"watch_file_limit"`
}

// DevHotloadSecurityConfig captures mTLS / PAT requirements.
type DevHotloadSecurityConfig struct {
	RequireMTLS     bool     `yaml:"require_mtls"`
	AllowedSubjects []string `yaml:"allowed_subjects"`
	PATHeader       string   `yaml:"pat_header"`
	TokenTTLSeconds int      `yaml:"token_ttl_seconds"`
}

// DevHotloadObservabilityConfig defines metrics and audit knobs.
type DevHotloadObservabilityConfig struct {
	MetricsNamespace string `yaml:"metrics_namespace"`
	SSEBufferSize    int    `yaml:"sse_buffer_size"`
	AuditTopic       string `yaml:"audit_topic"`
}

// DefaultDevHotloadConfig returns opinionated defaults for the Dev API.
func DefaultDevHotloadConfig() DevHotloadConfig {
	return DevHotloadConfig{
		FeatureFlags: DevHotloadFeatureFlagsConfig{
			Enabled:          true,
			GatewayFlag:      "PX_DEV_PLUGIN_HOTLOAD",
			SessionAuditFlag: "PX_DEV_SESSION_AUDIT",
		},
		Sessions: DevHotloadSessionConfig{
			TTLMinutes:             15,
			MaxConcurrentSessions:  25,
			CleanupIntervalSeconds: 30,
		},
		Sandbox: DevHotloadSandboxConfig{
			Image:          "ghcr.io/artisancloud/powerx/dev-hotload-host:latest",
			MaxCPUPercent:  25,
			MaxMemoryMB:    512,
			WatchFileLimit: 10000,
		},
		Security: DevHotloadSecurityConfig{
			RequireMTLS:     false,
			AllowedSubjects: []string{},
			PATHeader:       "X-PowerX-Dev-Token",
			TokenTTLSeconds: 900,
		},
		Observability: DevHotloadObservabilityConfig{
			MetricsNamespace: "dev.hotload",
			SSEBufferSize:    128,
			AuditTopic:       "dev.hotload.sessions",
		},
	}
}
