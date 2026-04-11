package ops

import (
	"context"
	"strings"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BackupPolicyRepository struct {
	*repository.BaseRepository[modelops.BackupPolicy]
	db *gorm.DB
}

func NewBackupPolicyRepository(db *gorm.DB) *BackupPolicyRepository {
	return &BackupPolicyRepository{
		BaseRepository: repository.NewBaseRepository[modelops.BackupPolicy](db),
		db:             db,
	}
}

func (r *BackupPolicyRepository) List(ctx context.Context, enabled *bool, limit, offset int) ([]modelops.BackupPolicy, int64, error) {
	return r.ListWithFilters(ctx, "", "", "", enabled, limit, offset)
}

func (r *BackupPolicyRepository) ListWithFilters(ctx context.Context, status, keyword, timezone string, enabled *bool, limit, offset int) ([]modelops.BackupPolicy, int64, error) {
	q := r.db.WithContext(ctx).Model(&modelops.BackupPolicy{})
	if enabled != nil {
		q = q.Where("enabled = ?", *enabled)
	} else {
		switch strings.TrimSpace(strings.ToLower(status)) {
		case "enabled":
			q = q.Where("enabled = ?", true)
		case "disabled":
			q = q.Where("enabled = ?", false)
		}
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		q = q.Where("name ILIKE ?", "%"+kw+"%")
	}
	if tz := strings.TrimSpace(timezone); tz != "" {
		q = q.Where("timezone = ?", tz)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []modelops.BackupPolicy
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *BackupPolicyRepository) SetEnabled(ctx context.Context, id uint64, enabled bool, updatedBy string) error {
	return r.db.WithContext(ctx).
		Model(&modelops.BackupPolicy{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"enabled":    enabled,
			"updated_by": strings.TrimSpace(updatedBy),
		}).Error
}

func (r *BackupPolicyRepository) UpsertByName(ctx context.Context, row *modelops.BackupPolicy) (*modelops.BackupPolicy, error) {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"backup_type", "schedule", "retention_days", "enabled", "storage_target", "updated_by", "updated_at",
			}),
		}).
		Create(row).Error
	if err != nil {
		return nil, err
	}
	return row, nil
}
