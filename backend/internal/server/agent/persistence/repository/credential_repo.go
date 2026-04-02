package repository

import (
	"context"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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

// Upsert 唯一键：env + tenant_uuid + name + provider
func (r *AIProviderCredentialRepository) UpsertByScopeNameProvider(
	ctx context.Context, env string, tenantUUID *string, in *dbmodel.AIProviderCredential,
) error {
	tx := r.db.WithContext(ctx)

	// 强制回写作用域（Create/Update 都会用到）
	in.Env = env
	in.TenantUUID = tenantUUID

	assign := clause.Assignments(map[string]any{
		"auth_scheme": in.AuthScheme,
		"data":        in.Data,
		"updated_at":  gorm.Expr("NOW()"),
	})

	var conflict clause.OnConflict
	if tenantUUID != nil {
		// 租户内：匹配 ai_cred_uniq_tenant
		conflict = clause.OnConflict{
			Columns: []clause.Column{
				{Name: "env"}, {Name: "tenant_uuid"},
				{Name: "name"}, {Name: "provider"},
			},
			DoUpdates: assign,
		}
	} else {
		// 全局：匹配 ai_cred_uniq_global
		conflict = clause.OnConflict{
			Columns: []clause.Column{
				{Name: "env"},
				{Name: "name"}, {Name: "provider"},
			},
			TargetWhere: clause.Where{
				Exprs: []clause.Expression{clause.Expr{SQL: "tenant_uuid IS NULL"}},
			},
			DoUpdates: assign,
		}
	}

	return tx.Clauses(conflict).Create(in).Error
}

func (r *AIProviderCredentialRepository) FindByScopeNameProvider(ctx context.Context, env string, tenantUUID *string, name, provider string) (*dbmodel.AIProviderCredential, error) {
	var out dbmodel.AIProviderCredential
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("name = ? AND provider = ?", name, provider).
		First(&out).Error
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *AIProviderCredentialRepository) ListByScope(
	ctx context.Context, env string, tenantUUID *string,
) ([]dbmodel.AIProviderCredential, error) {
	var out []dbmodel.AIProviderCredential
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Model(&dbmodel.AIProviderCredential{}).
		Order("provider, name").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}
