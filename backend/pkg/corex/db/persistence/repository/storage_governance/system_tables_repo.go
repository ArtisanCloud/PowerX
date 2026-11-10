package storagegovrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"gorm.io/gorm"
)

// SystemTableObservation 只返回容量和时间边界，不读取或暴露记录载荷。
type SystemTableObservation struct {
	m.SystemTableDefinition
	Schema           string     `json:"schema"`
	DatabaseIdentity string     `json:"-"`
	RelationID       int64      `json:"-"`
	State            string     `json:"state"`
	HeapBytes        *int64     `json:"heap_bytes"`
	IndexesBytes     *int64     `json:"indexes_bytes"`
	ToastBytes       *int64     `json:"toast_bytes"`
	TotalBytes       *int64     `json:"total_bytes"`
	EstimatedRows    *int64     `json:"estimated_rows"`
	TimeState        string     `json:"time_state"`
	OldestAt         *time.Time `json:"oldest_at"`
	NewestAt         *time.Time `json:"newest_at"`
	Problem          string     `json:"problem,omitempty"`
	TimeProblem      string     `json:"time_problem,omitempty"`
}

type SystemTablesRepository struct{ db *gorm.DB }

func NewSystemTablesRepository(db *gorm.DB) *SystemTablesRepository {
	return &SystemTablesRepository{db: db}
}

// Observe 限定当前部署的 schema 和固定目录；范围查询独立限时，失败不抹掉已取得的容量。
// 物理大小和时间范围均包含尚未物理清除的软删除记录，避免与磁盘计量混用不同范围。
func (r *SystemTablesRepository) Observe(ctx context.Context, schema, onlyID string, rangeTimeout time.Duration) ([]SystemTableObservation, error) {
	definitions := m.SystemTableCatalog()
	if onlyID != "" {
		definitions = nil
		for _, d := range m.SystemTableCatalog() {
			if d.Name == onlyID {
				definitions = append(definitions, d)
				break
			}
		}
		if len(definitions) == 0 {
			return nil, gorm.ErrRecordNotFound
		}
	}
	out := make([]SystemTableObservation, 0, len(definitions))
	for _, d := range definitions {
		out = append(out, SystemTableObservation{SystemTableDefinition: d, Schema: schema, State: "unavailable", TimeState: "unavailable"})
	}
	if r.db == nil || r.db.Dialector.Name() != "postgres" {
		for i := range out {
			out[i].Problem = "postgres_metrics_unavailable"
		}
		return out, nil
	}
	metrics := NewPostgresMetricsRepository(r.db)
	var databaseIdentity string
	if err := r.db.WithContext(ctx).Raw(`SELECT current_database() || ':' || oid::text FROM pg_catalog.pg_database WHERE datname=current_database()`).Scan(&databaseIdentity).Error; err != nil || databaseIdentity == "" {
		return nil, errors.New("database_identity_unavailable")
	}
	for i := range out {
		row := &out[i]
		row.DatabaseIdentity = databaseIdentity
		if ctx.Err() != nil {
			row.Problem = "relation_metrics_unreadable"
			continue
		}
		var meta struct {
			RelationID    int64
			Kind          string
			HasTime       bool
			CanRead       bool
			EstimatedRows int64
		}
		q := r.db.WithContext(ctx).Raw(`SELECT c.oid::bigint AS relation_id, c.relkind::text AS kind, c.reltuples::bigint AS estimated_rows,
			pg_catalog.has_table_privilege(c.oid, 'SELECT') AS can_read,
			EXISTS (SELECT 1 FROM pg_catalog.pg_attribute a WHERE a.attrelid=c.oid AND a.attname=? AND a.attnum>0 AND NOT a.attisdropped AND a.atttypid IN ('timestamp'::regtype,'timestamptz'::regtype,'date'::regtype)) AS has_time
			FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=? AND c.relname=?`, row.TimeColumn, schema, row.Name).Scan(&meta)
		row.RelationID = meta.RelationID
		switch {
		case q.Error != nil:
			row.Problem = "relation_metrics_unreadable"
		case q.RowsAffected == 0:
			row.State = "not_present"
			row.Problem = "table_not_present"
		case meta.Kind != "r" && meta.Kind != "m":
			row.Problem = "relation_kind_unsupported"
		case !meta.CanRead:
			row.Problem = "relation_permission_denied"
		default:
			u, err := metrics.Relation(ctx, schema, row.Name)
			if err != nil {
				row.Problem = "relation_metrics_unreadable"
				continue
			}
			if u.TotalBytes > 1<<53-1 {
				row.Problem = "relation_metrics_out_of_range"
				continue
			}
			row.State = "observed"
			row.HeapBytes, row.IndexesBytes, row.ToastBytes, row.TotalBytes = &u.HeapBytes, &u.IndexesBytes, &u.ToastBytes, &u.TotalBytes
			if meta.EstimatedRows >= 0 {
				n := meta.EstimatedRows
				row.EstimatedRows = &n
			}
			if !meta.HasTime {
				row.TimeProblem = "time_column_unavailable"
			}
		}
	}
	// 后台五分钟采样只读取容量元数据，不反复扫描时间字段。
	if rangeTimeout == 0 {
		return out, nil
	}
	// 先完成所有表的轻量容量观测，再推进时间边界；大表超时不使后面的表失去容量。
	for i := range out {
		row := &out[i]
		if row.State != "observed" || row.TimeProblem != "" {
			continue
		}
		if ctx.Err() != nil {
			row.TimeProblem = "time_range_timeout"
			continue
		}
		oldest, newest, hasRows, err := r.timeBounds(ctx, schema, row.Name, row.TimeColumn, rangeTimeout)
		if err != nil {
			row.TimeProblem = "time_range_unreadable_or_timeout"
			continue
		}
		row.OldestAt, row.NewestAt = oldest, newest
		switch {
		case !hasRows:
			row.TimeState = "empty"
		case oldest == nil || newest == nil:
			row.TimeState = "no_timestamps"
		default:
			row.TimeState = "observed"
		}
	}
	return out, nil
}

func (r *SystemTablesRepository) timeBounds(ctx context.Context, schema, table, column string, timeout time.Duration) (*time.Time, *time.Time, bool, error) {
	// 所有动态标识符均来自服务端目录，仍按 PostgreSQL 标识符规则引用。
	quote := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
	target, timestamp := quote(schema)+"."+quote(table), quote(column)
	if timeout <= 0 || timeout > 3*time.Second {
		return nil, nil, false, errors.New("invalid_range_timeout")
	}
	var row struct {
		OldestAt *time.Time
		NewestAt *time.Time
		HasRows  bool
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(fmt.Sprintf("SET LOCAL statement_timeout='%dms'", timeout.Milliseconds())).Error; err != nil {
			return err
		}
		if err := tx.Exec("SET LOCAL lock_timeout='50ms'").Error; err != nil {
			return err
		}
		query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s LIMIT 1) AS has_rows,
			(SELECT %s FROM %s WHERE %s IS NOT NULL ORDER BY %s ASC LIMIT 1) AS oldest_at,
			(SELECT %s FROM %s WHERE %s IS NOT NULL ORDER BY %s DESC LIMIT 1) AS newest_at`, target, timestamp, target, timestamp, timestamp, timestamp, target, timestamp, timestamp)
		return tx.Raw(query).Scan(&row).Error
	}, &sql.TxOptions{ReadOnly: true})
	return row.OldestAt, row.NewestAt, row.HasRows, err
}
