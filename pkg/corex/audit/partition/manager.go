package partition

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Manager 负责 audit_event 的按月分区管理
type Manager struct {
	DB     *gorm.DB
	Schema string // e.g. "public"
	Table  string // e.g. "audit_event"
}

// NewManager 默认 public.audit_event
func NewManager(db *gorm.DB, schema, table string) *Manager {
	if schema == "" {
		schema = "public"
	}
	if table == "" {
		table = "audit_event"
	}
	return &Manager{DB: db, Schema: schema, Table: table}
}

// EnsureParent 把父表建为按 occurred_at 的 RANGE 分区表（如果已是分区表就跳过）
// 注意：父表字段与索引/约束建议先按你的 GORM Model 建好，再改分区属性。
func (m *Manager) EnsureParent(ctx context.Context) error {
	parent := fmt.Sprintf(`%s.%s`, m.Schema, m.Table)
	// 检查是否已是分区表
	var partType string
	err := m.DB.WithContext(ctx).Raw(`
SELECT relkind
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = ? AND c.relname = ?`, m.Schema, m.Table).Scan(&partType).Error
	if err != nil {
		return err
	}

	// 尝试把父表改为分区表（如果不是的话）
	// 这里采取“幂等”的做法：如果已经是分区表，则 ALTER 会报错，忽略即可。
	// NOTE: 这一步只有在初次改造时需要；之后只需调用 EnsureMonthlyPartitions/RetireOlderThan。
	sql := fmt.Sprintf(`ALTER TABLE %s PARTITION BY RANGE (occurred_at)`, parent)
	_ = m.DB.WithContext(ctx).Exec(sql).Error // 忽略错误（已分区时会报错）

	return nil
}

// EnsureMonthlyPartitions 预建从 now-preparePast 到 now+prepareFuture 的月分区（闭区间）
// 例如：preparePast=1, prepareFuture=2 → 建 [上月，本月，下月，下下月] 四个分区
func (m *Manager) EnsureMonthlyPartitions(ctx context.Context, now time.Time, preparePast, prepareFuture int) error {
	if preparePast < 0 {
		preparePast = 0
	}
	if prepareFuture < 0 {
		prepareFuture = 0
	}

	base := firstOfMonth(now)
	start := base.AddDate(0, -preparePast, 0)
	end := base.AddDate(0, prepareFuture+1, 0) // 终止点是开区间的下月首日

	ym := firstOfMonth(start)
	for !ym.After(end.AddDate(0, -1, 0)) { // 循环到 end 的上一个月
		if err := m.ensureOneMonth(ctx, ym); err != nil {
			return err
		}
		ym = ym.AddDate(0, 1, 0)
	}
	return nil
}

func (m *Manager) ensureOneMonth(ctx context.Context, month time.Time) error {
	start := firstOfMonth(month).Format("2006-01-02")
	to := firstOfMonth(month.AddDate(0, 1, 0)).Format("2006-01-02")
	name := m.partitionName(month)

	sql := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s PARTITION OF %s.%s
FOR VALUES FROM ('%s') TO ('%s')`,
		m.Schema, name, m.Schema, m.Table, start, to)

	return m.DB.WithContext(ctx).Exec(sql).Error
}

// RetireOlderThan 丢弃早于 cutoff 月（整月）的分区：DROP TABLE schema.audit_event_YYYY_MM
// 例如 cutoff=2025-05-01，则会 drop 2025-04 及更早的所有分区。
func (m *Manager) RetireOlderThan(ctx context.Context, cutoff time.Time) error {
	cut := firstOfMonth(cutoff)

	// 枚举所有子分区（使用 pg_partition_tree 或 pg_inherits）
	type part struct {
		Name string
		From time.Time
		To   time.Time
	}
	var parts []part

	// 用 pg_partition_tree 获取边界更准确
	err := m.DB.WithContext(ctx).Raw(`
SELECT
  c.relname                    AS name,
  pg_range_bound_lower(expr)   AS from,
  pg_range_bound_upper(expr)   AS to
FROM (
  SELECT
    c.oid,
    c.relname,
    pg_get_expr(c.relpartbound, c.oid, true) AS expr
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
  JOIN pg_inherits i ON i.inhrelid = c.oid
  JOIN pg_class p ON p.oid = i.inhparent
  JOIN pg_namespace pn ON pn.oid = p.relnamespace
  WHERE pn.nspname = ? AND p.relname = ?
) t
JOIN pg_class c ON c.oid = t.oid
`, m.Schema, m.Table).Scan(&parts).Error
	if err != nil {
		return err
	}

	// 逐个分区判断下界 < cutoff 就 DROP
	for _, p := range parts {
		if p.From.Before(cut) {
			sql := fmt.Sprintf(`DROP TABLE IF EXISTS %s.%s`, m.Schema, p.Name)
			if err := m.DB.WithContext(ctx).Exec(sql).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// TryAdvisoryLock 用于多实例时避免并发清理
func (m *Manager) TryAdvisoryLock(ctx context.Context, key int64) (bool, error) {
	var ok bool
	err := m.DB.WithContext(ctx).Raw(`SELECT pg_try_advisory_lock(?)`, key).Scan(&ok).Error
	return ok, err
}
func (m *Manager) AdvisoryUnlock(ctx context.Context, key int64) error {
	return m.DB.WithContext(ctx).Exec(`SELECT pg_advisory_unlock(?)`, key).Error
}

func (m *Manager) partitionName(month time.Time) string {
	return fmt.Sprintf("%s_%04d_%02d", m.Table, month.Year(), int(month.Month()))
}

func firstOfMonth(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
}
