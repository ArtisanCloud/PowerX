package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func localUploadOptions(force bool) plugin_mgr.InstallOptions {
	return plugin_mgr.InstallOptions{LocalUpload: true, Force: force, Origin: plugin_mgr.InstallOrigin{Type: "local", Channel: "local"}}
}
func writeLegacyUploadPlugin(t *testing.T, root, version, code string) string {
	t.Helper()
	src := writeReplaceTestPluginVersion(t, root, version, code)
	path := filepath.Join(src, "plugin.yaml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	raw = []byte(strings.ReplaceAll(string(raw), "provider:\n  id: developer-test\n  authority: https://market.example/api/v1\n", ""))
	require.NoError(t, os.WriteFile(path, raw, 0o644))
	return src
}
func TestLocalUploadLegacyPackageReinstallsWithOriginalForceOptions(t *testing.T) {
	for _, version := range []string{"0.1.0", "0.2.0"} {
		t.Run(version, func(t *testing.T) {
			ctx := context.Background()
			m, root := newCredentialInstallManager(t)
			permissionSyncs := 0
			m.opts.PostInstallManifest = func(context.Context, plugin_mgr.Manifest) error { permissionSyncs++; return nil }
			m.opts.RuntimeCredential = func(context.Context, string) (*PluginRuntimeCredential, error) { return testInstallCredential(), nil }
			opts := localUploadOptions(true)
			opts.Metadata = plugin_mgr.InstallMetadata{Scope: "system", Notes: "old"}
			old, err := m.InstallFromFile(ctx, writeLegacyUploadPlugin(t, filepath.Join(root, "old"), "0.1.0", "old"), opts)
			require.NoError(t, err)
			opts.Metadata.Notes = "updated by upload"
			installed, err := m.InstallFromFile(ctx, writeLegacyUploadPlugin(t, filepath.Join(root, "new"), version, "new"), opts)
			require.NoError(t, err)
			require.False(t, installed.Provider.Known(), "legacy upload must not invent a publisher")
			require.NotEqual(t, old.Revision, installed.Revision)
			require.Equal(t, "updated by upload", installed.InstallMetadata.Notes)
			require.Equal(t, []string{version}, m.opts.Registry.ListVersions(ctx, old.ID))
			require.Equal(t, 2, permissionSyncs)
			bytes, err := os.ReadFile(filepath.Join(installed.Paths.Root, "backend/bin/plugin"))
			require.NoError(t, err)
			require.Equal(t, "new", string(bytes))
			cfg, err := loadHostConfig(installed.Paths.HostValuesFile)
			require.NoError(t, err)
			require.Equal(t, "test-client", cfg.Values["POWERX_STS_CLIENT_ID"])
			require.Equal(t, "test-secret", cfg.Values["POWERX_STS_CLIENT_SECRET"])
		})
	}
}
func TestLocalUploadWithoutForceRetainsSameVersionArtifact(t *testing.T) {
	ctx := context.Background()
	m, root := newCredentialInstallManager(t)
	syncs := 0
	m.opts.PostInstallManifest = func(context.Context, plugin_mgr.Manifest) error { syncs++; return nil }
	old, err := m.InstallFromFile(ctx, writeLegacyUploadPlugin(t, filepath.Join(root, "old"), "0.1.0", "old"), localUploadOptions(true))
	require.NoError(t, err)
	installed, err := m.InstallFromFile(ctx, writeLegacyUploadPlugin(t, filepath.Join(root, "new"), "0.1.0", "new"), localUploadOptions(false))
	require.NoError(t, err)
	require.Equal(t, old.Revision, installed.Revision)
	require.Equal(t, 2, syncs)
	raw, err := os.ReadFile(filepath.Join(old.Paths.Root, "backend/bin/plugin"))
	require.NoError(t, err)
	require.Equal(t, "old", string(raw))
}

func TestLocalUploadReinstallsHistoricalUnknownOriginWithoutNewIdentityFields(t *testing.T) {
	ctx := context.Background()
	m, root := newCredentialInstallManager(t)
	old, err := m.InstallFromFile(ctx, writeLegacyUploadPlugin(t, filepath.Join(root, "old"), "0.1.0", "old"), localUploadOptions(true))
	require.NoError(t, err)
	desc, err := NewFSLoader().LoadDescriptor(ctx, old.Paths.Root)
	require.NoError(t, err)
	desc.Revision = old.Revision
	desc.InstallOrigin = plugin_mgr.InstallOrigin{Type: "unknown"}
	require.NoError(t, m.opts.Registry.Put(ctx, desc, plugin_mgr.StateInstalled))
	require.NoError(t, m.opts.Registry.Save(ctx))
	installed, err := m.InstallFromFile(ctx, writeLegacyUploadPlugin(t, filepath.Join(root, "new"), "0.1.0", "new"), localUploadOptions(true))
	require.NoError(t, err)
	require.Equal(t, "local", installed.InstallOrigin.Type)
	require.False(t, installed.Provider.Known())
	require.Len(t, m.opts.Registry.ListVersions(ctx, old.ID), 1)
}
func TestLocalUploadPreservesKnownPublisherForLegacyPackage(t *testing.T) {
	ctx := context.Background()
	m, root := newCredentialInstallManager(t)
	old, err := m.InstallFromFile(ctx, writeReplaceTestPlugin(t, filepath.Join(root, "old"), "old"), localUploadOptions(true))
	require.NoError(t, err)
	installed, err := m.InstallFromFile(ctx, writeLegacyUploadPlugin(t, filepath.Join(root, "new"), "0.1.0", "new"), localUploadOptions(true))
	require.NoError(t, err)
	require.Equal(t, old.Provider, installed.Provider)
}
func TestLocalUploadCompatibilityDoesNotBypassKnownPublisherMismatchOrMarketConfirmation(t *testing.T) {
	for _, mode := range []string{"different_publisher", "market_origin", "stale_revision"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			m, root := newCredentialInstallManager(t)
			opts := localUploadOptions(true)
			if mode == "market_origin" {
				opts.LocalUpload = false
				opts.Origin = plugin_mgr.InstallOrigin{Type: "marketplace", SourceID: "https://market.example/api/v1"}
			}
			old, err := m.InstallFromFile(ctx, writeReplaceTestPlugin(t, filepath.Join(root, "old"), "old"), opts)
			require.NoError(t, err)
			target := writeReplaceTestPlugin(t, filepath.Join(root, "new"), "new")
			opts = localUploadOptions(true)
			if mode == "different_publisher" {
				path := filepath.Join(target, "plugin.yaml")
				raw, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(raw), "developer-test", "other-publisher")), 0o644))
			}
			if mode == "stale_revision" {
				opts.ExpectedRevision = "stale"
			}
			_, err = m.InstallFromFile(ctx, target, opts)
			require.Error(t, err)
			current, _ := m.opts.Registry.Get(ctx, old.ID)
			require.Equal(t, old.Revision, current.Revision)
			raw, err := os.ReadFile(filepath.Join(old.Paths.Root, "backend/bin/plugin"))
			require.NoError(t, err)
			require.Equal(t, "old", string(raw))
		})
	}
}
