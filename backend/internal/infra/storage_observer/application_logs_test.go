package storageobserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/stretchr/testify/require"
)

func TestStorageApplicationLogsMeasureOnlyConfiguredFilesAndProtectUnsafeCandidates(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	old := time.Now().Add(-40 * 24 * time.Hour)
	for _, name := range []string{"info.log", "info-2020-01-01T00-00-00.000.log", "info-2020-02-01T00-00-00.000.log", "info-2020-03-01T00-00-00.000.log", "private-business.txt"} {
		p := filepath.Join(root, name)
		require.NoError(t, os.WriteFile(p, []byte("test"), 0600))
		require.NoError(t, os.Chtimes(p, old, old))
	}
	// 旧文件的硬链接仍被其他路径持有，不能当作可释放空间或删除候选。
	require.NoError(t, os.Link(filepath.Join(root, "info-2020-01-01T00-00-00.000.log"), filepath.Join(root, "retained-reference")))
	require.NoError(t, os.Symlink(filepath.Join(root, "private-business.txt"), filepath.Join(root, "info-2019-01-01T00-00-00.000.log")))
	a := &ApplicationLogs{Sources: map[string]ApplicationLogSource{"logs": {Directory: root, ActiveFiles: []string{"info.log"}}}}
	scan, err := a.Inspect(context.Background(), "logs", time.Now().Add(-7*24*time.Hour))
	require.NoError(t, err)
	require.True(t, scan.Complete)
	require.Len(t, scan.Candidates, 1)
	require.Equal(t, "info-2020-02-01T00-00-00.000.log", scan.Candidates[0].Name)
	require.GreaterOrEqual(t, scan.Skipped["unsafe_file"], 2)
	// 预览后替换为软链，执行复核必须拒绝，目标业务文件仍在。
	c := scan.Candidates[0]
	require.NoError(t, os.Remove(filepath.Join(root, c.Name)))
	require.NoError(t, os.Symlink(filepath.Join(root, "private-business.txt"), filepath.Join(root, c.Name)))
	outcome, err := a.Remove(context.Background(), "logs", c)
	require.NoError(t, err)
	require.Equal(t, "changed", outcome)
	require.FileExists(t, filepath.Join(root, "private-business.txt"))
	snapshot := Collector{}.Collect(context.Background(), root, []gov.Resource{{ID: "logs", Path: root, Kind: "application_logs", LogActiveFiles: []string{"info.log"}}})
	require.Equal(t, int64(3), snapshot.Resources[0].Files) // 活动文件、硬链接旧文件和最新轮转；软链和无关文件不计。
	// 同用户之外可写的目录不具备可靠所有权；即使名字像日志也不能清理。
	require.NoError(t, os.Chmod(root, 0777))
	scan, err = a.Inspect(context.Background(), "logs", old)
	require.NoError(t, err)
	require.False(t, scan.Complete)
	require.Equal(t, 1, scan.Skipped["unsafe_directory"])
}
