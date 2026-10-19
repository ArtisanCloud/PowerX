package snapshot_ops

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestImportPublicationFailureRollsBackContentsAndAllowsRetry(t *testing.T) {
	source, _, _ := fixtureService(t)
	point := captureReady(t, source)
	pack := packageBytes(t, source, point.ID)
	target, _, _ := fixtureService(t)
	other := captureReady(t, target)
	record := target.record("snapshots", point.ID)
	blocked := true
	var outcomes []string
	target.opts.Audit = func(_ context.Context, event AuditEvent) error {
		if event.Action != "import" {
			return nil
		}
		outcomes = append(outcomes, event.Outcome)
		if blocked && event.Outcome == "started" {
			require.NoError(t, os.Mkdir(record, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(record, "block-publication"), []byte("owned fixture"), 0600))
		}
		return nil
	}
	_, err := target.Import(rootContext(), bytes.NewReader(pack), "independent-test-passphrase")
	require.Error(t, err)
	require.NoDirExists(t, target.data(point.ID), "记录发布失败必须回滚正式内容目录")
	staging, err := os.ReadDir(filepath.Join(target.cfg.Directory, "staging"))
	require.NoError(t, err)
	require.Empty(t, staging, "退回暂存的内容由导入收尾清理")
	require.Equal(t, []string{"started", "failed"}, outcomes)
	_, err = target.Get(rootContext(), other.ID)
	require.NoError(t, err, "其他快照保持可读取")
	// 清除本测试自建的失败注入后，同一完整包必须可以正常导入。
	require.NoError(t, os.RemoveAll(record))
	blocked = false
	imported, err := target.Import(rootContext(), bytes.NewReader(pack), "independent-test-passphrase")
	require.NoError(t, err)
	require.Equal(t, point.ID, imported.ID)
	require.True(t, imported.Imported)
	require.Equal(t, []string{"started", "failed", "started", "success"}, outcomes)
	view, err := target.Overview(rootContext())
	require.NoError(t, err)
	require.Len(t, view.Snapshots, 2)
	_, cleanup, err := target.ExportFile(rootContext(), imported.ID, "independent-export-passphrase")
	require.NoError(t, err, "重试导入的文件、清单和密钥仍能通过完整导出校验")
	cleanup()
	_, err = target.Import(rootContext(), bytes.NewReader(pack), "independent-test-passphrase")
	require.ErrorContains(t, err, "此快照已存在")
	require.DirExists(t, target.data(point.ID), "真正重复导入不能回滚既有包")
}

func TestImportPublicationKeepsReadablePackageAfterPublishedSyncError(t *testing.T) {
	s, _, _ := fixtureService(t)
	temp, err := os.MkdirTemp(filepath.Join(s.cfg.Directory, "staging"), "import-")
	require.NoError(t, err)
	defer os.RemoveAll(temp)
	require.NoError(t, os.WriteFile(filepath.Join(temp, "owned-data"), []byte("validated import content"), 0600))
	point := Snapshot{ID: uuid.NewString(), Status: "ready", Phase: "imported", Imported: true, Protected: true}
	syncErr := errors.New("fixture directory sync failed after record publication")
	err = s.publishImportedSnapshot(temp, point, func(file string, value any) (bool, error) {
		published, err := atomicJSONPublished(file, value)
		require.NoError(t, err)
		require.True(t, published)
		return published, syncErr
	})
	require.ErrorIs(t, err, syncErr, "同步错误仍必须返回调用方")
	require.FileExists(t, filepath.Join(s.data(point.ID), "owned-data"), "已发布记录不能指向被误删的内容")
	got, err := s.Get(rootContext(), point.ID)
	require.NoError(t, err)
	require.Equal(t, point.ID, got.ID)
	require.Equal(t, "ready", got.Status)
}

func TestImportPublicationRemovesUnpublishedContentsWhenRollbackRenameFails(t *testing.T) {
	s, _, _ := fixtureService(t)
	temp, err := os.MkdirTemp(filepath.Join(s.cfg.Directory, "staging"), "import-")
	require.NoError(t, err)
	defer os.RemoveAll(temp)
	require.NoError(t, os.WriteFile(filepath.Join(temp, "owned-data"), []byte("new private content"), 0600))
	point := Snapshot{ID: uuid.NewString()}
	publishErr := errors.New("fixture record publication failed")
	err = s.publishImportedSnapshot(temp, point, func(string, any) (bool, error) {
		// 内容已移动后，模拟暂存落点不可被 rename 覆盖。
		require.NoError(t, os.Mkdir(temp, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(temp, "owned-blocker"), []byte("fixture"), 0600))
		return false, publishErr
	})
	require.ErrorIs(t, err, publishErr)
	require.NoDirExists(t, s.data(point.ID), "回退 rename 失败仍应清理未发布内容")
	require.NoFileExists(t, s.record("snapshots", point.ID))
}

func TestAtomicJSONPublicationState(t *testing.T) {
	file := filepath.Join(t.TempDir(), "point.json")
	value := map[string]string{"state": "ready"}
	published, err := atomicJSONPublished(file, value)
	require.NoError(t, err)
	require.True(t, published)
	var got map[string]string
	require.NoError(t, readJSON(file, &got))
	require.Equal(t, value, got)
	blocked := filepath.Join(t.TempDir(), "blocked.json")
	require.NoError(t, os.Mkdir(blocked, 0700))
	published, err = atomicJSONPublished(blocked, value)
	require.Error(t, err)
	require.False(t, published)
	entries, err := os.ReadDir(filepath.Dir(blocked))
	require.NoError(t, err)
	require.Len(t, entries, 1, "失败的原子写入不遗留临时记录")
}
