package backup_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestControlledRetentionRejectsReplacementAndCancellation(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	path := filepath.Join(root, "owned.dump")
	data := []byte("owned backup")
	sum := sha256.Sum256(data)
	artifact := model.BackupArtifact{StorageURI: "file://" + path, SizeBytes: int64(len(data)), Checksum: hex.EncodeToString(sum[:])}
	require.NoError(t, os.WriteFile(path, data, 0600))
	verified, e := verifyControlledArtifact(context.Background(), root, &artifact)
	require.NoError(t, e)
	_, e = controlledArtifactRemovalPath(root, &artifact, verified)
	require.NoError(t, e)
	require.NoError(t, os.Rename(path, path+".before"))
	require.NoError(t, os.WriteFile(path, data, 0600))
	_, e = controlledArtifactRemovalPath(root, &artifact, verified)
	require.Error(t, e)
	_, e = os.Stat(path)
	require.NoError(t, e)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = verifyControlledArtifact(ctx, root, &artifact)
	require.ErrorIs(t, e, context.Canceled)
	require.NoError(t, os.WriteFile(path, []byte("bad checksum"), 0600))
	_, e = verifyControlledArtifact(context.Background(), root, &artifact)
	require.Error(t, e)
}
