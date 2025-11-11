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

// ProviderProfileRepository manages Agent Model Hub provider records.
type ProviderProfileRepository struct {
	*base.BaseRepository[model.ProviderProfile]
	db *gorm.DB
}

// NewProviderProfileRepository constructs a repository bound to the given DB handle.
func NewProviderProfileRepository(db *gorm.DB) *ProviderProfileRepository {
	return &ProviderProfileRepository{
		BaseRepository: base.NewBaseRepository[model.ProviderProfile](db),
		db:             db,
	}
}

// WithDB clones the repository with a different DB connection/transaction.
func (r *ProviderProfileRepository) WithDB(db *gorm.DB) *ProviderProfileRepository {
	return NewProviderProfileRepository(db)
}

// UpsertByScopeName enforces uniqueness on (env, tenant_id, name) while updating mutable fields.
func (r *ProviderProfileRepository) UpsertByScopeName(ctx context.Context, profile *model.ProviderProfile) (*model.ProviderProfile, error) {
	if profile == nil {
		return nil, errors.New("profile is nil")
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "env"},
			{Name: "tenant_id"},
			{Name: "name"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"capabilities",
			"primary_endpoint",
			"regions",
			"tenant_whitelist",
			"secret_refs",
			"sealed_secrets",
			"health_score",
			"rollout_status",
			"audit_trail_id",
			"updated_at",
		}),
	}).Create(profile).Error
	if err != nil {
		return nil, err
	}
	return profile, nil
}

// FindByUUID fetches a provider profile by UUID string.
func (r *ProviderProfileRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*model.ProviderProfile, error) {
	var record model.ProviderProfile
	err := r.db.WithContext(ctx).Where("uuid = ?", id).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// FindByName looks up a provider within a scope using its human readable name.
func (r *ProviderProfileRepository) FindByName(ctx context.Context, env string, tenantID *uint64, name string) (*model.ProviderProfile, error) {
	var record model.ProviderProfile
	query := r.db.WithContext(ctx).
		Where("env = ? AND tenant_id IS NOT DISTINCT FROM ? AND name = ?", env, tenantID, name)
	err := query.First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// ListByStatus returns provider profiles filtered by rollout status (all statuses if empty).
func (r *ProviderProfileRepository) ListByStatus(ctx context.Context, env string, status string, limit int) ([]model.ProviderProfile, error) {
	query := r.db.WithContext(ctx).Where("env = ?", env)
	if status != "" {
		query = query.Where("rollout_status = ?", status)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var records []model.ProviderProfile
	if err := query.Order("updated_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// UpdateSecretRefs overwrites the masked Vault references atomically.
func (r *ProviderProfileRepository) UpdateSecretRefs(ctx context.Context, id uuid.UUID, refs datatypes.JSONMap) error {
	return r.db.WithContext(ctx).
		Model(&model.ProviderProfile{}).
		Where("uuid = ?", id).
		Update("secret_refs", refs).Error
}

// UpdateSealedSecrets replaces the encrypted payload for a provider profile.
func (r *ProviderProfileRepository) UpdateSealedSecrets(ctx context.Context, id uuid.UUID, payload datatypes.JSONMap) error {
	return r.db.WithContext(ctx).
		Model(&model.ProviderProfile{}).
		Where("uuid = ?", id).
		Update("sealed_secrets", payload).Error
}

// UpdateFields performs a partial update on the provider profile.
func (r *ProviderProfileRepository) UpdateFields(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&model.ProviderProfile{}).
		Where("uuid = ?", id).
		Updates(updates).Error
}

// UpdateHealthScore sets the latest validator score for observability.
func (r *ProviderProfileRepository) UpdateHealthScore(ctx context.Context, id uuid.UUID, score float64) error {
	return r.db.WithContext(ctx).
		Model(&model.ProviderProfile{}).
		Where("uuid = ?", id).
		Update("health_score", score).Error
}
