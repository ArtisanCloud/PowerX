package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	"github.com/gin-gonic/gin"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func runtimeEventFixture(t *testing.T) (*managerImpl, *plugin_mgr.Plugin, *PluginRuntimeCredential) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "installed", "com.powerx.plugins.fixture", "1.0.0")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "config"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config", "service.yaml"), []byte("version: v1\n"), 0o600))
	doc := map[string]any{"runtime": map[string]any{"event_bridge": map[string]any{"topic_manifest": "config/service.yaml"}}, "env": map[string]any{"FIXTURE_SECRET": "retained"}, "operations": map[string]any{"manual": "retained"}}
	raw, err := yaml.Marshal(doc)
	require.NoError(t, err)
	path := filepath.Join(root, "config", "host-values.yaml")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	p := &plugin_mgr.Plugin{ID: "com.powerx.plugins.fixture", Version: "1.0.0", RequiredCapabilities: []string{"com.corex.eventfabric.service_publish"}, Paths: plugin_mgr.InstalledPaths{Root: root, HostValuesFile: path}, HostConfig: &plugin_mgr.HostConfig{ValuesFile: path, Spec: doc}}
	cred := &PluginRuntimeCredential{TenantUUID: "00000000-0000-0000-0000-000000000001"}
	reg := NewJSONRegistry(filepath.Join(base, "registry.json"))
	require.NoError(t, reg.Put(context.Background(), Descriptor{Manifest: plugin_mgr.Manifest{ID: p.ID, Version: p.Version}, Paths: p.Paths, HostConfig: p.HostConfig}, plugin_mgr.StateInstalled))
	require.NoError(t, reg.Save(context.Background()))
	m := &managerImpl{opts: Options{InstalledRoot: filepath.Join(base, "installed"), Registry: reg, PrepareEventFabric: func(_ context.Context, plugin plugin_mgr.Plugin, tenant, path string, provision bool, persist func(map[string]string) error) error {
		require.Equal(t, p.ID, plugin.ID)
		require.Equal(t, cred.TenantUUID, tenant)
		expected, err := filepath.EvalSymlinks(filepath.Join(root, "config", "service.yaml"))
		require.NoError(t, err)
		require.Equal(t, expected, path)
		if !provision {
			require.Nil(t, persist)
			return nil
		}
		return persist(map[string]string{"fixture.event.v1": tenant + ".fixture.event.v1"})
	}}}
	return m, p, cred
}

func TestEventFabricRuntimePersistsFormalReceiptsAndPreservesValues(t *testing.T) {
	m, p, cred := runtimeEventFixture(t)
	for i := 0; i < 2; i++ {
		require.NoError(t, m.prepareEventFabricForEnable(plugin_mgr.WithEventFabricProvisioning(context.Background()), p, cred))
	}
	raw, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	require.Equal(t, "retained", doc["env"].(map[string]any)["FIXTURE_SECRET"])
	require.Equal(t, "retained", doc["operations"].(map[string]any)["manual"])
	require.Equal(t, cred.TenantUUID+".fixture.event.v1", eventFabricBridge(doc)["topic_bindings"].(map[string]any)["fixture.event.v1"])
	stat, err := os.Stat(p.Paths.HostValuesFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), stat.Mode().Perm())
}

