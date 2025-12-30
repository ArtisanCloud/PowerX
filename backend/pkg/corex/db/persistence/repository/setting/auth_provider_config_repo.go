// pkg/corex/db/persistence/repository/setting/auth_provider_config_repo.go
package setting

import (
	"context"
	"fmt"
	"strings"

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

func (r *AuthProviderConfigRepository) Get(ctx context.Context, tenantUUID string, typ string) (*dbsetting.AuthProviderConfig, error) {
	return r.GetByScope(ctx, TenantScope{TenantUUID: tenantUUID}, typ)
}

func (r *AuthProviderConfigRepository) GetByScope(ctx context.Context, scope TenantScope, typ string) (*dbsetting.AuthProviderConfig, error) {
	var m dbsetting.AuthProviderConfig
	query := scope.apply(r.with(ctx))
	err := query.
		Where("type = ?", typ).
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
	tenantUUID := strings.TrimSpace(strings.ToLower(m.TenantUUID))
	if tenantUUID == "" {
		return fmt.Errorf("tenant uuid is required")
	}
	m.TenantUUID = tenantUUID
	return r.with(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_uuid"}, {Name: "type"}},
		DoUpdates: clause.AssignmentColumns([]string{"config_json", "enabled", "verified", "verify_note", "updated_at"}),
	}).Create(m).Error
}

func (r *AuthProviderConfigRepository) SetEnabled(ctx context.Context, tenantUUID string, typ string, enabled bool) error {
	return r.SetEnabledByScope(ctx, TenantScope{TenantUUID: tenantUUID}, typ, enabled)
}

func (r *AuthProviderConfigRepository) SetEnabledByScope(ctx context.Context, scope TenantScope, typ string, enabled bool) error {
	return scope.apply(r.with(ctx).Model(&dbsetting.AuthProviderConfig{})).
		Where("type = ?", typ).
		Update("enabled", enabled).Error
}

func (r *AuthProviderConfigRepository) ListEnabled(ctx context.Context, tenantUUID string) ([]*dbsetting.AuthProviderConfig, error) {
	return r.ListEnabledByScope(ctx, TenantScope{TenantUUID: tenantUUID})
}

func (r *AuthProviderConfigRepository) ListEnabledByScope(ctx context.Context, scope TenantScope) ([]*dbsetting.AuthProviderConfig, error) {
	var list []*dbsetting.AuthProviderConfig
	err := scope.apply(r.with(ctx)).
		Where("enabled = ?", true).
		Find(&list).Error
	return list, err
}
