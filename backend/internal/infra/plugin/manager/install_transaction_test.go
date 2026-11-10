package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func replacementOptions(p plugin_mgr.Plugin) plugin_mgr.InstallOptions {
	return plugin_mgr.InstallOptions{Force: true, ExpectedRevision: p.Revision, Origin: plugin_mgr.InstallOrigin{Type: "marketplace", SourceID: "https://market.example/api/v1", SHA256: strings.Repeat("a", 64)}}
}

func TestSingleInstallReplacesSourceAndVersionPreservingConfig(t *testing.T) {
	ctx := context.Background()
	m, root := newCredentialInstallManager(t)
	src := writeReplaceTestPlugin(t, filepath.Join(root, "first"), "first-code")
	old, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{HostConfigSeed: &plugin_mgr.HostConfig{Values: map[string]string{"BUSINESS_SETTING": "preserve-me"}, Spec: map[string]any{"business": map[string]any{"setting": "keep-me"}}}})
	require.NoError(t, err)
	target := writeReplaceTestPluginVersion(t, filepath.Join(root, "target"), "0.2.0", "new-code")
	_, err = m.InstallFromFile(ctx, target, plugin_mgr.InstallOptions{})
	var preview *plugin_mgr.ReplacementRequiredError
	require.ErrorAs(t, err, &preview)
	require.Equal(t, old.Revision, preview.Current.Revision)
	installed, err := m.InstallFromFile(ctx, target, replacementOptions(old))
	require.NoError(t, err)
	require.Equal(t, []string{"0.2.0"}, m.opts.Registry.ListVersions(ctx, installed.ID))
	require.NotEqual(t, old.Revision, installed.Revision)
	require.Equal(t, "marketplace", installed.InstallOrigin.Type)
	require.Equal(t, "preserve-me", installed.HostConfig.Values["BUSINESS_SETTING"])
	require.Equal(t, "keep-me", installed.HostConfig.Spec["business"].(map[string]any)["setting"])
	_, err = os.Stat(old.Paths.Root)
	require.True(t, os.IsNotExist(err))
	backups, err := filepath.Glob(filepath.Join(root, ".powerx-install-backups", "replace-*", "previous", "backend", "bin", "plugin"))
	require.NoError(t, err)
	require.Len(t, backups, 1)
	data, err := os.ReadFile(backups[0])
	require.NoError(t, err)
	require.Equal(t, "first-code", string(data))
	require.NoError(t, m.Bootstrap(ctx))
	require.Equal(t, []string{"0.2.0"}, m.opts.Registry.ListVersions(ctx, installed.ID))
}

func TestSingleInstallFailureRestoresOriginalRegistrationAndArtifact(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"hook", "migration", "enable"} {
		t.Run(mode, func(t *testing.T) {
			m, root := newCredentialInstallManager(t)
			src := writeReplaceTestPlugin(t, filepath.Join(root, "old"), "original")
			old, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{})
			require.NoError(t, err)
			target := writeReplaceTestPluginVersion(t, filepath.Join(root, "new"), "0.2.0", "target")
			opts := replacementOptions(old)
			switch mode {
			case "hook":
				m.opts.PostInstallManifest = func(context.Context, plugin_mgr.Manifest) error {
					return errors.New("controlled permission sync failure")
				}
			case "migration":
				require.NoError(t, os.WriteFile(filepath.Join(target, "fail.sh"), []byte("#!/bin/sh\nexit 7\n"), 0o755))
				f, err := os.OpenFile(filepath.Join(target, "plugin.yaml"), os.O_APPEND|os.O_WRONLY, 0)
				require.NoError(t, err)
				_, err = f.WriteString("\nmigrations:\n  entry: fail.sh\n")
				require.NoError(t, err)
				require.NoError(t, f.Close())
				opts.RunMigrations = true
			case "enable":
				opts.AutoEnable = true
			}
			_, err = m.InstallFromFile(ctx, target, opts)
			require.Error(t, err)
			current, ok := m.opts.Registry.Get(ctx, old.ID)
			require.True(t, ok)
			require.Equal(t, old.Revision, current.Revision)
			require.Equal(t, old.InstallOrigin, current.InstallOrigin)
			require.Equal(t, []string{old.Version}, m.opts.Registry.ListVersions(ctx, old.ID))
			data, err := os.ReadFile(filepath.Join(old.Paths.Root, "backend", "bin", "plugin"))
			require.NoError(t, err)
			require.Equal(t, "original", string(data))
			_, err = os.Stat(filepath.Join(m.opts.InstalledRoot, old.ID, "0.2.0"))
			require.True(t, os.IsNotExist(err))
		})
	}
}

