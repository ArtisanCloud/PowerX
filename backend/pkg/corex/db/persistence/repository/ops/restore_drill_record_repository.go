package ops

import (
	"context"
	"strings"
	"time"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type RestoreDrillRecordRepository struct {
	*repository.BaseRepository[modelops.RestoreDrillRecord]
	db *gorm.DB
}

func NewRestoreDrillRecordRepository(db *gorm.DB) *RestoreDrillRecordRepository {
	return &RestoreDrillRecordRepository{
		BaseRepository: repository.NewBaseRepository[modelops.RestoreDrillRecord](db),
		db:             db,
	}
}

func (r *RestoreDrillRecordRepository) ListBySourceJobID(ctx context.Context, sourceJobID uint64, limit, offset int) ([]modelops.RestoreDrillRecord, int64, error) {
	return r.List(ctx, sourceJobID, "", nil, nil, limit, offset)
}

func (r *RestoreDrillRecordRepository) List(ctx context.Context, sourceJobID uint64, status string, from, to *time.Time, limit, offset int) ([]modelops.RestoreDrillRecord, int64, error) {
	q := r.db.WithContext(ctx).Model(&modelops.RestoreDrillRecord{})
	if sourceJobID > 0 {
		q = q.Where("source_job_id = ?", sourceJobID)
	}
	if normalized := strings.TrimSpace(strings.ToLower(status)); normalized != "" {
		q = q.Where("status = ?", normalized)
	}
	if from != nil {
		q = q.Where("created_at >= ?", *from)
	}
	if to != nil {
		q = q.Where("created_at <= ?", *to)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []modelops.RestoreDrillRecord
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *RestoreDrillRecordRepository) GetLatestByPolicy(ctx context.Context, policyID uint64) (*modelops.RestoreDrillRecord, error) {
	if policyID == 0 {
		return nil, nil
	}
	var row modelops.RestoreDrillRecord
	err := r.db.WithContext(ctx).
		Table(modelops.RestoreDrillRecord{}.TableName()+" as rd").
		Joins("JOIN "+modelops.BackupJob{}.TableName()+" as bj ON bj.id = rd.source_job_id").
		Where("bj.policy_id = ?", policyID).
		Order("rd.id DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, nil
	}
	return &row, nil
}
