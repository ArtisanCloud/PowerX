package config

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"testing"
)

func TestMarketplaceEnrollmentIsServerOnlyAndHeartbeatConfigurable(t *testing.T) {
	cfg := GetDefaults()
	values := map[string]string{"POWERX_MARKETPLACE_API_ROOT": "http://127.0.0.1:8088/api/v1", "POWERX_MARKETPLACE_ENROLLMENT_TOKEN": "pxm_enroll_private", "POWERX_MARKETPLACE_HEARTBEAT_SECONDS": "420"}
	require.NoError(t, loadFromEnvironment(cfg, func(k string) (string, bool) { v, ok := values[k]; return v, ok }))
	require.Equal(t, "pxm_enroll_private", cfg.Plugin.Marketplace.EnrollmentToken)
	require.Equal(t, 420, cfg.Plugin.Marketplace.HeartbeatSeconds)
	raw, err := yaml.Marshal(cfg.Plugin.Marketplace)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "pxm_enroll_private")
	raw, err = json.Marshal(cfg.Plugin.Marketplace)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "pxm_enroll_private")
	require.NoError(t, yaml.Unmarshal([]byte("enrollment_token: package-cannot-override\nheartbeat_seconds: 60\n"), &cfg.Plugin.Marketplace))
	require.Equal(t, "pxm_enroll_private", cfg.Plugin.Marketplace.EnrollmentToken)
	require.Equal(t, 60, cfg.Plugin.Marketplace.HeartbeatSeconds)
}
