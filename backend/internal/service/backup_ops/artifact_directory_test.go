package backup_ops

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/stretchr/testify/require"
)

func TestBackupArtifactDirectory_ReleaseSwitchPreservesArtifacts(t *testing.T) {
	root := t.TempDir()
	instanceRoot := filepath.Join(root, "powerx")
	firstRelease := filepath.Join(instanceRoot, "releases", "v1", "backend")
	secondRelease := filepath.Join(instanceRoot, "releases", "v2", "backend")
	require.NoError(t, os.MkdirAll(firstRelease, 0o750))
	require.NoError(t, os.MkdirAll(secondRelease, 0o750))
	backendLink := filepath.Join(instanceRoot, "backend")
	require.NoError(t, os.Symlink(firstRelease, backendLink))
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", "")
	t.Setenv("POWERX_LINKS_ROOT", instanceRoot)
	t.Chdir(backendLink)

	firstDir, err := resolveBackupArtifactBaseDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(instanceRoot, "storage", "backups"), firstDir)
	svc := &JobService{artifactBaseDir: firstDir}
	job := &modelops.BackupJob{}
	job.ID = 8
	policy := &modelops.BackupPolicy{}
	policy.ID = 7
	artifactPath, err := svc.prepareArtifactPath(context.Background(), job, policy)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(artifactPath, []byte("retained-backup"), 0o600))

	require.NoError(t, os.Remove(backendLink))
	require.NoError(t, os.Symlink(secondRelease, backendLink))
	t.Chdir(backendLink)
	secondDir, err := resolveBackupArtifactBaseDir()
	require.NoError(t, err)
	require.Equal(t, firstDir, secondDir)
	data, err := os.ReadFile(artifactPath)
	require.NoError(t, err)
	require.Equal(t, "retained-backup", string(data))
}

func TestBackupArtifactDirectory_ExplicitPathAndEnvironmentIsolation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"powerx", "powerx-dev"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", "")
			t.Setenv("POWERX_LINKS_ROOT", filepath.Join(root, name))
			got, err := resolveBackupArtifactBaseDir()
			require.NoError(t, err)
			require.Equal(t, filepath.Join(root, name, "storage", "backups"), got)
		})
	}
	t.Setenv("POWERX_LINKS_ROOT", filepath.Join(root, "powerx"))
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", filepath.Join(root, "independent-volume"))
	got, err := resolveBackupArtifactBaseDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "independent-volume"), got)
}

func TestBackupArtifactDirectory_RelativeConfigurationFails(t *testing.T) {
	for _, name := range []string{"POWERX_OPS_BACKUP_ARTIFACT_DIR", "POWERX_LINKS_ROOT"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", "")
			t.Setenv("POWERX_LINKS_ROOT", "")
			t.Setenv(name, "backend/tmp/backups")
			_, err := resolveBackupArtifactBaseDir()
			require.ErrorContains(t, err, "absolute path")
		})
	}
}

func TestBackupArtifactDirectory_LocalDefaultIsOutsideWorkingDirectory(t *testing.T) {
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", "")
	t.Setenv("POWERX_LINKS_ROOT", "")
	t.Chdir(t.TempDir())
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	got, err := resolveBackupArtifactBaseDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(homeDir, ".powerx", "storage", "backups"), got)
}

func TestBackupArtifactDirectory_InvalidPathDoesNotCreateArtifact(t *testing.T) {
	svc := &JobService{artifactBaseDir: "relative"}
	job := &modelops.BackupJob{}
	job.ID = 1
	policy := &modelops.BackupPolicy{}
	policy.ID = 1
	_, err := svc.prepareArtifactPath(context.Background(), job, policy)
	require.ErrorContains(t, err, "absolute path")
}
