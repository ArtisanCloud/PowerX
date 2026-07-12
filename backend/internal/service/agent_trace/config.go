package agent_trace

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Enabled          bool
	LocalEnabled     bool
	LocalDir         string
	LokiEnabled      bool
	LokiEndpoint     string
	ArtifactPolicy   string
	MaxArtifactBytes int64
	RetentionDays    int
	LokiLabels       map[string]string
}

func DefaultConfig() Config {
	return Config{
		Enabled:          true,
		LocalEnabled:     true,
		LocalDir:         DefaultLocalDir,
		LokiEnabled:      false,
		ArtifactPolicy:   DefaultArtifactPolicy,
		MaxArtifactBytes: DefaultMaxArtifactBytes,
		LokiLabels: map[string]string{
			"service":   "powerx-agent",
			"component": "agent-runtime",
		},
	}
}

func ConfigFromEnv() Config {
	cfg := DefaultConfig()
	cfg.Enabled = envBool("AGENT_TRACE_ENABLED", cfg.Enabled)
	cfg.LocalEnabled = envBool("AGENT_TRACE_LOCAL_ENABLED", cfg.LocalEnabled)
	cfg.LokiEnabled = envBool("AGENT_TRACE_LOKI_ENABLED", cfg.LokiEnabled)
	if v := strings.TrimSpace(os.Getenv("AGENT_TRACE_LOCAL_DIR")); v != "" {
		cfg.LocalDir = v
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_TRACE_LOKI_ENDPOINT")); v != "" {
		cfg.LokiEndpoint = v
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_TRACE_ARTIFACT_POLICY")); v != "" {
		cfg.ArtifactPolicy = v
	}
	if v := envInt64("AGENT_TRACE_MAX_ARTIFACT_BYTES", cfg.MaxArtifactBytes); v > 0 {
		cfg.MaxArtifactBytes = v
	}
	if v := envInt("AGENT_TRACE_RETENTION_DAYS", cfg.RetentionDays); v > 0 {
		cfg.RetentionDays = v
	}
	cfg.LokiLabels = mergeLabels(cfg.LokiLabels, parseLabelEnv(os.Getenv("AGENT_TRACE_LOKI_LABELS")))
	return cfg
}

func (c Config) normalized() Config {
	out := c
	if strings.TrimSpace(out.LocalDir) == "" {
		out.LocalDir = DefaultLocalDir
	}
	if strings.TrimSpace(out.ArtifactPolicy) == "" {
		out.ArtifactPolicy = DefaultArtifactPolicy
	}
	if out.MaxArtifactBytes <= 0 {
		out.MaxArtifactBytes = DefaultMaxArtifactBytes
	}
	out.LokiLabels = mergeLabels(map[string]string{
		"service":   "powerx-agent",
		"component": "agent-runtime",
	}, out.LokiLabels)
	return out
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func envInt64(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

func parseLabelEnv(raw string) map[string]string {
	labels := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = sanitizeLabelKey(key)
		value = sanitizeLabelValue(value)
		if key == "" || value == "" {
			continue
		}
		labels[key] = value
	}
	return labels
}

func mergeLabels(base map[string]string, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		k = sanitizeLabelKey(k)
		v = sanitizeLabelValue(v)
		if k != "" && v != "" {
			out[k] = v
		}
	}
	for k, v := range extra {
		k = sanitizeLabelKey(k)
		v = sanitizeLabelValue(v)
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}
