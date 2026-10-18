package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSnapshotDirectoryConfigurationAndEnvironmentPrecedence(t *testing.T) {
	yamlDirectory := filepath.Join(t.TempDir(), "yaml snapshot store")
	envDirectory := filepath.Join(t.TempDir(), "environment store")
	for _, override := range []string{"", envDirectory} {
		cfg := GetDefaults()
		cfg.SystemSnapshots.Directory = yamlDirectory
		err := loadFromEnvironment(cfg, func(key string) (string, bool) {
			if key == "POWERX_SNAPSHOT_DIRECTORY" && override != "" {
				return override, true
			}
			return "", false
		})
		require.NoError(t, err)
		expected, source := yamlDirectory, "configuration"
		if override != "" {
			expected, source = envDirectory, "environment"
		}
		require.Equal(t, expected, cfg.SystemSnapshots.Directory)
		require.Equal(t, source, cfg.SystemSnapshots.DirectorySource())
		require.NoError(t, cfg.SystemSnapshots.Validate())
		raw, err := yaml.Marshal(cfg.SystemSnapshots)
		require.NoError(t, err)
		require.Contains(t, string(raw), "directory:")
		require.NotContains(t, string(raw), "directorySource")
		require.NotContains(t, string(raw), "directory_source")
	}
}
func TestSnapshotDirectoryValidation(t *testing.T) {
	for _, directory := range []string{"", filepath.Join(t.TempDir(), "store with spaces")} {
		require.NoError(t, (SnapshotConfig{Directory: directory}).Validate())
	}
	for _, directory := range []string{"./snapshots", "~/snapshots", "relative/path", "/invalid\x00path"} {
		require.ErrorContains(t, (SnapshotConfig{Directory: directory}).Validate(), "system_snapshots.directory")
	}
}
func TestRecoveryStartupRejectsSourceSnapshotDirectory(t *testing.T) {
	cfg := recoveryConfigFixture(t)
	cfg.SystemSnapshots.Directory = filepath.Join(cfg.Recovery.Root, "system-snapshots")
	require.NoError(t, cfg.ValidateRecoveryStartup())
	cfg.SystemSnapshots.Directory = filepath.Join(t.TempDir(), "production-store")
	require.ErrorContains(t, cfg.ValidateRecoveryStartup(), "候选目录以外")
}
