package knowledge

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type CorpusCheckJobRepository struct {
	*baseRepo.BaseRepository[models.CorpusCheckJob]
	db *gorm.DB
}

func NewCorpusCheckJobRepository(db *gorm.DB) *CorpusCheckJobRepository {
	if db == nil {
		panic("corpus check job repository requires db")
	}
	return &CorpusCheckJobRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.CorpusCheckJob](db),
		db:             db,
	}
}

func (r *CorpusCheckJobRepository) FindByUUID(ctx context.Context, jobUUID uuid.UUID) (*models.CorpusCheckJob, error) {
	if jobUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var job models.CorpusCheckJob
	err := r.db.WithContext(ctx).Where("uuid = ?", jobUUID).Take(&job).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

