package manager

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func legacyLicenseFixture(t *testing.T) (*managerImpl, plugin_mgr.Plugin, string) {
	t.Helper()
	m, root := newCredentialInstallManager(t)
	p, err := m.InstallFromFile(context.Background(), writeReplaceTestPlugin(t, filepath.Join(root, "source"), "fixture"), plugin_mgr.InstallOptions{})
	require.NoError(t, err)
	raw, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	host := doc["host"].(map[string]any)
	prefix := host["redis"].(map[string]any)["key_prefix"].(string)
	setNestedValue(doc, []string{"marketplace", "license", "cache", "key_prefix"}, prefix+"license")
	setNestedValue(doc, []string{"marketplace", "license", "policy"}, "keep-policy")
	setNestedValue(doc, []string{"marketplace", "unrelated"}, "keep-business")
	raw, err = yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p.Paths.HostValuesFile, raw, 0600))
	p.HostConfig, err = loadHostConfig(p.Paths.HostValuesFile)
	require.NoError(t, err)
	require.NoError(t, m.opts.Registry.UpdateHostConfig(context.Background(), p.ID, p.Version, p.HostConfig))
	require.NoError(t, m.opts.Registry.Save(context.Background()))
	return m, p, filepath.Join(root, "registry.json")
}
func TestLegacyLicenseCachePreviewAndConfirmedCleanup(t *testing.T) {
	_, p, registry := legacyLicenseFixture(t)
	ctx := context.Background()
	before, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	report, err := PreviewLegacyLicenseCache(ctx, registry, p.ID, "")
	require.NoError(t, err)
	require.Equal(t, "legacy_default_candidate", report.Status)
	require.True(t, report.RequiresConfirmation)
	after, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	require.Equal(t, before, after, "preview must not write")
	backup := filepath.Join(filepath.Dir(registry), "cleanup-backup")
	report, err = RepairLegacyLicenseCache(ctx, registry, p.ID, p.Version, report.PlanDigest, backup, true, true)
	require.NoError(t, err)
	require.Equal(t, "absent", report.Status)
	host, err := loadHostConfig(p.Paths.HostValuesFile)
	require.NoError(t, err)
	marketplace := host.Spec["marketplace"].(map[string]any)
	license := marketplace["license"].(map[string]any)
	require.NotContains(t, license, "cache")
	require.Equal(t, "keep-policy", license["policy"])
	require.Equal(t, "keep-business", marketplace["unrelated"])
	require.NotContains(t, host.Spec["runtime"].(map[string]any), "cache")
	reg := NewJSONRegistry(registry)
	require.NoError(t, reg.Load(ctx))
	stored, ok := reg.Get(ctx, p.ID)
	require.True(t, ok)
	require.Equal(t, host.Spec, stored.HostConfig.Spec)
	for _, file := range []string{"registry.json", "host-values.yaml", "preview.json"} {
		stat, err := os.Stat(filepath.Join(backup, file))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), stat.Mode().Perm())
	}
	original, err := os.ReadFile(filepath.Join(backup, "host-values.yaml"))
	require.NoError(t, err)
	require.Equal(t, before, original)
}
func TestLegacyLicenseCacheRequiresReviewAndFreshPlan(t *testing.T) {
	for _, scenario := range []string{"no_confirmation", "core_running", "stale_file", "stale_registry", "backup_in_package"} {
		t.Run(scenario, func(t *testing.T) {
			_, p, registry := legacyLicenseFixture(t)
			ctx := context.Background()
			report, err := PreviewLegacyLicenseCache(ctx, registry, p.ID, p.Version)
			require.NoError(t, err)
			confirm, stopped := true, true
			backup := filepath.Join(filepath.Dir(registry), "backup")
			switch scenario {
			case "no_confirmation":
				confirm = false
			case "core_running":
				stopped = false
			case "stale_file":
				f, err := os.OpenFile(p.Paths.HostValuesFile, os.O_APPEND|os.O_WRONLY, 0)
				require.NoError(t, err)
				_, err = f.WriteString("\n# changed since preview\n")
				require.NoError(t, err)
				require.NoError(t, f.Close())
			case "stale_registry":
				f, err := os.OpenFile(registry, os.O_APPEND|os.O_WRONLY, 0)
				require.NoError(t, err)
				_, err = f.WriteString("\n")
				require.NoError(t, err)
				require.NoError(t, f.Close())
			case "backup_in_package":
				backup = filepath.Join(p.Paths.Root, "backup")
			}
			before, err := os.ReadFile(p.Paths.HostValuesFile)
			require.NoError(t, err)
			_, err = RepairLegacyLicenseCache(ctx, registry, p.ID, p.Version, report.PlanDigest, backup, confirm, stopped)
			require.Error(t, err)
			after, err := os.ReadFile(p.Paths.HostValuesFile)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.NoDirExists(t, backup)
		})
	}
}
func TestLegacyLicenseCacheManualConfigurationIsPreserved(t *testing.T) {
	for _, scenario := range []string{"provider", "redis_url", "unknown_prefix", "empty_block", "drift"} {
		t.Run(scenario, func(t *testing.T) {
			m, p, registry := legacyLicenseFixture(t)
			ctx := context.Background()
			raw, err := os.ReadFile(p.Paths.HostValuesFile)
			require.NoError(t, err)
			var doc map[string]any
			require.NoError(t, yaml.Unmarshal(raw, &doc))
			cache := doc["marketplace"].(map[string]any)["license"].(map[string]any)["cache"].(map[string]any)
			switch scenario {
			case "provider":
				cache["provider"] = "redis"
			case "redis_url":
				cache["redis_url"] = "redis://:do-not-print@fixture.invalid:6379/0"
			case "unknown_prefix":
				cache["key_prefix"] = "user-prefix"
			case "empty_block":
				delete(cache, "key_prefix")
			case "drift":
				setNestedValue(doc, []string{"manual"}, "changed")
			}
			raw, err = yaml.Marshal(doc)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(p.Paths.HostValuesFile, raw, 0600))
			if scenario != "drift" {
				p.HostConfig, err = loadHostConfig(p.Paths.HostValuesFile)
				require.NoError(t, err)
				require.NoError(t, m.opts.Registry.UpdateHostConfig(ctx, p.ID, p.Version, p.HostConfig))
				require.NoError(t, m.opts.Registry.Save(ctx))
			}
			report, err := PreviewLegacyLicenseCache(ctx, registry, p.ID, p.Version)
			require.NoError(t, err)
			if scenario == "drift" {
				require.Equal(t, "configuration_drift", report.Status)
			} else {
				require.Equal(t, "manual_migration_required", report.Status)
			}
			safe, err := json.Marshal(report)
			require.NoError(t, err)
			require.NotContains(t, string(safe), "do-not-print")
			_, err = RepairLegacyLicenseCache(ctx, registry, p.ID, p.Version, report.PlanDigest, filepath.Join(filepath.Dir(registry), "backup"), true, true)
			require.Error(t, err)
			after, err := os.ReadFile(p.Paths.HostValuesFile)
			require.NoError(t, err)
			require.Equal(t, raw, after)
		})
	}
}

