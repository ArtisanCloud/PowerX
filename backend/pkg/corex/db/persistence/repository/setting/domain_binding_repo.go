// pkg/corex/db/persistence/repository/setting/domain_binding_repo.go
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

type DomainBindingRepository struct{ db *gorm.DB }

func NewDomainBindingRepository(db *gorm.DB) *DomainBindingRepository {
	return &DomainBindingRepository{db: db}
}

func (r *DomainBindingRepository) with(ctx context.Context) *gorm.DB {
	db := r.db.WithContext(ctx)
	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		db = db.Debug()
	}
	return db
}

func (r *DomainBindingRepository) GetByTenantUUIDAndHost(ctx context.Context, tenantUUID string, host string) (*dbsetting.DomainBinding, error) {
	return r.GetByScope(ctx, TenantScope{TenantUUID: tenantUUID}, host)
}

func (r *DomainBindingRepository) GetByScope(ctx context.Context, scope TenantScope, host string) (*dbsetting.DomainBinding, error) {
	var m dbsetting.DomainBinding
	err := scope.apply(r.with(ctx)).
		Where("host = ?", host).
		First(&m).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *DomainBindingRepository) ListByTenantUUID(ctx context.Context, tenantUUID string, onlyActive bool) ([]*dbsetting.DomainBinding, error) {
	return r.ListByScope(ctx, TenantScope{TenantUUID: tenantUUID}, onlyActive)
}

func (r *DomainBindingRepository) ListByScope(ctx context.Context, scope TenantScope, onlyActive bool) ([]*dbsetting.DomainBinding, error) {
	var list []*dbsetting.DomainBinding
	db := scope.apply(r.with(ctx))
	if onlyActive {
		db = db.Where("active = ?", true)
	}
	if err := db.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// 绑定或更新域名（唯一键：tenant_uuid + host）
func (r *DomainBindingRepository) Upsert(ctx context.Context, m *dbsetting.DomainBinding) error {
	tenantUUID := strings.TrimSpace(strings.ToLower(m.TenantUUID))
	if tenantUUID == "" {
		return fmt.Errorf("tenant uuid is required")
	}
	m.TenantUUID = tenantUUID
	return r.with(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_uuid"}, {Name: "host"}},
		DoUpdates: clause.AssignmentColumns([]string{"https_mode", "cert_ref_id", "cdn_domain", "active", "valid_from", "valid_to", "updated_at"}),
	}).Create(m).Error
}

func (r *DomainBindingRepository) Activate(ctx context.Context, tenantUUID string, host string, active bool) error {
	return r.ActivateByScope(ctx, TenantScope{TenantUUID: tenantUUID}, host, active)
}

func (r *DomainBindingRepository) ActivateByScope(ctx context.Context, scope TenantScope, host string, active bool) error {
	return scope.apply(r.with(ctx).Model(&dbsetting.DomainBinding{})).
		Where("host = ?", host).
		Update("active", active).Error
}

func (r *DomainBindingRepository) SwitchCert(ctx context.Context, tenantUUID string, host string, certRefID uint64) error {
	return r.SwitchCertByScope(ctx, TenantScope{TenantUUID: tenantUUID}, host, certRefID)
}

func (r *DomainBindingRepository) SwitchCertByScope(ctx context.Context, scope TenantScope, host string, certRefID uint64) error {
	return scope.apply(r.with(ctx).Model(&dbsetting.DomainBinding{})).
		Where("host = ?", host).
		Updates(map[string]interface{}{"cert_ref_id": certRefID}).Error
}
