package snapshot_ops

import (
	"context"
	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"time"
)

// PostgresBackend 使用同一 MVCC 事务导出数据库及媒体引用，不阻塞业务 DML。
// 媒体按引用摘要校验；并发删除或覆盖使备份重试，不拒绝客户提交。
type PostgresBackend struct {
	DSN       string
	MediaRoot string
	Restore   func(context.Context, string) (string, string, error)
}

func (b *PostgresBackend) CapturePoint(ctx context.Context, dir string) (string, time.Time, error) {
	j, e := archiveops.CaptureSystemComponent(ctx, dir, archiveops.Source{ID: "media", Name: "媒体与配套数据库", Kind: "media", Directory: b.MediaRoot, DatabaseDSN: b.DSN})
	return j.SnapshotID, j.CreatedAt, e
}
func (b *PostgresBackend) Capture(ctx context.Context, dir string) (string, error) {
	id, _, e := b.CapturePoint(ctx, dir)
	return id, e
}
func (b *PostgresBackend) RestoreDatabase(ctx context.Context, file string) (string, string, error) {
	if b.Restore == nil {
		return "", "", failure(503, "SNAPSHOT_RESTORE_UNAVAILABLE", "未配置隔离数据库恢复")
	}
	return b.Restore(ctx, file)
}