type hostConfigFailureRegistry struct {
	Registry
	stage  string
	failed bool
}

func (r *hostConfigFailureRegistry) UpdateHostConfig(ctx context.Context, id, version string, h *plugin_mgr.HostConfig) error {
	if r.stage == "update" && !r.failed {
		r.failed = true
		return errors.New("TEST_REGISTRY_UPDATE_FAILED")
	}
	return r.Registry.UpdateHostConfig(ctx, id, version, h)
}
func (r *hostConfigFailureRegistry) Save(ctx context.Context) error {
	if r.stage == "save" && !r.failed {
		r.failed = true
		return errors.New("TEST_REGISTRY_SAVE_FAILED")
	}
	return r.Registry.Save(ctx)
}
func TestHostConfigUpdateFailureRestoresFileMemoryAndRegistry(t *testing.T) {
	for _, stage := range []string{"write", "update", "save"} {
		t.Run(stage, func(t *testing.T) {
			m, p, registry := legacyLicenseFixture(t)
			ctx := context.Background()
			original := cloneHostConfig(p.HostConfig)
			before, err := os.ReadFile(p.Paths.HostValuesFile)
			require.NoError(t, err)
			registryBefore, err := os.ReadFile(registry)
			require.NoError(t, err)
			if stage == "write" {
				require.NoError(t, os.Remove(p.Paths.HostValuesFile))
				require.NoError(t, os.Mkdir(p.Paths.HostValuesFile, 0700))
			} else {
				m.opts.Registry = &hostConfigFailureRegistry{Registry: m.opts.Registry, stage: stage}
			}
			m.opts.CoreConfig.EnvironmentID = "changed-environment"
			require.Error(t, m.persistDelegatedHostContractForEnable(ctx, &p, testInstallCredential()))
			require.Equal(t, original, p.HostConfig)
			if stage != "write" {
				after, err := os.ReadFile(p.Paths.HostValuesFile)
				require.NoError(t, err)
				require.Equal(t, before, after)
			}
			registryAfter, err := os.ReadFile(registry)
			require.NoError(t, err)
			require.Equal(t, registryBefore, registryAfter)
			stored, ok := m.opts.Registry.Get(ctx, p.ID)
			require.True(t, ok)
			require.Equal(t, original, stored.HostConfig)
		})
	}
}

