package plugin_compat

import (
	"context"
	"errors"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_compat"
	corexrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ExceptionRepository manages compatibility exceptions.
type ExceptionRepository struct {
	corexrepo.BaseRepository[model.CompatException]
	DB *gorm.DB
}

// NewExceptionRepository constructs the repository.
func NewExceptionRepository(db *gorm.DB) *ExceptionRepository {
	return &ExceptionRepository{
		BaseRepository: corexrepo.BaseRepository[model.CompatException]{DB: db},
		DB:            db,
	}
}

// Create inserts an exception.
func (r *ExceptionRepository) Create(ctx context.Context, entity *model.CompatException) (*model.CompatException, error) {
	if entity == nil {
		return nil, errors.New("exception is nil")
	}
	return entity, r.DB.WithContext(ctx).Create(entity).Error
}

// Get fetches by UUID.
func (r *ExceptionRepository) Get(ctx context.Context, id uuid.UUID) (*model.CompatException, error) {
	var record model.CompatException
	if err := r.DB.WithContext(ctx).Take(&record, "uuid = ?", id).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// UpdateStatus updates status/resolution info.
func (r *ExceptionRepository) UpdateStatus(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	if id == uuid.Nil {
		return errors.New("invalid id")
	}
	return r.DB.WithContext(ctx).
		Model(&model.CompatException{}).
		Where("uuid = ?", id).
		Updates(fields).Error
}
