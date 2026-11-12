package manager

import (
	"context"
	"errors"
	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"os"
	"path/filepath"
	"testing"

	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTaskInstallationRevisionAndCheckpointFencing(t *testing.T) {
	m, root := newCredentialInstallManager(t)
	source := writeReplaceTestPlugin(t, filepath.Join(root, "source"), "binary")
	taskID := uuid.NewString()
	var phases []string
	p, e := m.InstallFromFile(context.Background(), source, pm.InstallOptions{InstallationRevision: taskID, Progress: func(ctx context.Context, phase string, p pm.Plugin) error {
		phases = append(phases, phase)
		require.Equal(t, taskID, p.Revision)
		return nil
	}})
	require.NoError(t, e)
	require.Equal(t, taskID, p.Revision)
	require.Equal(t, []string{"copying", "migrating", "committing"}, phases)
}
func TestTaskCheckpointFailureNeverCommitsAndPreservesReplacement(t *testing.T) {
	for _, phase := range []string{"copying", "migrating", "committing"} {
		t.Run(phase, func(t *testing.T) {
			m, root := newCredentialInstallManager(t)
			oldSource := writeReplaceTestPlugin(t, filepath.Join(root, "old"), "original")
			old, e := m.InstallFromFile(context.Background(), oldSource, pm.InstallOptions{})
			require.NoError(t, e)
			source := writeReplaceTestPluginVersion(t, filepath.Join(root, "new"), "0.2.0", "replacement")
			o := replacementOptions(old)
			o.InstallationRevision = uuid.NewString()
			o.Progress = func(ctx context.Context, stage string, p pm.Plugin) error {
				if stage == phase {
					return errors.New("lease lost")
				}
				return nil
			}
			_, e = m.InstallFromFile(context.Background(), source, o)
			require.Error(t, e)
			after, e := m.Get(context.Background(), old.ID)
			require.NoError(t, e)
			require.Equal(t, old.Revision, after.Revision)
			binary, e := os.ReadFile(filepath.Join(after.Paths.Root, "backend/bin/plugin"))
			require.NoError(t, e)
			require.Equal(t, "original", string(binary))
		})
	}
}
func TestTaskLocalIdentityMismatchBeforeMutation(t *testing.T) {
	m, root := newCredentialInstallManager(t)
	source := writeReplaceTestPlugin(t, filepath.Join(root, "source"), "binary")
	_, e := m.InstallFromFile(context.Background(), source, pm.InstallOptions{ExpectedPluginID: "com.powerx.plugins.other", ExpectedVersion: "1.0.0", InstallationRevision: uuid.NewString()})
	require.Error(t, e)
	require.Empty(t, m.opts.Registry.List(context.Background()))
}

func TestInstallTaskCapacityRejectsBeforeArtifactMutation(t *testing.T) {
	m, root := newCredentialInstallManager(t)
	source := writeReplaceTestPlugin(t, filepath.Join(root, "source"), "binary")
	coordinator, e := budget.New(budget.Config{Directory: filepath.Join(root, "budget"), FilesystemPath: root, HostID: "test-host", FilesystemID: "test-fs", InstanceID: "test-instance", MinFreeBytes: 1 << 50, CriticalFreeBytes: 1, MaxConcurrent: 1})
	require.NoError(t, e)
	budget.SetDefault(coordinator)
	defer budget.SetDefault(nil)
	_, e = m.InstallFromFile(context.Background(), source, pm.InstallOptions{InstallationRevision: uuid.NewString()})
	require.ErrorIs(t, e, budget.ErrInsufficient)
	require.Empty(t, m.opts.Registry.List(context.Background()))
	snapshot, e := coordinator.Status(context.Background())
	require.NoError(t, e)
	require.Empty(t, snapshot.Active)
}

func TestDelayedFirstInstallCannotAdoptConcurrentRevision(t *testing.T) {
	m, root := newCredentialInstallManager(t)
	source := writeReplaceTestPlugin(t, filepath.Join(root, "source"), "binary")
	old, e := m.InstallFromFile(context.Background(), source, pm.InstallOptions{})
	require.NoError(t, e)
	_, e = m.InstallFromFile(context.Background(), source, pm.InstallOptions{LocalUpload: true, Force: true, RequireAbsent: true, InstallationRevision: uuid.NewString()})
	require.Error(t, e)
	after, e := m.Get(context.Background(), old.ID)
	require.NoError(t, e)
	require.Equal(t, old.Revision, after.Revision)
}