func TestEventFabricRuntimeRefusesInvalidOrFailedPreparationWithoutWriting(t *testing.T) {
	for _, name := range []string{"missing_preparer", "empty_manifest", "outside_package", "empty_receipts", "foreign_tenant", "duplicate_receipt", "upstream_failure"} {
		t.Run(name, func(t *testing.T) {
			m, p, cred := runtimeEventFixture(t)
			bridge := eventFabricBridge(p.HostConfig.Spec)
			switch name {
			case "missing_preparer":
				m.opts.PrepareEventFabric = nil
			case "empty_manifest":
				bridge["topic_manifest"] = ""
			case "outside_package":
				outside := filepath.Join(t.TempDir(), "outside.yaml")
				require.NoError(t, os.WriteFile(outside, []byte("fixture"), 0o600))
				require.NoError(t, os.Symlink(outside, filepath.Join(p.Paths.Root, "config", "outside.yaml")))
				bridge["topic_manifest"] = "config/outside.yaml"
			case "manual_conflict":
				bridge["topic_bindings"] = map[string]any{"fixture.event.v1": cred.TenantUUID + ".manual.topic"}
			default:
				m.opts.PrepareEventFabric = func(_ context.Context, _ plugin_mgr.Plugin, _, _ string, _ bool, persist func(map[string]string) error) error {
					switch name {
					case "empty_receipts":
						return persist(map[string]string{})
					case "foreign_tenant":
						return persist(map[string]string{"fixture.event.v1": "00000000-0000-0000-0000-000000000002.fixture.event.v1"})
					case "duplicate_receipt":
						return persist(map[string]string{"fixture.event.v1": cred.TenantUUID + ".fixture.event.v1", "fixture.other.v1": cred.TenantUUID + ".fixture.event.v1"})
					default:
						return errors.New("FIXTURE_PREPARATION_FAILED")
					}
				}
			}
			raw, err := yaml.Marshal(p.HostConfig.Spec)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(p.Paths.HostValuesFile, raw, 0o600))
			require.Error(t, m.prepareEventFabricForEnable(plugin_mgr.WithEventFabricProvisioning(context.Background()), p, cred))
			after, err := os.ReadFile(p.Paths.HostValuesFile)
			require.NoError(t, err)
			require.Equal(t, raw, after)
		})
	}
}

func TestEventFabricRuntimeLeavesUnrelatedPluginsUntouched(t *testing.T) {
	m, p, _ := runtimeEventFixture(t)
	p.RequiredCapabilities = []string{"com.corex.runtime.cache.service_read"}
	m.opts.PrepareEventFabric = func(_ context.Context, _ plugin_mgr.Plugin, _, _ string, _ bool, persist func(map[string]string) error) error {
		t.Fatal("UNRELATED_PLUGIN_PREPARATION")
		return nil
	}
	before, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	require.NoError(t, m.prepareEventFabricForEnable(context.Background(), p, nil))
	after, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestEventFabricRuntimeEnablePreparesBeforeProcessAndStopsOnFailure(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			m, root := newCredentialInstallManager(t)
			ctx := plugin_mgr.WithEventFabricProvisioning(context.Background())
			src := writeReplaceTestPlugin(t, filepath.Join(root, "src"), "not an executable")
			manifestPath := filepath.Join(src, "plugin.yaml")
			raw, err := os.ReadFile(manifestPath)
			require.NoError(t, err)
			raw = append(raw, []byte("\ncapabilities:\n  required: [com.corex.eventfabric.service_publish]\n")...)
			require.NoError(t, os.WriteFile(manifestPath, raw, 0600))
			require.NoError(t, os.MkdirAll(filepath.Join(src, "config"), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(src, "config/values.example.yaml"), []byte("runtime:\n  event_bridge:\n    topic_manifest: config/service.yaml\n"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(src, "config/service.yaml"), []byte("version: v1\n"), 0600))
			m.opts.RuntimeCredential = func(context.Context, string) (*PluginRuntimeCredential, error) { return testInstallCredential(), nil }
			p, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{})
			require.NoError(t, err)
			m.http = router.NewDynamicRouter("/_p", gin.New())
			prepared, rolledBack := false, false
			m.opts.PrepareRuntimeEnable = func(context.Context, string) (func(context.Context) error, error) {
				return func(context.Context) error { rolledBack = true; return nil }, nil
			}
			m.opts.PrepareEventFabric = func(_ context.Context, got plugin_mgr.Plugin, tenant, path string, provision bool, persist func(map[string]string) error) error {
				prepared = true
				require.Equal(t, p.ID, got.ID)
				require.FileExists(t, path)
				if failure {
					return errors.New("FIXTURE_EVENT_PREPARATION_FAILED")
				}
				if !provision {
					require.Nil(t, persist)
					return nil
				}
				return persist(map[string]string{"fixture.event.v1": tenant + ".fixture.event.v1"})
			}
			err = m.Enable(ctx, p.ID)
			require.True(t, prepared)
			require.True(t, rolledBack)
			if failure {
				require.ErrorContains(t, err, "enable.event_fabric")
			} else {
				// No supervisor exists: this boundary proves file and registry preparation
				// completed before the first attempt to start the plugin process.
				require.ErrorContains(t, err, "supervisor not initialized")
				reloaded := NewJSONRegistry(filepath.Join(root, "registry.json"))
				require.NoError(t, reloaded.Load(ctx))
				stored, ok := reloaded.Get(ctx, p.ID)
				require.True(t, ok)
				require.Equal(t, testInstallCredential().TenantUUID+".fixture.event.v1", eventFabricBridge(stored.HostConfig.Spec)["topic_bindings"].(map[string]any)["fixture.event.v1"])
			}
		})
	}
}