func TestSingleInstallIdentityAndRevisionChecksDoNotMutate(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"other_provider", "missing_provider", "stale_revision", "no_confirmation"} {
		t.Run(mode, func(t *testing.T) {
			m, root := newCredentialInstallManager(t)
			src := writeReplaceTestPlugin(t, filepath.Join(root, "old"), "original")
			old, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{})
			require.NoError(t, err)
			target := writeReplaceTestPluginVersion(t, filepath.Join(root, "new"), "0.2.0", "target")
			opts := replacementOptions(old)
			if mode == "stale_revision" {
				opts.ExpectedRevision = "stale"
			}
			if mode == "no_confirmation" {
				opts.Force = false
			}
			if mode == "other_provider" || mode == "missing_provider" {
				raw, err := os.ReadFile(filepath.Join(target, "plugin.yaml"))
				require.NoError(t, err)
				if mode == "other_provider" {
					raw = []byte(strings.ReplaceAll(string(raw), "developer-test", "another-developer"))
				} else {
					raw = []byte(strings.ReplaceAll(string(raw), "provider:\n  id: developer-test\n  authority: https://market.example/api/v1\n", ""))
				}
				require.NoError(t, os.WriteFile(filepath.Join(target, "plugin.yaml"), raw, 0o644))
			}
			_, err = m.InstallFromFile(ctx, target, opts)
			require.Error(t, err)
			current, ok := m.opts.Registry.Get(ctx, old.ID)
			require.True(t, ok)
			require.Equal(t, old.Revision, current.Revision)
			require.Equal(t, []string{old.Version}, m.opts.Registry.ListVersions(ctx, old.ID))
		})
	}
}

func TestSingleInstallSameVerifiedArtifactIsIdempotent(t *testing.T) {
	ctx := context.Background()
	m, root := newCredentialInstallManager(t)
	src := writeReplaceTestPlugin(t, filepath.Join(root, "src"), "code")
	sum := strings.Repeat("a", 64)
	old, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{Origin: plugin_mgr.InstallOrigin{Type: "local", SHA256: sum}})
	require.NoError(t, err)
	current, err := m.InstallFromFile(ctx, src, replacementOptions(old))
	require.NoError(t, err)
	require.Equal(t, old.Revision, current.Revision)
	require.Equal(t, "local", current.InstallOrigin.Type)
}

func TestConcurrentInstallCannotRegisterParallelVersions(t *testing.T) {
	ctx := context.Background()
	m, root := newCredentialInstallManager(t)
	sources := []string{writeReplaceTestPlugin(t, filepath.Join(root, "one"), "one"), writeReplaceTestPluginVersion(t, filepath.Join(root, "two"), "0.2.0", "two")}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, src := range sources {
		wg.Add(1)
		go func(src string) {
			defer wg.Done()
			<-start
			_, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{})
			results <- err
		}(src)
	}
	close(start)
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			require.True(t, plugin_mgr.IsCode(err, plugin_mgr.CodeConflict))
		}
	}
	require.Equal(t, 1, succeeded)
	require.Len(t, m.opts.Registry.ListVersions(ctx, "com.powerx.plugins.replace-test"), 1)
}

func TestBootstrapRecoversInterruptedReplacementBeforeDiscovery(t *testing.T) {
	ctx := context.Background()
	m, root := newCredentialInstallManager(t)
	src := writeReplaceTestPlugin(t, filepath.Join(root, "src"), "original")
	old, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{})
	require.NoError(t, err)
	registry := m.opts.Registry.(*JSONRegistry)
	snapshot, err := registry.installSnapshot(old.ID)
	require.NoError(t, err)
	work := filepath.Join(root, ".powerx-install-backups", "replace-interrupted")
	require.NoError(t, os.MkdirAll(work, 0o700))
	backup, target := filepath.Join(work, "previous"), filepath.Join(m.opts.InstalledRoot, old.ID, "0.2.0")
	journal := installRecovery{PluginID: old.ID, InstalledRoot: m.opts.InstalledRoot, PreviousRoot: old.Paths.Root, BackupRoot: backup, TargetRoot: target, Revision: "unfinished-target", Snapshot: snapshot}
	require.NoError(t, journal.write(work))
	require.NoError(t, os.Rename(old.Paths.Root, backup))
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "partial"), []byte("unfinished"), 0o600))
	require.NoError(t, m.Bootstrap(ctx))
	current, ok := registry.Get(ctx, old.ID)
	require.True(t, ok)
	require.Equal(t, old.Revision, current.Revision)
	require.Equal(t, []string{old.Version}, registry.ListVersions(ctx, old.ID))
	_, err = os.Stat(target)
	require.True(t, os.IsNotExist(err))
}

