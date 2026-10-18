package snapshot_ops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestLegacySnapshotWithoutComponentsRemainsReadableWithoutRewritingLedger(t *testing.T) {
	for _, status := range []string{"queued", "failed", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			s, _, _ := fixtureService(t)
			id := uuid.NewString()
			file := s.record("snapshots", id)
			// 模拟采集组件前失败的旧账本，与线上 null 字段一致。
			require.NoError(t, atomicJSON(file, Snapshot{ID: id, Status: status, Phase: status, Error: "legacy capture error"}))
			before, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Contains(t, string(before), `"components": null`)
			row, err := s.Get(rootContext(), id)
			require.NoError(t, err)
			require.NotNil(t, row.Components)
			require.Empty(t, row.Components)
			out, err := s.Overview(rootContext())
			require.NoError(t, err)
			require.Len(t, out.Snapshots, 1)
			require.Equal(t, status, out.Snapshots[0].Status)
			require.Equal(t, "legacy capture error", out.Snapshots[0].Error)
			body, err := json.Marshal(out)
			require.NoError(t, err)
			require.Contains(t, string(body), `"components":[]`)
			after, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Equal(t, before, after, "读取不能删除或改写历史任务")
		})
	}
}

func TestSnapshotDefaultStoragePreservesExistingDirectory(t *testing.T) {
	t.Setenv("POWERX_LINKS_ROOT", "")
	home, e := os.UserHomeDir()
	require.NoError(t, e)
	dir, source := DefaultStorage()
	require.Equal(t, filepath.Join(home, ".powerx/storage/system-snapshots"), dir)
	require.Equal(t, "default", source)
	root := t.TempDir()
	t.Setenv("POWERX_LINKS_ROOT", root)
	dir, source = DefaultStorage()
	require.Equal(t, filepath.Join(root, "storage/system-snapshots"), dir)
	require.Equal(t, "install_root", source)
}
func TestSnapshotOverviewReportsActualCanonicalStoreNotDynamicSourceDirectory(t *testing.T) {
	seed, b, _ := fixtureService(t)
	configured := filepath.Join(t.TempDir(), "YAML store not created")
	sourceCfg := seed.cfg
	sourceCfg.Directory = configured
	sourceCfg.DirectorySource = "configuration"
	actual := t.TempDir()
	alias := filepath.Join(t.TempDir(), "coordinator store alias")
	require.NoError(t, os.Symlink(actual, alias))
	cfg := sourceCfg
	cfg.Directory = alias
	cfg.DirectorySource = "recovery_profile"
	service := NewService(context.Background(), cfg, Options{Backend: b, Sources: func(context.Context) (Config, error) { return sourceCfg, nil }})
	defer service.Close()
	overview, e := service.Overview(rootContext())
	require.NoError(t, e)
	canonical, e := filepath.EvalSymlinks(actual)
	require.NoError(t, e)
	require.Equal(t, canonical, overview.Directory)
	require.Equal(t, "recovery_profile", overview.DirectorySource)
	capture := captureReady(t, service)
	_, e = os.Stat(filepath.Join(overview.Directory, "contents", capture.ID, "manifest.json"))
	require.NoError(t, e)
	_, e = os.Stat(configured)
	require.True(t, os.IsNotExist(e), "source enumeration must not redirect the effective store")
	_, e = service.Overview(context.Background())
	require.Error(t, e, "unauthorized calls must not expose the private directory")
}
