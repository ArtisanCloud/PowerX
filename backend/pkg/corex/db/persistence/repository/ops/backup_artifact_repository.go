package ops

import (
	"context"

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
