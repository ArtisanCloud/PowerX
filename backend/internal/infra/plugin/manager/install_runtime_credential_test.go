package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func newCredentialInstallManager(t *testing.T) (*managerImpl, string) {
	t.Helper()
	root := t.TempDir()
	registry := NewJSONRegistry(filepath.Join(root, "registry.json"))
	require.NoError(t, registry.Load(context.Background()))
	return &managerImpl{opts: Options{
		InstalledRoot: filepath.Join(root, "installed"), Registry: registry, Loader: NewFSLoader(),
		CoreConfig: &config.Config{Deployment: config.DeploymentConfig{Env: config.DeploymentEnvDev}},
	}, databaseSectionBuilder: testDatabaseSection}, root
}

func testInstallCredential() *PluginRuntimeCredential {
	return &PluginRuntimeCredential{
		TenantUUID: "00000000-0000-0000-0000-000000000001", ClientID: "test-client",
		ClientSecret: "test-secret", GRPCAddress: "127.0.0.1:9010", STSAudience: "powerx:api", STSScope: "access",
	}
}

func TestForceInstallRecoversCredentialBeforeRemovingOldVersion(t *testing.T) {
	m, root := newCredentialInstallManager(t)
	ctx := context.Background()
	src := writeReplaceTestPlugin(t, filepath.Join(root, "src"), "first")
	m.opts.RuntimeCredential = func(context.Context, string) (*PluginRuntimeCredential, error) { return testInstallCredential(), nil }
	p, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{})
	require.NoError(t, err)
	calls := 0
	m.opts.RuntimeCredential = func(context.Context, string) (*PluginRuntimeCredential, error) {
		calls++
		// 复现旧配置是唯一恢复源：删除前必须已成功读到它，后续不能重复读取。
		hc, err := loadHostConfig(p.Paths.HostValuesFile)
		require.NoError(t, err)
		require.Equal(t, "test-secret", hc.Values["POWERX_STS_CLIENT_SECRET"])
		return testInstallCredential(), nil
	}
	src2 := writeReplaceTestPlugin(t, filepath.Join(root, "src2"), "second")
	updated, err := m.InstallFromFile(ctx, src2, plugin_mgr.InstallOptions{Force: true})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	hc, err := loadHostConfig(updated.Paths.HostValuesFile)
	require.NoError(t, err)
	require.Equal(t, "test-client", hc.Values["POWERX_STS_CLIENT_ID"])
	require.Equal(t, "test-secret", hc.Values["POWERX_STS_CLIENT_SECRET"])
	info, err := os.Stat(updated.Paths.HostValuesFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestCredentialPreflightFailureLeavesForceTargetAndRegistryIntact(t *testing.T) {
	for _, registered := range []bool{true, false} {
		t.Run(map[bool]string{true: "registered", false: "orphan_directory"}[registered], func(t *testing.T) {
			m, root := newCredentialInstallManager(t)
			ctx := context.Background()
			src := writeReplaceTestPlugin(t, filepath.Join(root, "src"), "first")
			p, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{})
			require.NoError(t, err)
			if !registered {
				require.NoError(t, m.opts.Registry.Remove(ctx, p.ID, p.Version))
				require.NoError(t, m.opts.Registry.Save(ctx))
			}
			before, err := os.ReadFile(filepath.Join(root, "registry.json"))
			require.NoError(t, err)
			m.opts.RuntimeCredential = func(context.Context, string) (*PluginRuntimeCredential, error) {
				return nil, errors.New("POWERX_STS_CLIENT_ID missing")
			}
			_, err = m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{Force: true})
			require.ErrorContains(t, err, "runtime_credential_preflight")
			binary, err := os.ReadFile(filepath.Join(p.Paths.Root, "backend/bin/plugin"))
			require.NoError(t, err)
			require.Equal(t, "first", string(binary))
			after, err := os.ReadFile(filepath.Join(root, "registry.json"))
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestHostConfigCredentialErrorIsNotSwallowed(t *testing.T) {
	m, root := newCredentialInstallManager(t)
	m.opts.RuntimeCredential = func(context.Context, string) (*PluginRuntimeCredential, error) {
		return nil, errors.New("credential unavailable")
	}
	_, err := m.generateHostConfig(plugin_mgr.Manifest{ID: "com.powerx.plugins.test"}, root, nil)
	require.ErrorContains(t, err, "credential unavailable")
	_, statErr := os.Stat(filepath.Join(root, "config/host-values.yaml"))
	require.True(t, os.IsNotExist(statErr))
}

func TestEnableStopsWhenHostConfigCannotBePersisted(t *testing.T) {
	m, root := newCredentialInstallManager(t)
	ctx := context.Background()
	src := writeReplaceTestPlugin(t, filepath.Join(root, "src"), "first")
	p, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{})
	require.NoError(t, err)
	require.NoError(t, os.Remove(p.Paths.HostValuesFile))
	require.NoError(t, os.Mkdir(p.Paths.HostValuesFile, 0o700))
	m.opts.RuntimeCredential = func(context.Context, string) (*PluginRuntimeCredential, error) { return testInstallCredential(), nil }
	err = m.Enable(ctx, p.ID)
	require.ErrorContains(t, err, "enable.host_config")
	stored, ok := m.opts.Registry.Get(ctx, p.ID)
	require.True(t, ok)
	require.Equal(t, plugin_mgr.StateInstalled, stored.State)
}

func TestEnablePersistsRepairedCredentialToRegistryBeforeStarting(t *testing.T) {
	m, root := newCredentialInstallManager(t)
	ctx := context.Background()
	src := writeReplaceTestPlugin(t, filepath.Join(root, "src"), "first")
	p, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{})
	require.NoError(t, err)
	m.opts.RuntimeCredential = func(context.Context, string) (*PluginRuntimeCredential, error) { return testInstallCredential(), nil }
	// 此测试不启动进程；断点在路由尚未初始化，但运行配置必须已持久保存。
	err = m.Enable(ctx, p.ID)
	require.ErrorContains(t, err, "dynamic router not initialized")
	reloaded := NewJSONRegistry(filepath.Join(root, "registry.json"))
	require.NoError(t, reloaded.Load(ctx))
	stored, ok := reloaded.Get(ctx, p.ID)
	require.True(t, ok)
	require.Equal(t, plugin_mgr.StateInstalled, stored.State)
	require.Equal(t, "test-client", stored.HostConfig.Values["POWERX_STS_CLIENT_ID"])
	require.Equal(t, "test-secret", stored.HostConfig.Values["POWERX_STS_CLIENT_SECRET"])
}
