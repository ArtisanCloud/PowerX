// pkg/corex/db/persistence/repository/setting/tenant_setting_repo.go
package setting

import (
	"context"
	"encoding/json"

	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TenantSettingRepository struct{ db *gorm.DB }

func NewTenantSettingRepository(db *gorm.DB) *TenantSettingRepository {
	return &TenantSettingRepository{db: db}
}

func (r *TenantSettingRepository) with(ctx context.Context) *gorm.DB {
	db := r.db.WithContext(ctx)
	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		db = db.Debug()
	}
	return db
}

func (r *TenantSettingRepository) GetByTenantAndKey(ctx context.Context, tenantUUID, key string) (*dbsetting.TenantSetting, error) {
	var s dbsetting.TenantSetting
	err := r.with(ctx).
		Where("tenant_uuid = ? AND key = ?", tenantUUID, key).
		First(&s).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *TenantSettingRepository) ListByTenantAndPrefix(ctx context.Context, tenantUUID, prefix string) ([]*dbsetting.TenantSetting, error) {
	var list []*dbsetting.TenantSetting
	err := r.with(ctx).
		Where("tenant_uuid = ? AND key LIKE ?", tenantUUID, prefix+"%").
		Find(&list).Error
	return list, err
}

func (r *TenantSettingRepository) Upsert(ctx context.Context, s *dbsetting.TenantSetting) error {
	return r.with(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_uuid"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value_json", "group", "description", "editable", "updated_at"}),
	}).Create(s).Error
}

func (r *TenantSettingRepository) Delete(ctx context.Context, tenantUUID, key string, soft bool) error {
	db := r.with(ctx).Where("tenant_uuid = ? AND key = ?", tenantUUID, key)
	if !soft {
		db = db.Unscoped()
	}
	return db.Delete(&dbsetting.TenantSetting{}).Error
}

// GetEffective 返回 DB 侧“生效值”：tenant 覆盖优先；若无，则回退到 system。
// 关于与 文件/ENV 合并：建议放到上层 Service 完成，这里只管 DB 两层。
func (r *TenantSettingRepository) GetEffective(ctx context.Context, tenantUUID *string, key string, sysRepo *SystemSettingRepository) (value json.RawMessage, source string, err error) {
	// 优先租户
	if tenantUUID != nil {
		if ts, e := r.GetByTenantAndKey(ctx, *tenantUUID, key); e != nil {
			return nil, "", e
		} else if ts != nil {
			return json.RawMessage(ts.ValueJSON), "tenant", nil
		}
	}
	// 回退系统
	ss, e := sysRepo.GetByKey(ctx, key)
	if e != nil {
		return nil, "", e
	}
	if ss != nil {
		return json.RawMessage(ss.ValueJSON), "system", nil
	}
	return nil, "none", nil
}
