package repository

import (
	"context"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistance/model"

	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

// ===============================
// 1) AIProviderCredential Repository
// ===============================
type AIProviderCredentialRepository struct {
	*coreRepo.BaseRepository[dbmodel.AIProviderCredential]
	db *gorm.DB
}

func NewAIProviderCredentialRepository(db *gorm.DB) *AIProviderCredentialRepository {
	return &AIProviderCredentialRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AIProviderCredential](db),
		db:             db,
	}
}

// Upsert 唯一键：env + tenant_id + name + provider
func (r *AIProviderCredentialRepository) UpsertByScopeNameProvider(ctx context.Context, in *dbmodel.AIProviderCredential) error {
	tx := r.db.WithContext(ctx)
	var old dbmodel.AIProviderCredential
	err := tx.Scopes(dbmodel.WithScope(in.Env, in.TenantID)).
		Where("name = ? AND provider = ?", in.Name, in.Provider).
		First(&old).Error
	switch err {
	case nil:
		in.ID = old.ID
		return tx.Save(in).Error
	case gorm.ErrRecordNotFound:
		return tx.Create(in).Error
	default:
		return err
	}
}

func (r *AIProviderCredentialRepository) FindByScopeNameProvider(ctx context.Context, env string, tenantID *uint64, name, provider string) (*dbmodel.AIProviderCredential, error) {
	var out dbmodel.AIProviderCredential
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantID)).
		Where("name = ? AND provider = ?", name, provider).
		First(&out).Error
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ===============================
// 2) AIModelProfile Repository
// ===============================
type AIModelProfileRepository struct {
	*coreRepo.BaseRepository[dbmodel.AIModelProfile]
	db *gorm.DB
}

func NewAIModelProfileRepository(db *gorm.DB) *AIModelProfileRepository {
	return &AIModelProfileRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AIModelProfile](db),
		db:             db,
	}
}

// Upsert 唯一键：env + tenant_id + modality + provider + model
func (r *AIModelProfileRepository) UpsertByScopeModalityProviderModel(ctx context.Context, in *dbmodel.AIModelProfile) error {
	tx := r.db.WithContext(ctx)
	var old dbmodel.AIModelProfile
	err := tx.Scopes(dbmodel.WithScope(in.Env, in.TenantID)).
		Where("modality = ? AND provider = ? AND model = ?", in.Modality, in.Provider, in.Model).
		First(&old).Error
	switch err {
	case nil:
		in.ID = old.ID
		return tx.Save(in).Error
	case gorm.ErrRecordNotFound:
		return tx.Create(in).Error
	default:
		return err
	}
}

func (r *AIModelProfileRepository) FindByScopeModalityProviderModel(ctx context.Context, env string, tenantID *uint64, modality, provider, model string) (*dbmodel.AIModelProfile, error) {
	var out dbmodel.AIModelProfile
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantID)).
		Where("modality = ? AND provider = ? AND model = ?", modality, provider, model).
		First(&out).Error
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ===============================
// 3) AIRoutePolicy Repository
// ===============================
type AIRoutePolicyRepository struct {
	*coreRepo.BaseRepository[dbmodel.AIRoutePolicy]
	db *gorm.DB
}

func NewAIRoutePolicyRepository(db *gorm.DB) *AIRoutePolicyRepository {
	return &AIRoutePolicyRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AIRoutePolicy](db),
		db:             db,
	}
}

// 允许多条策略并存（按 specificity 在 service 层挑）
func (r *AIRoutePolicyRepository) Save(ctx context.Context, in *dbmodel.AIRoutePolicy) error {
	return r.db.WithContext(ctx).Save(in).Error
}

func (r *AIRoutePolicyRepository) ListByScopeModality(ctx context.Context, env string, tenantID *uint64, modality string) ([]*dbmodel.AIRoutePolicy, error) {
	var out []*dbmodel.AIRoutePolicy
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantID)).
		Where("modality = ?", modality).
		Find(&out).Error
	return out, err
}

// ===============================
// 4) AIUsageLog Repository
// ===============================
type AIUsageLogRepository struct {
	*coreRepo.BaseRepository[dbmodel.AIUsageLog]
	db *gorm.DB
}

func NewAIUsageLogRepository(db *gorm.DB) *AIUsageLogRepository {
	return &AIUsageLogRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AIUsageLog](db),
		db:             db,
	}
}

func (r *AIUsageLogRepository) Insert(ctx context.Context, in *dbmodel.AIUsageLog) error {
	return r.db.WithContext(ctx).Create(in).Error
}
