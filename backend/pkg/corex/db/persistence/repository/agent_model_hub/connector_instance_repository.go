package agent_model_hub

import (
	"context"
	"errors"

	"github.com/google/uuid"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConnectorInstanceRepository persists connector guard state per tenant instance.
type ConnectorInstanceRepository struct {
	*base.BaseRepository[model.ConnectorInstance]
	db *gorm.DB
}

func NewConnectorInstanceRepository(db *gorm.DB) *ConnectorInstanceRepository {
	return &ConnectorInstanceRepository{
		BaseRepository: base.NewBaseRepository[model.ConnectorInstance](db),
		db:             db,
	}
}

func (r *ConnectorInstanceRepository) WithDB(db *gorm.DB) *ConnectorInstanceRepository {
	return NewConnectorInstanceRepository(db)
}

// Upsert stores or updates connector metadata keyed by UUID.
func (r *ConnectorInstanceRepository) Upsert(ctx context.Context, inst *model.ConnectorInstance) (*model.ConnectorInstance, error) {
	if inst == nil {
		return nil, errors.New("instance is nil")
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"env",
			"tenant_scope",
			"platform",
			"region",
			"oauth_ref",
			"webhook_signing_key_ref",
			"mapping_template",
			"sealed_secrets",
			"status",
			"error_rate",
			"last_pause_reason",
			"rate_limit_per_minute",
			"updated_at",
		}),
	}).Create(inst).Error
	if err != nil {
		return nil, err
	}
	return inst, nil
}

func (r *ConnectorInstanceRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*model.ConnectorInstance, error) {
	var record model.ConnectorInstance
	err := r.db.WithContext(ctx).Where("uuid = ?", id).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *ConnectorInstanceRepository) ListByTenant(ctx context.Context, env, tenantScope, platform string) ([]model.ConnectorInstance, error) {
	query := r.db.WithContext(ctx).
		Where("env = ? AND tenant_scope = ?", env, tenantScope)
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	var records []model.ConnectorInstance
	if err := query.Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *ConnectorInstanceRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status, reason string) error {
	updates := map[string]any{
		"status": status,
	}
	if reason != "" {
		updates["last_pause_reason"] = reason
	}
	return r.db.WithContext(ctx).
		Model(&model.ConnectorInstance{}).
		Where("uuid = ?", id).
		Updates(updates).Error
}

// UpdateSealedSecrets stores refreshed encrypted payloads for rotation workflows.
func (r *ConnectorInstanceRepository) UpdateSealedSecrets(ctx context.Context, id uuid.UUID, payload datatypes.JSONMap) error {
	return r.db.WithContext(ctx).
		Model(&model.ConnectorInstance{}).
		Where("uuid = ?", id).
		Update("sealed_secrets", payload).Error
}
