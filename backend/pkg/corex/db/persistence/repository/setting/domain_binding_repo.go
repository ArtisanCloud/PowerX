// pkg/corex/db/persistence/repository/setting/domain_binding_repo.go
package setting

import (
	"context"

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

func (r *DomainBindingRepository) GetByTenantAndHost(ctx context.Context, tenantID uint64, host string) (*dbsetting.DomainBinding, error) {
	var m dbsetting.DomainBinding
	err := r.with(ctx).Where("tenant_id = ? AND host = ?", tenantID, host).First(&m).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *DomainBindingRepository) ListByTenant(ctx context.Context, tenantID uint64, onlyActive bool) ([]*dbsetting.DomainBinding, error) {
	var list []*dbsetting.DomainBinding
	db := r.with(ctx).Where("tenant_id = ?", tenantID)
	if onlyActive {
		db = db.Where("active = ?", true)
	}
	if err := db.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// 绑定或更新域名（唯一键：tenant_id + host）
func (r *DomainBindingRepository) Upsert(ctx context.Context, m *dbsetting.DomainBinding) error {
	return r.with(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "host"}},
		DoUpdates: clause.AssignmentColumns([]string{"https_mode", "cert_ref_id", "cdn_domain", "active", "valid_from", "valid_to", "updated_at"}),
	}).Create(m).Error
}

func (r *DomainBindingRepository) Activate(ctx context.Context, tenantID uint64, host string, active bool) error {
	return r.with(ctx).
		Model(&dbsetting.DomainBinding{}).
		Where("tenant_id = ? AND host = ?", tenantID, host).
		Update("active", active).Error
}

func (r *DomainBindingRepository) SwitchCert(ctx context.Context, tenantID uint64, host string, certRefID uint64) error {
	return r.with(ctx).
		Model(&dbsetting.DomainBinding{}).
		Where("tenant_id = ? AND host = ?", tenantID, host).
		Updates(map[string]interface{}{"cert_ref_id": certRefID}).Error
}
