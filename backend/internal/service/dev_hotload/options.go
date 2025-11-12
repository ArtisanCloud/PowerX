package devhotload

import "time"

// Options aggregates runtime configuration for Dev Hotload service.
type Options struct {
	FeatureFlags  FeatureFlags
	Sessions      SessionOptions
	Sandbox       SandboxOptions
	Security      SecurityOptions
	Observability ObservabilityOptions
}

type FeatureFlags struct {
	Enabled          bool
	GatewayFlag      string
	SessionAuditFlag string
}

type SessionOptions struct {
	TTL             time.Duration
	MaxConcurrent   int
	CleanupInterval time.Duration
}

type SandboxOptions struct {
	Image          string
	MaxCPUPercent  int
	MaxMemoryMB    int
	WatchFileLimit int
}

type SecurityOptions struct {
	RequireMTLS     bool
	AllowedSubjects []string
	PATHeader       string
	TokenTTL        time.Duration
}

type ObservabilityOptions struct {
	MetricsNamespace string
	SSEBufferSize    int
	AuditTopic       string
}
