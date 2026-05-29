// pkg/corex/db/persistence/repository/setting/plugin_instance_config_repo.go
package setting

import (
	"context"
	"strings"
	"time"

	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/gorm"
)

type PluginInstanceConfigRepository struct{ db *gorm.DB }

func NewPluginInstanceConfigRepository(db *gorm.DB) *PluginInstanceConfigRepository {
	return &PluginInstanceConfigRepository{db: db}
}

func (r *PluginInstanceConfigRepository) DB() *gorm.DB {
	if r == nil {
		return nil
	}
	return r.db
}

const KeyClientCredentials = "auth.credentials"

type TenantPluginBinding struct {
	TenantUUID string
	PluginID   string
}

type ListTenantPluginOptions struct {
	TenantUUIDs []string
	PluginIDs   []string
	Statuses    []string
	Key         string
	OnlyEnabled bool
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
	m.Status = dbsetting.NormalizePluginInstanceStatus(m.Status, m.Enabled)
	now := time.Now()
	values := map[string]any{
		"value_json":         m.ValueJSON,
		"enabled":            m.Enabled,
		"status":             m.Status,
		"drain_job_id":       m.DrainJobID,
		"drain_requested_at": m.DrainRequestedAt,
		"drained_at":         m.DrainedAt,
		"updated_at":         now,
	}
	tx := r.with(ctx).
		Model(&dbsetting.PluginInstanceConfig{}).
		Where("tenant_uuid = ? AND plugin_id = ? AND key = ?", m.TenantUUID, m.PluginID, m.Key).
		Updates(values)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected > 0 {
		return nil
	}
	values["tenant_uuid"] = m.TenantUUID
	values["plugin_id"] = m.PluginID
	values["key"] = m.Key
	values["created_at"] = now
	return r.with(ctx).Model(&dbsetting.PluginInstanceConfig{}).Create(values).Error
}

func (r *PluginInstanceConfigRepository) SetEnabled(ctx context.Context, tenantUUID, pluginID string, enabled bool) error {
	var err error
	tenantUUID, err = canonicalTenantUUIDStrict(tenantUUID)
	if err != nil {
		return err
	}
	pluginID = strings.TrimSpace(pluginID)
	status := dbsetting.PluginInstanceStatusDisabled
	if enabled {
		status = dbsetting.PluginInstanceStatusEnabled
	}
	return r.with(ctx).
		Model(&dbsetting.PluginInstanceConfig{}).
		Where("tenant_uuid = ? AND plugin_id = ?", tenantUUID, pluginID).
		Updates(map[string]any{"enabled": enabled, "status": status, "updated_at": time.Now()}).Error
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

func (r *PluginInstanceConfigRepository) ListTenantPluginBindings(ctx context.Context, opts ListTenantPluginOptions) ([]TenantPluginBinding, error) {
	db := r.with(ctx).Model(&dbsetting.PluginInstanceConfig{})

	if opts.Key != "" {
		db = db.Where("key = ?", strings.TrimSpace(opts.Key))
	}
	if opts.OnlyEnabled {
		if r.db != nil && r.db.Dialector != nil && strings.EqualFold(r.db.Dialector.Name(), "sqlite") {
			db = db.Where("enabled = 1")
		} else {
			db = db.Where("enabled = ?", true)
		}
	}
	if len(opts.Statuses) > 0 {
		statuses := make([]string, 0, len(opts.Statuses))
		for _, status := range opts.Statuses {
			if trimmed := strings.TrimSpace(status); trimmed != "" {
				statuses = append(statuses, trimmed)
			}
		}
		if len(statuses) > 0 {
			db = db.Where("status IN ?", statuses)
		}
	}
	if len(opts.TenantUUIDs) > 0 {
		tenantIDs := make([]string, 0, len(opts.TenantUUIDs))
		for _, tenant := range opts.TenantUUIDs {
			canonical, err := canonicalTenantUUIDStrict(tenant)
			if err != nil {
				return nil, err
			}
			tenantIDs = append(tenantIDs, canonical)
		}
		db = db.Where("tenant_uuid IN ?", tenantIDs)
	}
	if len(opts.PluginIDs) > 0 {
		normalized := make([]string, 0, len(opts.PluginIDs))
		for _, plugin := range opts.PluginIDs {
			if trimmed := strings.TrimSpace(plugin); trimmed != "" {
				normalized = append(normalized, trimmed)
			}
		}
		if len(normalized) > 0 {
			db = db.Where("plugin_id IN ?", normalized)
		}
	}

	var rows []TenantPluginBinding
	err := db.Select("tenant_uuid", "plugin_id").
		Distinct("tenant_uuid", "plugin_id").
		Order("tenant_uuid, plugin_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *PluginInstanceConfigRepository) CountTenantPluginBindings(ctx context.Context, opts ListTenantPluginOptions) (int64, error) {
	rows, err := r.ListTenantPluginBindings(ctx, opts)
	if err != nil {
		return 0, err
	}
	return int64(len(rows)), nil
}

func (r *PluginInstanceConfigRepository) CountActiveTenantPluginBindings(ctx context.Context, pluginID string) (int64, error) {
	return r.CountTenantPluginBindings(ctx, ListTenantPluginOptions{
		PluginIDs: []string{pluginID},
		Key:       KeyClientCredentials,
		Statuses: []string{
			dbsetting.PluginInstanceStatusAvailable,
			dbsetting.PluginInstanceStatusSubscribed,
			dbsetting.PluginInstanceStatusEnabled,
			dbsetting.PluginInstanceStatusDisabled,
			dbsetting.PluginInstanceStatusExpired,
		},
	})
}

func (r *PluginInstanceConfigRepository) CountDrainedTenantPluginBindings(ctx context.Context, pluginID string) (int64, error) {
	return r.CountTenantPluginBindings(ctx, ListTenantPluginOptions{
		PluginIDs: []string{pluginID},
		Key:       KeyClientCredentials,
		Statuses:  []string{dbsetting.PluginInstanceStatusDrained},
	})
}

func (r *PluginInstanceConfigRepository) DeleteDrainedTenantPluginBindings(ctx context.Context, pluginID string) (int64, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return 0, nil
	}
	res := r.with(ctx).
		Unscoped().
		Where("plugin_id = ? AND key = ? AND status = ?", pluginID, KeyClientCredentials, dbsetting.PluginInstanceStatusDrained).
		Delete(&dbsetting.PluginInstanceConfig{})
	return res.RowsAffected, res.Error
}

func (r *PluginInstanceConfigRepository) MarkPluginInstancesDraining(ctx context.Context, pluginID, drainJobID string, now time.Time) (int64, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return 0, nil
	}
	db := r.with(ctx).
		Model(&dbsetting.PluginInstanceConfig{}).
		Where("plugin_id = ? AND key = ?", pluginID, KeyClientCredentials).
		Where("status <> ?", dbsetting.PluginInstanceStatusDrained)
	res := db.Updates(map[string]any{
		"enabled":            false,
		"status":             dbsetting.PluginInstanceStatusDrainingRequested,
		"drain_job_id":       strings.TrimSpace(drainJobID),
		"drain_requested_at": now,
		"updated_at":         now,
	})
	return res.RowsAffected, res.Error
}

