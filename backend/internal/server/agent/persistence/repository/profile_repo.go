package repository

import (
	"context"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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

// Upsert 唯一键：env + tenant_uuid + modality + provider + model
func (r *AIModelProfileRepository) UpsertByScopeModalityProviderModel(
	ctx context.Context, env string, tenantUUID *string, in *dbmodel.AIModelProfile,
) error {
	tx := r.db.WithContext(ctx)

	// 写库前强制回写作用域
	in.Env = env
	in.TenantUUID = tenantUUID

	assign := clause.Assignments(map[string]any{
		"label":      in.Label,
		"defaults":   in.Defaults,
		"tags":       in.Tags,
		"updated_at": gorm.Expr("NOW()"),
	})

	// 根据是否有租户，选择对应的冲突列集（匹配上面两条部分唯一索引）
	var conflict clause.OnConflict
	if tenantUUID != nil {
		conflict = clause.OnConflict{
			Columns: []clause.Column{
				{Name: "env"}, {Name: "tenant_uuid"},
				{Name: "modality"}, {Name: "provider"}, {Name: "model"},
			},
			DoUpdates: assign,
		}
	} else {
		conflict = clause.OnConflict{
			Columns: []clause.Column{
				{Name: "env"},
				{Name: "modality"}, {Name: "provider"}, {Name: "model"},
			},
			DoUpdates: assign,
		}
	}

	return tx.Clauses(conflict).Create(in).Error
}

func (r *AIModelProfileRepository) FindByScopeModalityProviderModel(ctx context.Context, env string, tenantUUID *string, modality, provider, model string) (*dbmodel.AIModelProfile, error) {
	var out dbmodel.AIModelProfile
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("modality = ? AND provider = ? AND model = ?", modality, provider, model).
		First(&out).Error
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *AIModelProfileRepository) ListByScope(
	ctx context.Context, env string, tenantUUID *string, modalities ...string,
) ([]dbmodel.AIModelProfile, error) {
	tx := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Model(&dbmodel.AIModelProfile{})

	if len(modalities) > 0 {
		tx = tx.Where("modality IN ?", modalities)
	}

	var out []dbmodel.AIModelProfile
	if err := tx.Order("modality, provider, model").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}
