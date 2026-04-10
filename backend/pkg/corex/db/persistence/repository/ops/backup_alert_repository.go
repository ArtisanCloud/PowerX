package ops

import (
	"context"
	"time"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type BackupAlertRepository struct {
	*repository.BaseRepository[modelops.BackupAlert]
	db *gorm.DB
}

func NewBackupAlertRepository(db *gorm.DB) *BackupAlertRepository {
	return &BackupAlertRepository{
		BaseRepository: repository.NewBaseRepository[modelops.BackupAlert](db),
		db:             db,
	}
}

func (r *BackupAlertRepository) List(ctx context.Context, level string, acked *bool, limit, offset int) ([]modelops.BackupAlert, int64, error) {
	q := r.db.WithContext(ctx).Model(&modelops.BackupAlert{})
	if level != "" {
		q = q.Where("level = ?", level)
	}
	if acked != nil {
		q = q.Where("acknowledged = ?", *acked)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []modelops.BackupAlert
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *BackupAlertRepository) Ack(ctx context.Context, id uint64, ackBy string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&modelops.BackupAlert{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"acknowledged": true,
			"ack_by":       ackBy,
			"ack_at":       now,
		}).Error
}
