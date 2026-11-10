package monitorlogs

import (
	"context"
	"github.com/ArtisanCloud/PowerX/config"
	logcfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorageOwnershipBlocksLegacyDestructiveExecutionAndReenable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "old.log")
	require.NoError(t, os.WriteFile(file, []byte("retain event audit"), 0600))
	at := time.Now().AddDate(0, 0, -30)
	require.NoError(t, os.Chtimes(file, at, at))
	cfg := &config.Config{StorageGovernance: config.StorageGovernanceConfig{LifecycleEnabled: true}, LogConfig: logcfg.LogConfig{Retention: logcfg.RetentionConfig{Enabled: true, DefaultRetentionDays: 7, FilePaths: []string{filepath.Dir(file)}}}}
	s := NewRetentionService(cfg, nil)
	r := s.TriggerNow(context.Background(), "root")
	require.Equal(t, "failed", r.Status)
	require.Zero(t, r.DeletedFiles)
	require.Zero(t, r.DeletedRows)
	require.FileExists(t, file)
	p := s.Policy()
	p.Enabled = true
	_, err := s.UpdatePolicy(context.Background(), p, "root")
	require.Error(t, err)
}
