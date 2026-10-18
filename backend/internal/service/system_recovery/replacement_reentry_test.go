package system_recovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestReplacementCompensationSurvivesCrashAfterRenameBack(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	root := filepath.Join(parent, uuid.NewString())
	require.NoError(t, os.Mkdir(root, 0700))
	store, err := snapshots.NewCoordinatorStore(filepath.Join(parent, "ops"), nil)
	require.NoError(t, err)
	n := Native{Profile: Profile{CandidateDirectory: parent}, Store: store}
	j := snapshots.ManagedJob{ID: uuid.NewString()}
	d := Deployment{ID: filepath.Base(root), Root: root}
	journal := replacementJournal{Current: d, BackupRoot: filepath.Join(parent, "retained-"+j.ID), Moved: true}
	require.NoError(t, snapshots.WriteCoordinatorJSON(n.replacementFile(j.ID), journal))
	// Simulate power loss: B was renamed back, but Moved=true is still on disk.
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(root, ".before-restore-"+j.ID), map[string]string{"job": j.ID, "instance": d.ID}))
	require.NoError(t, os.WriteFile(filepath.Join(root, "business-B"), []byte("test5"), 0600))
	require.NoError(t, n.RevertReplacement(context.Background(), j, "interrupted-"+j.ID))
	require.NoError(t, n.RevertReplacement(context.Background(), j, "interrupted-"+j.ID))
	data, err := os.ReadFile(filepath.Join(root, "business-B"))
	require.NoError(t, err)
	require.Equal(t, "test5", string(data))
	// Missing backup plus an unproven directory must never be accepted as B.
	journal.Moved = true
	require.NoError(t, snapshots.WriteCoordinatorJSON(n.replacementFile(j.ID), journal))
	require.NoError(t, os.Remove(filepath.Join(root, ".before-restore-"+j.ID)))
	require.Error(t, n.RevertReplacement(context.Background(), j, "interrupted-"+j.ID))
}
