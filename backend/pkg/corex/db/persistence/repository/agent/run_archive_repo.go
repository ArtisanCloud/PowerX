package agent

import (
	"context"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MarkRunArchived 仅保存已完成运行的低频归档定位；相同定位可幂等补写。
func (r *AdminRunRepository) MarkRunArchived(ctx context.Context, tenant uuid.UUID, env string, id uuid.UUID, key string, at time.Time) error {
	return markArchive(r.db.WithContext(ctx).Model(&m.AdminRunAdmission{}).
		Where("tenant_uuid = ? AND env = ? AND uuid = ?", tenant, env, id), key, at)
}

func (r *ServiceSessionRepository) MarkRunArchived(ctx context.Context, owner SessionOwner, env string, session, id uuid.UUID, key string, at time.Time) error {
	return markArchive(r.scoped(ctx, owner).Model(&m.ServiceInvocation{}).
		Where("run_env = ? AND session_uuid = ? AND uuid = ?", env, session, id), key, at)
}

func markArchive(q *gorm.DB, key string, at time.Time) error {
	if key == "" || at.IsZero() {
		return gorm.ErrInvalidData
	}
	result := q.Where("admission_state = ? AND finished_at IS NOT NULL", "admitted").
		Where("archive_key IS NULL OR archive_key = ? OR archive_key = ?", "", key).
		Updates(map[string]any{"archive_key": key, "archived_at": at.UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ArchiveBacklog 只供 Core 运维扫描器按环境聚合低频终态索引。
func (r *AdminRunRepository) ArchiveBacklog(ctx context.Context, env string) (int64, *time.Time, error) {
	return archiveBacklog(r.db.WithContext(ctx).Model(&m.AdminRunAdmission{}).Where("env = ?", env))
}
func (r *ServiceSessionRepository) ArchiveBacklog(ctx context.Context, env string) (int64, *time.Time, error) {
	return archiveBacklog(r.db.WithContext(ctx).Model(&m.ServiceInvocation{}).Where("run_env = ?", env))
}
func archiveBacklog(q *gorm.DB) (int64, *time.Time, error) {
	q = q.Where("admission_state = ? AND finished_at IS NOT NULL AND archived_at IS NULL", "admitted")
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, nil, err
	}
	var rows []struct{ FinishedAt time.Time }
	if err := q.Select("finished_at").Order("finished_at ASC").Limit(1).Find(&rows).Error; err != nil {
		return 0, nil, err
	}
	if len(rows) == 0 {
		return count, nil, nil
	}
	return count, &rows[0].FinishedAt, nil
}
func (r *AdminRunRepository) MarkHotExpiry(ctx context.Context, tenant uuid.UUID, env string, id uuid.UUID, key string, at time.Time) error {
	return markHotExpiry(r.db.WithContext(ctx).Model(&m.AdminRunAdmission{}).Where("tenant_uuid = ? AND env = ? AND uuid = ?", tenant, env, id), key, at)
}
func (r *ServiceSessionRepository) MarkHotExpiry(ctx context.Context, owner SessionOwner, env string, session, id uuid.UUID, key string, at time.Time) error {
	return markHotExpiry(r.scoped(ctx, owner).Model(&m.ServiceInvocation{}).Where("run_env = ? AND session_uuid = ? AND uuid = ?", env, session, id), key, at)
}
func markHotExpiry(q *gorm.DB, key string, at time.Time) error {
	if key == "" || at.IsZero() {
		return gorm.ErrInvalidData
	}
	result := q.Where("admission_state = ? AND finished_at IS NOT NULL AND archived_at IS NOT NULL AND archive_key = ?", "admitted", key).Update("hot_expires_at", at.UTC())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