func TestHostConfigEnableRejectsMalformedFileWithoutOverwriting(t *testing.T) {
	for _, raw := range []string{"marketplace: [broken\n", "   \n"} {
		t.Run(raw, func(t *testing.T) {
			m, p, registry := legacyLicenseFixture(t)
			before := cloneHostConfig(p.HostConfig)
			require.NoError(t, os.WriteFile(p.Paths.HostValuesFile, []byte(raw), 0600))
			regBefore, err := os.ReadFile(registry)
			require.NoError(t, err)
			require.ErrorContains(t, m.persistDelegatedHostContractForEnable(context.Background(), &p, testInstallCredential()), "HOST_VALUES_INVALID")
			after, err := os.ReadFile(p.Paths.HostValuesFile)
			require.NoError(t, err)
			require.Equal(t, raw, string(after))
			require.Equal(t, before, p.HostConfig)
			regAfter, err := os.ReadFile(registry)
			require.NoError(t, err)
			require.Equal(t, regBefore, regAfter)
		})
	}
}

func TestHostConfigEnableDoesNotReintroduceRegistryOnlyLegacyCache(t *testing.T) {
	m, p, _ := legacyLicenseFixture(t)
	raw, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	removeLegacyLicenseCache(doc)
	raw, err = yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p.Paths.HostValuesFile, raw, 0600))
	require.NoError(t, m.persistDelegatedHostContractForEnable(context.Background(), &p, testInstallCredential()))
	status, _ := legacyLicenseCacheStatus(p.HostConfig.Spec, p.ID)
	require.Equal(t, "absent", status)
	loaded, err := loadHostConfig(p.Paths.HostValuesFile)
	require.NoError(t, err)
	status, _ = legacyLicenseCacheStatus(loaded.Spec, p.ID)
	require.Equal(t, "absent", status)
	stored, ok := m.opts.Registry.Get(context.Background(), p.ID)
	require.True(t, ok)
	status, _ = legacyLicenseCacheStatus(stored.HostConfig.Spec, p.ID)
	require.Equal(t, "absent", status)
}

func TestEnableRegistryFailureRestoresHostConfiguration(t *testing.T) {
	m, p, registry := legacyLicenseFixture(t)
	ctx := context.Background()
	before := cloneHostConfig(p.HostConfig)
	fileBefore, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	regBefore, err := os.ReadFile(registry)
	require.NoError(t, err)
	m.opts.RuntimeCredential = func(context.Context, string) (*PluginRuntimeCredential, error) { return testInstallCredential(), nil }
	m.opts.Registry = &hostConfigFailureRegistry{Registry: m.opts.Registry, stage: "save"}
	err = m.Enable(ctx, p.ID)
	require.ErrorContains(t, err, "enable.host_config")
	stored, ok := m.opts.Registry.Get(ctx, p.ID)
	require.True(t, ok)
	require.Equal(t, before, stored.HostConfig)
	require.Equal(t, plugin_mgr.StateInstalled, stored.State)
	fileAfter, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	require.Equal(t, fileBefore, fileAfter)
	regAfter, err := os.ReadFile(registry)
	require.NoError(t, err)
	require.Equal(t, regBefore, regAfter)
}

func TestHostConfigAtomicWriteFailurePreservesOriginal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("filesystem permission failure requires unprivileged user")
	}
	m, p, registry := legacyLicenseFixture(t)
	before := cloneHostConfig(p.HostConfig)
	fileBefore, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	regBefore, err := os.ReadFile(registry)
	require.NoError(t, err)
	dir := filepath.Dir(p.Paths.HostValuesFile)
	require.NoError(t, os.Chmod(dir, 0500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0700)) })
	require.Error(t, m.persistDelegatedHostContractForEnable(context.Background(), &p, testInstallCredential()))
	require.Equal(t, before, p.HostConfig)
	fileAfter, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	require.Equal(t, fileBefore, fileAfter)
	regAfter, err := os.ReadFile(registry)
	require.NoError(t, err)
	require.Equal(t, regBefore, regAfter)
}
