package local

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestRequireExistingPathInitializationNeverCreatesDirectories(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "not-created")
	_, err := New(Options{BasePath: missing, RequireExistingPath: true})
	require.Error(t, err)
	_, err = os.Stat(missing)
	require.True(t, os.IsNotExist(err))
	_, err = New(Options{BasePath: root, RequireExistingPath: true})
	require.NoError(t, err)
}