func (r *PluginInstanceConfigRepository) MarkPluginInstancesDisabledByPlatform(ctx context.Context, pluginID, drainJobID string, now time.Time) (int64, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return 0, nil
	}
	db := r.with(ctx).
		Model(&dbsetting.PluginInstanceConfig{}).
		Where("plugin_id = ? AND key = ?", pluginID, KeyClientCredentials).
		Where("status <> ?", dbsetting.PluginInstanceStatusDrained)
	res := db.Updates(map[string]any{
		"enabled":            false,
		"status":             dbsetting.PluginInstanceStatusDisabledByPlatform,
		"drain_job_id":       strings.TrimSpace(drainJobID),
		"drain_requested_at": now,
		"updated_at":         now,
	})
	return res.RowsAffected, res.Error
}

func (r *PluginInstanceConfigRepository) MarkPluginDrainInstancesDrained(ctx context.Context, pluginID, drainJobID string, now time.Time) (int64, error) {
	pluginID = strings.TrimSpace(pluginID)
	drainJobID = strings.TrimSpace(drainJobID)
	if pluginID == "" || drainJobID == "" {
		return 0, nil
	}
	res := r.with(ctx).
		Model(&dbsetting.PluginInstanceConfig{}).
		Where("plugin_id = ? AND key = ?", pluginID, KeyClientCredentials).
		Where("status IN ?", []string{
			dbsetting.PluginInstanceStatusAvailable,
			dbsetting.PluginInstanceStatusSubscribed,
			dbsetting.PluginInstanceStatusEnabled,
			dbsetting.PluginInstanceStatusDisabled,
			dbsetting.PluginInstanceStatusDrainingRequested,
			dbsetting.PluginInstanceStatusDisabledByPlatform,
			dbsetting.PluginInstanceStatusExpired,
		}).
		Updates(map[string]any{
			"enabled":            false,
			"status":             dbsetting.PluginInstanceStatusDrained,
			"drain_job_id":       drainJobID,
			"drain_requested_at": now,
			"drained_at":         now,
			"updated_at":         now,
		})
	return res.RowsAffected, res.Error
}

func (r *PluginInstanceConfigRepository) DisableTenantPluginBindings(ctx context.Context, tenantUUID, key string) (int64, error) {
	canonicalTenant, err := canonicalTenantUUIDStrict(tenantUUID)
	if err != nil {
		return 0, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = KeyClientCredentials
	}
	res := r.with(ctx).
		Model(&dbsetting.PluginInstanceConfig{}).
		Where("tenant_uuid = ? AND key = ? AND enabled = ?", canonicalTenant, key, true).
		Updates(map[string]any{
			"enabled":    false,
			"status":     dbsetting.PluginInstanceStatusDisabled,
			"updated_at": time.Now(),
		})
	return res.RowsAffected, res.Error
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
