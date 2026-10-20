package archive_ops

import (
	"bytes"
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOneTimeMediaWithoutPolicies(t *testing.T) {
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "photo.jpg"), []byte("media A"), 0600))
	s := NewService(context.Background(), Config{Directory: t.TempDir(), Instance: "once", Sources: []Source{{ID: "media", Kind: "media", Directory: source}, {ID: "other", Kind: "media", Directory: source}, {ID: "logs", Kind: "postgresql_logs", Directory: source}}}, Options{})
	t.Cleanup(s.Close)
	ctx := archiveRoot()
	_, err := s.StartOneTime(context.Background(), "media", uuid.NewString(), "root", "")
	require.Equal(t, 403, dto.StatusCode(err))
	_, err = s.StartOneTime(ctx, "../../etc", uuid.NewString(), "root", "")
	require.Equal(t, 400, dto.StatusCode(err))
	_, err = s.StartOneTime(ctx, "logs", uuid.NewString(), "root", "")
	require.Equal(t, 400, dto.StatusCode(err))
	key := uuid.NewString()
	j, err := s.StartOneTime(ctx, "media", key, "root", "trace")
	require.NoError(t, err)
	require.True(t, j.OneTime)
	require.Empty(t, j.PolicyID)
	require.Eventually(t, func() bool { current, e := s.Job(ctx, j.ID); return e == nil && current.Status == "success" }, 5*time.Second, 20*time.Millisecond)
	// 终态清单先写入，worker 仍可能执行保留清理；等待收尾后再下载。
	s.workers.Wait()
	again, err := s.StartOneTime(ctx, "media", key, "root", "trace")
	require.NoError(t, err)
	require.Equal(t, j.ID, again.ID)
	_, err = s.StartOneTime(ctx, "other", key, "root", "trace")
	require.Equal(t, 409, dto.StatusCode(err))
	require.NoError(t, s.Tick(ctx))
	view, err := s.Overview(ctx)
	require.NoError(t, err)
	require.Empty(t, view.Policies)
	require.Len(t, view.Jobs, 1)
	require.Empty(t, view.Jobs[0].Database)
	var archive bytes.Buffer
	require.NoError(t, s.Download(ctx, j.ID, &archive))
	require.Greater(t, archive.Len(), 0)
	r, err := s.StartRestore(ctx, j.ID, false, "root", "")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		current, e := s.RestoreDirectory(ctx, r.ID)
		return e == nil && current.Status == "success"
	}, 5*time.Second, 20*time.Millisecond)
	s.workers.Wait()
	restored, err := s.RestoreDirectory(ctx, r.ID)
	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(restored.Directory, "media/photo.jpg"))
	require.NoError(t, err)
	require.Equal(t, "media A", string(content))
	row, err := s.readJob(j.ID)
	require.NoError(t, err)
	row.CreatedAt = time.Now().Add(-8 * 24 * time.Hour)
	require.NoError(t, s.saveJob(&row))
	_, err = s.StartOneTime(ctx, "media", uuid.NewString(), "root", "")
	require.NoError(t, err)
	require.Eventually(t, func() bool { current, e := s.Job(ctx, j.ID); return e == nil && current.Status == "pruned" }, 5*time.Second, 20*time.Millisecond)
}
