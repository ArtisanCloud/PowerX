package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestHostRedisNamespaceSurvivesFreshInstallUpgradeAndEnable(t *testing.T) {
	m, _ := newCredentialInstallManager(t)
	m.opts.CoreConfig = config.GetDefaults()
	m.opts.CoreConfig.Deployment.Env = "dev"
	m.opts.CoreConfig.EnvironmentID = "instance-a"
	m.opts.RuntimeCredential = func(context.Context, string) (*PluginRuntimeCredential, error) { return testInstallCredential(), nil }
	manifest := plugin_mgr.Manifest{ID: "com.powerx.plugins.ai-craft"}
	generate := func(root string, seed *plugin_mgr.HostConfig) *plugin_mgr.HostConfig {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "config"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "config/values.example.yaml"), []byte("event_bridge:\n  redis_stream: package-default\n  source_plugin: ai-craft\nruntime:\n  event_bridge:\n    redis_stream: nested-package-default\n"), 0o600))
		host, err := m.generateHostConfig(manifest, root, seed)
		require.NoError(t, err)
		state, _ := legacyLicenseCacheStatus(host.Spec, manifest.ID)
		require.Equal(t, "absent", state, "fresh/upgrade host config must not manufacture License cache")
		runtime, _ := host.Spec["runtime"].(map[string]any)
		require.NotContains(t, runtime, "cache")
		require.NotEmpty(t, host.Values["POWERX_ENVIRONMENT_ID"])
		require.NotEmpty(t, host.Values["POWERX_REDIS_KEY_PREFIX"])
		return host
	}
	a := generate(t.TempDir(), nil)
	b := generate(t.TempDir(), a)
	c := generate(t.TempDir(), nil) // reinstall without any previous-version seed
	stream := a.Spec["event_bridge"].(map[string]any)["redis_stream"]
	require.NotEqual(t, "package-default", stream)
	require.Equal(t, stream, b.Spec["event_bridge"].(map[string]any)["redis_stream"])
	require.Equal(t, stream, c.Spec["event_bridge"].(map[string]any)["redis_stream"])
	require.Equal(t, stream, a.Spec["runtime"].(map[string]any)["event_bridge"].(map[string]any)["redis_stream"])
	plugin := &plugin_mgr.Plugin{ID: manifest.ID, HostConfig: a}
	plugin.Paths.HostValuesFile = a.ValuesFile
	require.NoError(t, m.ensureDelegatedHostContractForEnable(plugin, testInstallCredential()))
	loaded, err := loadHostConfig(a.ValuesFile)
	require.NoError(t, err)
	state, _ := legacyLicenseCacheStatus(loaded.Spec, manifest.ID)
	require.Equal(t, "absent", state)
	expected, err := json.Marshal(plugin.HostConfig.Spec)
	require.NoError(t, err)
	actual, err := json.Marshal(loaded.Spec)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(actual))
	require.Equal(t, stream, loaded.Spec["event_bridge"].(map[string]any)["redis_stream"])
	m.opts.CoreConfig.EnvironmentID = "instance-b"
	d := generate(t.TempDir(), a) // an old seed must not override another installation
	require.NotEqual(t, stream, d.Spec["event_bridge"].(map[string]any)["redis_stream"])
	manifest.ID = "com.powerx.plugins.other"
	e := generate(t.TempDir(), nil)
	require.NotEqual(t, d.Spec["event_bridge"].(map[string]any)["redis_stream"], e.Spec["event_bridge"].(map[string]any)["redis_stream"])
}

func TestHostRedisNamespacePreservesManualBusinessCache(t *testing.T) {
	m, _ := newCredentialInstallManager(t)
	env := map[string]string{}
	doc := map[string]any{"marketplace": map[string]any{"license": map[string]any{"cache": map[string]any{"provider": "redis", "redis_url": "redis://manual.invalid/0", "key_prefix": "manual-prefix"}}, "policy": "keep"}, "runtime": map[string]any{"cache": map[string]any{"provider": "redis", "redis_url": "redis://manual-local.invalid/0"}}}
	before := cloneAnyMap(doc["marketplace"].(map[string]any))
	runtimeBefore := cloneAnyMap(doc["runtime"].(map[string]any))
	require.NoError(t, m.applyHostRedisNamespace(env, doc, "com.powerx.plugins.fixture"))
	require.Equal(t, before, doc["marketplace"])
	require.Equal(t, runtimeBefore["cache"], doc["runtime"].(map[string]any)["cache"])
	require.NotEmpty(t, env["POWERX_REDIS_KEY_PREFIX"])
}
