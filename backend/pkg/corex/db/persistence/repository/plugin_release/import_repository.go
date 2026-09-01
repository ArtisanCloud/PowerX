package plugin_release

import (
	"context"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ImportRepository persists PluginImportRun records.
type ImportRepository struct {
	db *gorm.DB
}

// NewImportRepository constructs a repository bound to db.
func NewImportRepository(db *gorm.DB) *ImportRepository {
	return &ImportRepository{db: db}
}

// Create inserts a new import run.
func (r *ImportRepository) Create(ctx context.Context, run *model.PluginImportRun) error {
	if run == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(run).Error
}

// UpdateStatus updates status, risk or findings for the run.
func (r *ImportRepository) UpdateStatus(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&model.PluginImportRun{}).
		Where("id = ?", id).
		Updates(fields).
		Error
}

// MarkCompleted sets completion metadata.
func (r *ImportRepository) MarkCompleted(ctx context.Context, id uuid.UUID, status, risk string, note string) error {
	update := map[string]any{
		"status":        status,
		"risk_level":    risk,
		"approval_note": note,
		"completed_at":  time.Now().UTC(),
	}
	return r.UpdateStatus(ctx, id, update)
}

// FindByID returns the run by identifier.
func (r *ImportRepository) FindByTenantUUID(ctx context.Context, tenantUUID string, id uuid.UUID) (*model.PluginImportRun, error) {
	var run model.PluginImportRun
	if err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND CAST(uuid AS TEXT) = ?", tenantUUID, id.String()).First(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *ImportRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.PluginImportRun, error) {
	var run model.PluginImportRun
	if err := r.db.WithContext(ctx).Where("CAST(uuid AS TEXT) = ?", id.String()).First(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}
