package system_recovery

import (
	"context"
	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginProgramsIncludeDeclaredCatalogsAndSkills(t *testing.T) {
	cfg := config.GetDefaults()
	cfg.Plugin.InstalledDir = t.TempDir()
	cfg.Plugin.RegistryFile = ""
	for _, name := range []string{"plugin.d/capabilities.yaml", "skills/template/SKILL.md"} {
		file := filepath.Join(cfg.Plugin.InstalledDir, "fixture", "1.0.0", name)
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
		require.NoError(t, os.WriteFile(file, []byte("fixture"), 0600))
	}
	credential := filepath.Join(cfg.Plugin.InstalledDir, "fixture", ".runtime-credentials.json")
	require.NoError(t, os.WriteFile(credential, []byte(`{"client_id":"test-client","client_secret":"private-fixture","tenant_uuid":"fixture"}`), 0600))
	source := snapshots.Config{}
	require.NoError(t, refreshPluginSources(context.Background(), &source, cfg))
	require.Contains(t, source.Artifacts, "plugins/fixture/1.0.0/plugin.d")
	require.Contains(t, source.Artifacts, "plugins/fixture/1.0.0/skills")
	require.Equal(t, credential, source.ConfigFiles["plugins/fixture/.runtime-credentials.json"])
	require.NotContains(t, source.Artifacts, "plugins/fixture/.runtime-credentials.json")
}
