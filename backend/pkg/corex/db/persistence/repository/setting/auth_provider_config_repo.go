// pkg/corex/db/persistence/repository/setting/auth_provider_config_repo.go
package setting

import (
	"context"

	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AuthProviderConfigRepository struct{ db *gorm.DB }

func NewAuthProviderConfigRepository(db *gorm.DB) *AuthProviderConfigRepository {
	return &AuthProviderConfigRepository{db: db}
}

func (r *AuthProviderConfigRepository) with(ctx context.Context) *gorm.DB {
	db := r.db.WithContext(ctx)
	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		db = db.Debug()
	}
	return db
}

func (r *AuthProviderConfigRepository) Get(ctx context.Context, tenantID uint64, typ string) (*dbsetting.AuthProviderConfig, error) {
	var m dbsetting.AuthProviderConfig
	err := r.with(ctx).
		Where("tenant_id = ? AND type = ?", tenantID, typ).
		First(&m).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *AuthProviderConfigRepository) Upsert(ctx context.Context, m *dbsetting.AuthProviderConfig) error {
	return r.with(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "type"}},
		DoUpdates: clause.AssignmentColumns([]string{"config_json", "enabled", "verified", "verify_note", "updated_at"}),
	}).Create(m).Error
}

func (r *AuthProviderConfigRepository) SetEnabled(ctx context.Context, tenantID uint64, typ string, enabled bool) error {
	return r.with(ctx).
		Model(&dbsetting.AuthProviderConfig{}).
		Where("tenant_id = ? AND type = ?", tenantID, typ).
		Update("enabled", enabled).Error
}

func (r *AuthProviderConfigRepository) ListEnabledByTenant(ctx context.Context, tenantID uint64) ([]*dbsetting.AuthProviderConfig, error) {
	var list []*dbsetting.AuthProviderConfig
	err := r.with(ctx).Where("tenant_id = ? AND enabled = ?", tenantID, true).Find(&list).Error
	return list, err
}
