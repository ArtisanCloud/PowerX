package opsintegration

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLokiRetentionAndPromtailLabeling(t *testing.T) {
	root := repoRootFromHere(t)
	lokiPath := filepath.Join(root, "deploy", "observability", "loki", "loki-config.yaml")
	promtailPath := filepath.Join(root, "deploy", "observability", "promtail", "promtail-config.yaml")

	lokiRaw, err := os.ReadFile(lokiPath)
	require.NoError(t, err)
	lokiText := string(lokiRaw)
	require.Contains(t, lokiText, "retention_period: 720h", "loki retention should be 30 days")
	require.Contains(t, lokiText, "retention_enabled: true", "loki compactor retention must be enabled")

	promtailRaw, err := os.ReadFile(promtailPath)
	require.NoError(t, err)
	promtailText := string(promtailRaw)
	require.True(t, strings.Contains(promtailText, "job") || strings.Contains(promtailText, "app"), "promtail labels should include app/job dimensions")
}

func repoRootFromHere(t testing.TB) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime caller failed")
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "..")
}