func TestFirstInstallFailureLeavesNoDiscoverableArtifact(t *testing.T) {
	ctx := context.Background()
	m, root := newCredentialInstallManager(t)
	m.opts.PostInstallManifest = func(context.Context, plugin_mgr.Manifest) error { return errors.New("controlled failure") }
	src := writeReplaceTestPlugin(t, filepath.Join(root, "src"), "first")
	_, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{})
	require.Error(t, err)
	require.Empty(t, m.opts.Registry.List(ctx))
	descriptors, err := NewFSLoader().Discover(ctx, m.opts.InstalledRoot)
	require.NoError(t, err)
	require.Empty(t, descriptors)
}

func TestLegacyPublisherRequiresExplicitBindingBeforeReplacement(t *testing.T) {
	ctx := context.Background()
	m, root := newCredentialInstallManager(t)
	src := writeReplaceTestPlugin(t, filepath.Join(root, "old"), "original")
	path := filepath.Join(src, "plugin.yaml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(raw), "provider:\n  id: developer-test\n  authority: https://market.example/api/v1\n", "")), 0o644))
	old, err := m.InstallFromFile(ctx, src, plugin_mgr.InstallOptions{})
	require.NoError(t, err)
	require.False(t, old.Provider.Known())
	target := writeReplaceTestPluginVersion(t, filepath.Join(root, "new"), "0.2.0", "target")
	opts := replacementOptions(old)
	_, err = m.InstallFromFile(ctx, target, opts)
	var preview *plugin_mgr.ReplacementRequiredError
	require.ErrorAs(t, err, &preview)
	require.True(t, preview.RequiresProviderConfirmation)
	current, _ := m.opts.Registry.Get(ctx, old.ID)
	require.Equal(t, old.Revision, current.Revision)
	opts.ConfirmLegacyProvider = true
	installed, err := m.InstallFromFile(ctx, target, opts)
	require.NoError(t, err)
	require.Equal(t, "developer-test", installed.Provider.ID)
	require.Equal(t, []string{"0.2.0"}, m.opts.Registry.ListVersions(ctx, old.ID))
}

func TestRecoveryDoesNotUndoCommittedReplacement(t *testing.T) {
	ctx := context.Background()
	m, root := newCredentialInstallManager(t)
	old, err := m.InstallFromFile(ctx, writeReplaceTestPlugin(t, filepath.Join(root, "old"), "old"), plugin_mgr.InstallOptions{})
	require.NoError(t, err)
	registry := m.opts.Registry.(*JSONRegistry)
	snapshot, err := registry.installSnapshot(old.ID)
	require.NoError(t, err)
	installed, err := m.InstallFromFile(ctx, writeReplaceTestPluginVersion(t, filepath.Join(root, "new"), "0.2.0", "new"), replacementOptions(old))
	require.NoError(t, err)
	works, err := filepath.Glob(filepath.Join(root, ".powerx-install-backups", "replace-*"))
	require.NoError(t, err)
	require.Len(t, works, 1)
	journal := installRecovery{PluginID: old.ID, InstalledRoot: m.opts.InstalledRoot, PreviousRoot: old.Paths.Root, BackupRoot: filepath.Join(works[0], "previous"), TargetRoot: installed.Paths.Root, Revision: installed.Revision, Snapshot: snapshot}
	require.NoError(t, journal.write(works[0]))
	require.NoError(t, m.Bootstrap(ctx))
	current, _ := registry.Get(ctx, old.ID)
	require.Equal(t, installed.Revision, current.Revision)
	require.Equal(t, []string{"0.2.0"}, registry.ListVersions(ctx, old.ID))
}
