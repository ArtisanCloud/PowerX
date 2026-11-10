package storagegovrepo

import (
	"context"
	"database/sql"
	"errors"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	tenants "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

// ProbeRetentionID 是旧健康探测的固定适配器标识。
const ProbeRetentionID = "capability_registry_health_probe_results"

// DiscoveryRetentionID 是已退休发现缓存的固定适配器标识。
const DiscoveryRetentionID = "capability_registry_discovery_cache"

// SystemRetentionRepository 只包含经审查的两个固定适配器；禁止任意表、时间列和谓词。
type SystemRetentionRepository struct{ db *gorm.DB }

// NewSystemRetentionRepository 绑定当前部署数据库，不接收动态表配置。
func NewSystemRetentionRepository(db *gorm.DB) *SystemRetentionRepository {
	return &SystemRetentionRepository{db: db}
}

// SystemRetentionSupported 只批准已逐类审查的固定规则。
func SystemRetentionSupported(id string) bool {
	return id == ProbeRetentionID || id == DiscoveryRetentionID
}
func quotedRelation(name string) string {
	parts := strings.Split(name, ".")
	for i := range parts {
		parts[i] = `"` + strings.ReplaceAll(parts[i], `"`, `""`) + `"`
	}
	return strings.Join(parts, ".")
}

// Ready 只检查字段、有效索引及外部引用，不自动执行 DDL。
func (r *SystemRetentionRepository) Ready(ctx context.Context, id string) error {
	if !SystemRetentionSupported(id) {
		return errors.New("system_retention_not_approved")
	}
	if r == nil || r.db == nil || r.db.Dialector.Name() != "postgres" {
		return errors.New("postgres_required")
	}
	var model any
	var table, index string
	var columns []string
	if id == ProbeRetentionID {
		model = &m.HealthProbeResult{}
		table = m.HealthProbeResult{}.TableName()
		index = "idx_registry_probe_retention"
		columns = []string{"adapter_id", "probe_window_start", "status", "next_retry_at", "updated_at", "deleted_at"}
	} else {
		model = &m.DiscoveryCacheEntry{}
		table = m.DiscoveryCacheEntry{}.TableName()
		index = "idx_registry_discovery_expiry"
		columns = []string{"tenant_uuid", "capability_id", "client_id", "expires_at", "issued_at", "updated_at", "deleted_at"}
	}
	db := r.db.WithContext(ctx)
	if !db.Migrator().HasTable(model) {
		return errors.New("system_retention_table_missing")
	}
	for _, c := range columns {
		if !db.Migrator().HasColumn(model, c) {
			return errors.New("system_retention_migration_required")
		}
	}
	if !db.Migrator().HasIndex(model, index) {
		return errors.New("system_retention_index_required")
	}
	var validIndex bool
	if e := db.Raw(`SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_index i JOIN pg_catalog.pg_class idx ON idx.oid=i.indexrelid WHERE i.indrelid=to_regclass(?) AND idx.relname=? AND i.indisvalid AND i.indisready)`, quotedRelation(table), index).Scan(&validIndex).Error; e != nil {
		return e
	}
	if !validIndex {
		return errors.New("system_retention_index_required")
	}
	if id == DiscoveryRetentionID && !db.Migrator().HasTable(&tenants.Tenant{}) {
		return errors.New("tenant_registry_unavailable")
	}
	var refs int64
	if e := db.Raw(`SELECT count(*) FROM pg_catalog.pg_constraint WHERE contype='f' AND confrelid=to_regclass(?)`, quotedRelation(table)).Scan(&refs).Error; e != nil {
		return e
	}
	if refs != 0 {
		return errors.New("system_retention_external_references")
	}
	return ctx.Err()
}

// Batch 返回有界候选或实际删除数；未知状态、有效缓存、当前探测与未来重试保持原样。
func (r *SystemRetentionRepository) Batch(ctx context.Context, id string, cutoff, now time.Time, limit int, apply bool) (int64, error) {
	if !SystemRetentionSupported(id) || limit < 1 || limit > 500 || !cutoff.Before(now) {
		return 0, errors.New("system_retention_invalid")
	}
	if e := r.Ready(ctx, id); e != nil {
		return 0, e
	}
	var total int64
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SET LOCAL lock_timeout='500ms'").Error; e != nil {
			return e
		}
		if e := tx.Exec("SET LOCAL statement_timeout='2s'").Error; e != nil {
			return e
		}
		if apply {
			table := m.HealthProbeResult{}.TableName()
			if id == DiscoveryRetentionID {
				table = m.DiscoveryCacheEntry{}.TableName()
			}
			// 与普通写入兼容，但阻止在引用检查后并发添加级联外键。
			if e := tx.Exec("LOCK TABLE " + quotedRelation(table) + " IN ROW EXCLUSIVE MODE").Error; e != nil {
				return e
			}
			if e := NewSystemRetentionRepository(tx).Ready(ctx, id); e != nil {
				return e
			}
		}
		if id == ProbeRetentionID {
			table := quotedRelation(m.HealthProbeResult{}.TableName())
			q := tx.Unscoped().Model(&m.HealthProbeResult{}).Table(table+" AS x").Where("x.adapter_id <> '' AND x.probe_window_start < ? AND x.updated_at < ? AND x.status IN ? AND x.next_retry_at IS NOT NULL AND x.next_retry_at <= ?", cutoff, cutoff, []string{"healthy", "unhealthy", "degraded"}, now).
				Where("EXISTS (SELECT 1 FROM " + table + " AS newer WHERE newer.adapter_id=x.adapter_id AND newer.probe_window_start>x.probe_window_start AND newer.deleted_at IS NULL)")
			if apply {
				q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED", Table: clause.Table{Name: "x"}})
			}
			var keys []m.HealthProbeResult
			if e := q.Select("x.adapter_id,x.probe_window_start").Order("x.probe_window_start,x.adapter_id").Limit(limit).Find(&keys).Error; e != nil {
				return e
			}
			total = int64(len(keys))
			if !apply || len(keys) == 0 {
				return nil
			}
			pairs := make([][]any, 0, len(keys))
			for _, k := range keys {
				pairs = append(pairs, []any{k.AdapterID, k.ProbeWindowStart})
			}
			result := tx.Unscoped().Where("(adapter_id,probe_window_start) IN ?", pairs).Delete(&m.HealthProbeResult{})
			total = result.RowsAffected
			return result.Error
		}
		table := quotedRelation(m.DiscoveryCacheEntry{}.TableName())
		// 当前缓存即使过期也可用于陈旧回退，只有明确软删除且宽限期已过的条目可物理清理。
		q := tx.Unscoped().Model(&m.DiscoveryCacheEntry{}).Table(table+" AS x").Where("x.deleted_at IS NOT NULL AND x.deleted_at < ? AND x.expires_at < ? AND x.issued_at < ? AND x.updated_at < ?", cutoff, cutoff, cutoff, cutoff).
			Where("EXISTS (SELECT 1 FROM " + quotedRelation((&tenants.Tenant{}).TableName()) + " AS rt WHERE CAST(rt.uuid AS TEXT)=x.tenant_uuid AND rt.deleted_at IS NULL)")
		if apply {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED", Table: clause.Table{Name: "x"}})
		}
		var keys []m.DiscoveryCacheEntry
		if e := q.Select("x.tenant_uuid,x.capability_id,x.client_id").Order("x.expires_at,x.capability_id,x.client_id").Limit(limit).Find(&keys).Error; e != nil {
			return e
		}
		total = int64(len(keys))
		if !apply || len(keys) == 0 {
			return nil
		}
		tuples := make([][]any, 0, len(keys))
		for _, k := range keys {
			tuples = append(tuples, []any{k.TenantUUID, k.CapabilityID, k.ClientID})
		}
		result := tx.Unscoped().Where("(tenant_uuid,capability_id,client_id) IN ?", tuples).Delete(&m.DiscoveryCacheEntry{})
		total = result.RowsAffected
		return result.Error
	}, &sql.TxOptions{ReadOnly: !apply})
	return total, e
}
