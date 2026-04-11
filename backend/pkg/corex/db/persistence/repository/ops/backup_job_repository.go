package ops

import (
	"context"
	"strings"
	"time"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type BackupJobRepository struct {
	*repository.BaseRepository[modelops.BackupJob]
	db *gorm.DB
}

func NewBackupJobRepository(db *gorm.DB) *BackupJobRepository {
	return &BackupJobRepository{
		BaseRepository: repository.NewBaseRepository[modelops.BackupJob](db),
		db:             db,
	}
}

func (r *BackupJobRepository) List(ctx context.Context, policyID uint64, limit, offset int) ([]modelops.BackupJob, int64, error) {
	return r.ListWithFilters(ctx, policyID, "", nil, nil, limit, offset)
}

func (r *BackupJobRepository) ListWithFilters(ctx context.Context, policyID uint64, status string, from, to *time.Time, limit, offset int) ([]modelops.BackupJob, int64, error) {
	q := r.db.WithContext(ctx).Model(&modelops.BackupJob{})
	if policyID > 0 {
		q = q.Where("policy_id = ?", policyID)
	}
	if normalized := strings.TrimSpace(strings.ToLower(status)); normalized != "" {
		q = q.Where("status = ?", normalized)
	}
	if from != nil {
		q = q.Where("started_at >= ?", *from)
	}
	if to != nil {
		q = q.Where("started_at <= ?", *to)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []modelops.BackupJob
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *BackupJobRepository) ExistsRunningByPolicy(ctx context.Context, policyID uint64) (bool, error) {
	if policyID == 0 {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&modelops.BackupJob{}).
		Where("policy_id = ? AND status = ?", policyID, modelops.BackupJobStatusRunning).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *BackupJobRepository) CountByStatus(ctx context.Context, status modelops.BackupJobStatus) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&modelops.BackupJob{}).
		Where("status = ?", status).
		Count(&count).Error
	return count, err
}

func (r *BackupJobRepository) CountFailedSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&modelops.BackupJob{}).
		Where("status = ? AND started_at >= ?", modelops.BackupJobStatusFailed, since).
		Count(&count).Error
	return count, err
}

func (r *BackupJobRepository) GetLatestSuccess(ctx context.Context) (*modelops.BackupJob, error) {
	var row modelops.BackupJob
	err := r.db.WithContext(ctx).
		Where("status = ?", modelops.BackupJobStatusSuccess).
		Order("ended_at DESC").
		Take(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *BackupJobRepository) ListByPolicyAndStatus(ctx context.Context, policyID uint64, status modelops.BackupJobStatus, limit int) ([]modelops.BackupJob, error) {
	q := r.db.WithContext(ctx).
		Model(&modelops.BackupJob{}).
		Where("policy_id = ? AND status = ?", policyID, status).
		Order("id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []modelops.BackupJob
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *BackupJobRepository) CountConsecutiveFailures(ctx context.Context, policyID uint64, maxDepth int) (int, error) {
	if policyID == 0 {
		return 0, nil
	}
	if maxDepth <= 0 {
		maxDepth = 10
	}
	var rows []modelops.BackupJob
	if err := r.db.WithContext(ctx).
		Model(&modelops.BackupJob{}).
		Where("policy_id = ?", policyID).
		Order("id DESC").
		Limit(maxDepth).
		Find(&rows).Error; err != nil {
		return 0, err
	}
	consecutive := 0
	for _, row := range rows {
		if row.Status != modelops.BackupJobStatusFailed {
			break
		}
		consecutive++
	}
	return consecutive, nil
}