func TestEventFabricRuntimeAcceptsExplicitPreprovisionedBindings(t *testing.T) {
	m, p, cred := runtimeEventFixture(t)
	bridge := eventFabricBridge(p.HostConfig.Spec)
	delete(bridge, "topic_manifest")
	full := cred.TenantUUID + ".fixture.event.v1"
	bridge["topic_bindings"] = map[string]any{"fixture.event.v1": full}
	raw, err := yaml.Marshal(p.HostConfig.Spec)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p.Paths.HostValuesFile, raw, 0600))
	called := false
	m.opts.PrepareEventFabric = func(_ context.Context, _ plugin_mgr.Plugin, tenant, path string, provision bool, persist func(map[string]string) error) error {
		called = true
		require.Empty(t, path)
		if !provision {
			return nil
		}
		return persist(map[string]string{"fixture.event.v1": full})
	}
	require.NoError(t, m.prepareEventFabricForEnable(plugin_mgr.WithEventFabricProvisioning(context.Background()), p, cred))
	require.True(t, called)
}

func TestEventFabricRuntimeLeavesDirectCapabilityConsumersUntouched(t *testing.T) {
	m, p, _ := runtimeEventFixture(t)
	p.HostConfig.Spec = map[string]any{"runtime": map[string]any{"event_bridge": map[string]any{"mode": "host"}}}
	m.opts.PrepareEventFabric = func(_ context.Context, _ plugin_mgr.Plugin, _, _ string, _ bool, persist func(map[string]string) error) error {
		t.Fatal("UNDECLARED_PREPARATION")
		return nil
	}
	before, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	require.NoError(t, m.prepareEventFabricForEnable(context.Background(), p, nil))
	after, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestEventFabricRuntimeRecoveryOnlyVerifies(t *testing.T) {
	m, p, cred := runtimeEventFixture(t)
	before, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	old := cloneHostConfig(p.HostConfig)
	called := false
	m.opts.PrepareEventFabric = func(_ context.Context, _ plugin_mgr.Plugin, _, _ string, provision bool, persist func(map[string]string) error) error {
		called = true
		require.False(t, provision)
		require.Nil(t, persist)
		return errors.New("EVENT_FABRIC_INSTALL_PREPARATION_REQUIRED")
	}
	require.ErrorContains(t, m.prepareEventFabricForEnable(context.Background(), p, cred), "PREPARATION_REQUIRED")
	require.True(t, called)
	after, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	require.Equal(t, before, after)
	stored, ok := m.opts.Registry.Get(context.Background(), p.ID)
	require.True(t, ok)
	require.Equal(t, old, stored.HostConfig)
	require.NoFileExists(t, eventCheckpointPath(t, m, p))
}

type eventFabricFailSaveRegistry struct {
	Registry
	fail bool
}

func (r *eventFabricFailSaveRegistry) Save(ctx context.Context) error {
	if r.fail {
		r.fail = false
		return errors.New("REGISTRY_WRITE_FAILED")
	}
	return r.Registry.Save(ctx)
}
func TestEventFabricRuntimeRestoresFilesAndRegistryOnCommitFailure(t *testing.T) {
	for _, kind := range []string{"registry_save", "database_commit"} {
		t.Run(kind, func(t *testing.T) {
			m, p, cred := runtimeEventFixture(t)
			before, err := os.ReadFile(p.Paths.HostValuesFile)
			require.NoError(t, err)
			old := cloneHostConfig(p.HostConfig)
			if kind == "registry_save" {
				m.opts.Registry = &eventFabricFailSaveRegistry{Registry: m.opts.Registry, fail: true}
			} else {
				prepare := m.opts.PrepareEventFabric
				m.opts.PrepareEventFabric = func(ctx context.Context, p plugin_mgr.Plugin, tenant, path string, provision bool, persist func(map[string]string) error) error {
					if err := prepare(ctx, p, tenant, path, provision, persist); err != nil {
						return err
					}
					return errors.New("DATABASE_COMMIT_FAILED")
				}
			}
			require.Error(t, m.prepareEventFabricForEnable(plugin_mgr.WithEventFabricProvisioning(context.Background()), p, cred))
			after, err := os.ReadFile(p.Paths.HostValuesFile)
			require.NoError(t, err)
			require.Equal(t, before, after)
			stored, ok := m.opts.Registry.Get(context.Background(), p.ID)
			require.True(t, ok)
			require.Equal(t, old, stored.HostConfig)
			require.NoFileExists(t, eventCheckpointPath(t, m, p))
		})
	}
}
func TestEventFabricRuntimeInterruptedCommitNeedsExplicitRecovery(t *testing.T) {
	m, p, cred := runtimeEventFixture(t)
	before, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	checkpoint, err := json.Marshal(eventFabricCheckpoint{PluginID: p.ID, Version: p.Version, TenantUUID: cred.TenantUUID, Values: before, HostConfig: cloneHostConfig(p.HostConfig)})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(eventCheckpointPath(t, m, p)), 0700))
	require.NoError(t, os.WriteFile(eventCheckpointPath(t, m, p), checkpoint, 0600))
	require.NoError(t, os.WriteFile(p.Paths.HostValuesFile, []byte("uncommitted: true\n"), 0600))
	require.ErrorContains(t, m.prepareEventFabricForEnable(context.Background(), p, cred), "RECOVERY_REQUIRED")
	after, err := os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	require.Equal(t, "uncommitted: true\n", string(after))
	require.NoError(t, m.prepareEventFabricForEnable(plugin_mgr.WithEventFabricProvisioning(context.Background()), p, cred))
	require.NoFileExists(t, eventCheckpointPath(t, m, p))
	after, err = os.ReadFile(p.Paths.HostValuesFile)
	require.NoError(t, err)
	require.Contains(t, string(after), "retained")
}

func eventCheckpointPath(t *testing.T, m *managerImpl, p *plugin_mgr.Plugin) string {
	t.Helper()
	path, err := m.eventFabricCheckpointPath(p)
	require.NoError(t, err)
	return path
}

func TestEventFabricRuntimeDoesNotTrustPackageCheckpoint(t *testing.T) {
	m, p, cred := runtimeEventFixture(t)
	// 恶意/旧包放在版本目录内的同名文件不能成为 Core 的恢复来源。
	require.NoError(t, os.WriteFile(p.Paths.HostValuesFile+".event-fabric.pending", []byte(`{"values":"fake"}`), 0600))
	require.NoError(t, m.prepareEventFabricForEnable(plugin_mgr.WithEventFabricProvisioning(context.Background()), p, cred))
	actual, err := filepath.Rel(p.Paths.Root, eventCheckpointPath(t, m, p))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(actual, ".."+string(filepath.Separator)))
}
