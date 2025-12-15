// pkg/corex/db/persistence/repository/setting/plugin_instance_config_repo.go
package setting

import (
	"context"
	"strings"

	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PluginInstanceConfigRepository struct{ db *gorm.DB }

func NewPluginInstanceConfigRepository(db *gorm.DB) *PluginInstanceConfigRepository {
	return &PluginInstanceConfigRepository{db: db}
}

func (r *PluginInstanceConfigRepository) with(ctx context.Context) *gorm.DB {
	db := r.db.WithContext(ctx)
	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		db = db.Debug()
	}
	return db
}

func (r *PluginInstanceConfigRepository) Get(ctx context.Context, tenantUUID, pluginID, key string) (*dbsetting.PluginInstanceConfig, error) {
	var err error
	tenantUUID, err = canonicalTenantUUIDStrict(tenantUUID)
	if err != nil {
		return nil, err
	}
	pluginID = strings.TrimSpace(pluginID)
	key = strings.TrimSpace(key)
	var m dbsetting.PluginInstanceConfig
	err = r.with(ctx).
		Where("tenant_uuid = ? AND plugin_id = ? AND key = ?", tenantUUID, pluginID, key).
		First(&m).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *PluginInstanceConfigRepository) Upsert(ctx context.Context, m *dbsetting.PluginInstanceConfig) error {
	canonicalTenant, err := canonicalTenantUUIDStrict(m.TenantUUID)
	if err != nil {
		return err
	}
	m.TenantUUID = canonicalTenant
	m.PluginID = strings.TrimSpace(m.PluginID)
	m.Key = strings.TrimSpace(m.Key)
	return r.with(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_uuid"}, {Name: "plugin_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value_json", "enabled", "updated_at"}),
	}).Create(m).Error
}

func (r *PluginInstanceConfigRepository) SetEnabled(ctx context.Context, tenantUUID, pluginID string, enabled bool) error {
	var err error
	tenantUUID, err = canonicalTenantUUIDStrict(tenantUUID)
	if err != nil {
		return err
	}
	pluginID = strings.TrimSpace(pluginID)
	return r.with(ctx).
		Model(&dbsetting.PluginInstanceConfig{}).
		Where("tenant_uuid = ? AND plugin_id = ?", tenantUUID, pluginID).
		Update("enabled", enabled).Error
}

func (r *PluginInstanceConfigRepository) ListByTenantAndPlugin(ctx context.Context, tenantUUID, pluginID string) ([]*dbsetting.PluginInstanceConfig, error) {
	var err error
	tenantUUID, err = canonicalTenantUUIDStrict(tenantUUID)
	if err != nil {
		return nil, err
	}
	pluginID = strings.TrimSpace(pluginID)
	var list []*dbsetting.PluginInstanceConfig
	err = r.with(ctx).
		Where("tenant_uuid = ? AND plugin_id = ?", tenantUUID, pluginID).
		Find(&list).Error
	return list, err
}

func (r *PluginInstanceConfigRepository) ListEnabledPluginsByTenant(ctx context.Context, tenantUUID string) ([]string, error) {
	var err error
	tenantUUID, err = canonicalTenantUUIDStrict(tenantUUID)
	if err != nil {
		return nil, err
	}
	type row struct{ PluginID string }
	var rows []row
	err = r.with(ctx).
		Model(&dbsetting.PluginInstanceConfig{}).
		Select("DISTINCT plugin_id").
		Where("tenant_uuid = ? AND enabled = ?", tenantUUID, true).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.PluginID)
	}
	return ids, nil
}

// Delete 租户-插件-键 的配置
func (r *PluginInstanceConfigRepository) Delete(ctx context.Context, tenantUUID, pluginID, key string, soft bool) error {
	var err error
	tenantUUID, err = canonicalTenantUUIDStrict(tenantUUID)
	if err != nil {
		return err
	}
	pluginID = strings.TrimSpace(pluginID)
	key = strings.TrimSpace(key)
	db := r.with(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ?", tenantUUID, pluginID, key)
	if !soft {
		db = db.Unscoped()
	}
	return db.Delete(&dbsetting.PluginInstanceConfig{}).Error
}
