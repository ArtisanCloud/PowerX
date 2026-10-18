package system_recovery

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestVerificationSessionCannotWriteOriginalDatabase(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	d := Deployment{ID: uuid.NewString(), Root: root, SocketDirectory: filepath.Join(root, "s"), PostgresData: filepath.Join(root, "postgres"), PostgresPort: 15432}
	require.NoError(t, os.Mkdir(d.SocketDirectory, 0700))
	owned := fmt.Sprintf("host=%s port=15432 user=powerx_recovery dbname=powerx sslmode=disable", d.SocketDirectory)
	require.NoError(t, validateVerificationDatabase(d, owned))
	for _, dsn := range []string{"host=127.0.0.1 port=5432 user=powerx dbname=powerx sslmode=disable", fmt.Sprintf("host=%s port=15433 user=powerx_recovery dbname=powerx", d.SocketDirectory), fmt.Sprintf("host=%s port=15432 user=developer dbname=powerx", d.SocketDirectory)} {
		require.Error(t, validateVerificationDatabase(d, dsn))
	}
	d.SourceManaged = true
	require.Error(t, validateVerificationDatabase(d, owned))
	d.SourceManaged = false
	require.NoError(t, os.Remove(d.SocketDirectory))
	require.NoError(t, os.Symlink(t.TempDir(), d.SocketDirectory))
	require.Error(t, validateVerificationDatabase(d, owned))
}
