package ops

import (
	"context"
	"slices"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type BackupArtifactRepository struct {
	*repository.BaseRepository[modelops.BackupArtifact]
	db *gorm.DB
}

func NewBackupArtifactRepository(db *gorm.DB) *BackupArtifactRepository {
	return &BackupArtifactRepository{
		BaseRepository: repository.NewBaseRepository[modelops.BackupArtifact](db),
		db:             db,
	}
}

func (r *BackupArtifactRepository) ListByJobID(ctx context.Context, jobID uint64) ([]modelops.BackupArtifact, error) {
	var rows []modelops.BackupArtifact
	err := r.db.WithContext(ctx).Where("job_id = ?", jobID).Order("id DESC").Find(&rows).Error
	return rows, err
}

func (r *BackupArtifactRepository) DeleteByJobID(ctx context.Context, jobID uint64) error {
	if jobID == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("job_id = ?", jobID).Delete(&modelops.BackupArtifact{}).Error
}

func (r *BackupArtifactRepository) GetLatestByJobID(ctx context.Context, jobID uint64) (*modelops.BackupArtifact, error) {
	if jobID == 0 {
		return nil, nil
	}
	var row modelops.BackupArtifact
	err := r.db.WithContext(ctx).
		Where("job_id = ?", jobID).
		Order("id DESC").
		Limit(1).
		Find(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, nil
	}
	return &row, nil
}

func (r *BackupArtifactRepository) GetLatestByJobIDs(ctx context.Context, jobIDs []uint64) (map[uint64]modelops.BackupArtifact, error) {
	jobIDs = slices.DeleteFunc(jobIDs, func(v uint64) bool { return v == 0 })
	if len(jobIDs) == 0 {
		return map[uint64]modelops.BackupArtifact{}, nil
	}
	var rows []modelops.BackupArtifact
	if err := r.db.WithContext(ctx).
		Where("job_id IN ?", jobIDs).
		Order("job_id ASC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[uint64]modelops.BackupArtifact, len(rows))
	for _, row := range rows {
		if _, exists := out[row.JobID]; exists {
			continue
		}
		out[row.JobID] = row
	}
	return out, nil
}
