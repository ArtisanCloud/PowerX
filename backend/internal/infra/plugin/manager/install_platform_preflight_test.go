package manager

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	coreconfig "github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestInstallFromFileRejectsIncompatibleExecutableBeforeReplacing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	installedRoot := filepath.Join(root, "installed")
	registry := NewJSONRegistry(filepath.Join(root, "registry.json"))
	require.NoError(t, registry.Load(ctx))

	srcV1 := writeReplaceTestPlugin(t, filepath.Join(root, "src-v1"), "first")
	m := &managerImpl{
		opts: Options{
			InstalledRoot: installedRoot,
			Registry:      registry,
			Loader:        NewFSLoader(),
			CoreConfig: &coreconfig.Config{
				Deployment: coreconfig.DeploymentConfig{Env: coreconfig.DeploymentEnvDev},
			},
		},
		databaseSectionBuilder: testDatabaseSection,
	}

	installed, err := m.InstallFromFile(ctx, srcV1, plugin_mgr.InstallOptions{})
	require.NoError(t, err)
	require.Equal(t, "0.1.0", installed.Version)

	srcV2 := writeIncompatiblePlatformTestPlugin(t, filepath.Join(root, "src-v2"))
	_, err = m.InstallFromFile(ctx, srcV2, plugin_mgr.InstallOptions{Force: true})
	require.Error(t, err)
	require.True(t, plugin_mgr.IsCode(err, plugin_mgr.CodeInvalidArg), "err=%v", err)
	require.Contains(t, err.Error(), "incompatible executable")

	got, err := os.ReadFile(filepath.Join(installedRoot, "com.powerx.plugins.replace-test", "0.1.0", "backend", "bin", "plugin"))
	require.NoError(t, err)
	require.Equal(t, "first", string(got))
}

func TestDetectExecutableFormat(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	elf := filepath.Join(root, "migrate-linux-amd64")
	require.NoError(t, os.WriteFile(elf, testELF64AMD64(), 0o755))
	format, osName, arch, err := detectExecutableFormat(elf)
	require.NoError(t, err)
	require.Equal(t, "elf", format)
	require.Equal(t, "linux", osName)
	require.Equal(t, "amd64", arch)

	macho := filepath.Join(root, "migrate-darwin-arm64")
	require.NoError(t, os.WriteFile(macho, testMachO64ARM64(), 0o755))
	format, osName, arch, err = detectExecutableFormat(macho)
	require.NoError(t, err)
	require.Equal(t, "mach-o", format)
	require.Equal(t, "darwin", osName)
	require.Equal(t, "arm64", arch)
}

func TestExecutableCompatibilityCandidatesSkipHostCommands(t *testing.T) {
	t.Parallel()

	candidates := executableCompatibilityCandidates(plugin_mgr.Manifest{
		Runtime: plugin_mgr.RuntimeSpec{
			Kind:  plugin_mgr.RuntimeKindProcess,
			Entry: "backend/bin/plugin",
		},
		Frontend: plugin_mgr.FrontendSpec{
			Admin: plugin_mgr.FrontendAdminSpec{
				Kind: plugin_mgr.FrontendKindProcess,
				Process: &plugin_mgr.RuntimeSpec{
					Entry: "node",
					Args:  []string{"./web-admin/.output/server/index.mjs"},
				},
			},
		},
		Migrations: &plugin_mgr.MigrationsSpec{
			Entry: "./backend/bin/migrate",
		},
	})

	require.Equal(t, []string{"backend/bin/plugin", "./backend/bin/migrate"}, candidates)
}

func writeIncompatiblePlatformTestPlugin(t *testing.T, root string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "backend", "bin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "web-admin", ".output"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "backend", "bin", "plugin"), []byte("second"), 0o755))
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		require.NoError(t, os.WriteFile(filepath.Join(root, "backend", "bin", "migrate"), testMachO64ARM64(), 0o755))
	} else {
		require.NoError(t, os.WriteFile(filepath.Join(root, "backend", "bin", "migrate"), testELF64AMD64(), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), []byte(`
id: com.powerx.plugins.replace-test
name: Replace Test
version: 0.1.0
runtime:
  kind: process
  entry: backend/bin/plugin
migrations:
  entry: backend/bin/migrate
endpoints:
  http_base_path: /api/v1
frontend:
  admin:
    kind: static
    static_dir: web-admin/.output
routes:
  basePath: /api/v1
`), 0o644))
	return root
}

func testELF64AMD64() []byte {
	return []byte{
		0x7f, 'E', 'L', 'F',
		0x02, 0x01, 0x01, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x3e, 0x00,
	}
}

func testMachO64ARM64() []byte {
	return []byte{
		0xcf, 0xfa, 0xed, 0xfe,
		0x0c, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
}
