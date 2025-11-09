package plugin_sandbox

import (
	"context"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_sandbox"
	corexrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RunRepository persists sandbox validation runs.
type RunRepository struct {
	corexrepo.BaseRepository[model.SandboxValidationRun]
}

// NewRunRepository constructs the repository.
func NewRunRepository(db *gorm.DB) *RunRepository {
	return &RunRepository{
		BaseRepository: corexrepo.BaseRepository[model.SandboxValidationRun]{DB: db},
	}
}

// Create inserts a new sandbox run.
func (r *RunRepository) Create(ctx context.Context, run *model.SandboxValidationRun) (*model.SandboxValidationRun, error) {
	if run == nil {
		return nil, nil
	}
	err := r.DB.WithContext(ctx).Create(run).Error
	return run, err
}

// UpdateFields updates provided columns for a run.
func (r *RunRepository) UpdateFields(ctx context.Context, id uuid.UUID, values map[string]any) error {
	return r.DB.WithContext(ctx).
		Model(&model.SandboxValidationRun{}).
		Where("uuid = ?", id).
		Updates(values).
		Error
}

// Get fetches a sandbox run.
func (r *RunRepository) Get(ctx context.Context, id uuid.UUID) (*model.SandboxValidationRun, error) {
	var run model.SandboxValidationRun
	if err := r.DB.WithContext(ctx).First(&run, "uuid = ?", id).Error; err != nil {
		return nil, err
	}
	return &run, nil
}
