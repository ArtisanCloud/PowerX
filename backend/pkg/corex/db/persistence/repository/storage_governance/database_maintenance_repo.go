package storagegovrepo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sys/unix"
	"gorm.io/gorm"
)

var (
	ErrMaintenanceUnavailable = errors.New("database_maintenance_unavailable")
	ErrMaintenanceTarget      = errors.New("database_maintenance_target_changed")
	ErrMaintenanceDisk        = errors.New("database_maintenance_disk_unverified")
	ErrMaintenanceIndexes     = errors.New("database_maintenance_indexes_invalid")
	ErrMaintenanceBusy        = errors.New("database_maintenance_busy")
)

// MaintenanceTableID 只允许正式固定系统表；调用方不提交 SQL 标识符。
var maintenanceTables = map[string]string{
	"capability-invocation-traces":  "capability_registry_invocation_traces",
	"capability-event-publications": "capability_registry_event_publications",
}

type MaintenanceTable struct {
	ID                string `json:"id"`
	OID               int64  `json:"oid"`
	HeapBytes         int64  `json:"heap_bytes"`
	IndexBytes        int64  `json:"index_bytes"`
	TotalBytes        int64  `json:"total_bytes"`
	LiveRowsEstimate  int64  `json:"live_rows_estimate"`
	DeadRowsEstimate  int64  `json:"dead_rows_estimate"`
	CanMaintain       bool   `json:"can_maintain"`
	IndexesValid      bool   `json:"indexes_valid"`
	DefaultTablespace bool   `json:"default_tablespace"`
}
type MaintenanceSnapshot struct {
	DatabaseOID       int64              `json:"database_oid"`
	Fingerprint       string             `json:"database_fingerprint"`
	DatabaseBytes     int64              `json:"database_bytes"`
	Tables            []MaintenanceTable `json:"tables"`
	LocalDiskVerified bool               `json:"local_disk_verified"`
	WALBudgetBytes    int64              `json:"wal_budget_bytes"`
	RewriteSafe       bool               `json:"rewrite_safe"`
}
type MaintenanceSession struct {
	PID         int       `json:"pid"`
	StartedAt   time.Time `json:"started_at"`
	Application string    `json:"application"`
}

// DatabaseMaintenanceRepository 使用当前 Core 的连接池，不创建新连接配置或调用 shell。
type DatabaseMaintenanceRepository struct {
	db              *gorm.DB
	schema          string
	dataDirectory   string
	filesystem      string
	directoryDevice uint64
	directoryInode  uint64
}

func NewDatabaseMaintenanceRepository(db *gorm.DB, directory, filesystem string) *DatabaseMaintenanceRepository {
	if directory != "" {
		if path, e := filepath.EvalSymlinks(directory); e == nil {
			directory = path
		}
	}
	r := &DatabaseMaintenanceRepository{db: db, schema: coremodel.PowerXSchema, dataDirectory: directory, filesystem: filesystem}
	var st unix.Stat_t
	if directory != "" && unix.Stat(directory, &st) == nil {
		r.directoryDevice = uint64(st.Dev)
		r.directoryInode = st.Ino
	}
	return r
}
func ValidMaintenanceTables(ids []string) bool {
	if len(ids) < 1 || len(ids) > len(maintenanceTables) {
		return false
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if maintenanceTables[id] == "" || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func (r *DatabaseMaintenanceRepository) pool() (*sql.DB, error) {
	if r.db == nil || r.db.Dialector.Name() != "postgres" {
		return nil, ErrMaintenanceUnavailable
	}
	return r.db.DB()
}
func (r *DatabaseMaintenanceRepository) Inspect(ctx context.Context) (MaintenanceSnapshot, error) {
	p, e := r.pool()
	if e != nil {
		return MaintenanceSnapshot{}, e
	}
	c, e := p.Conn(ctx)
	if e != nil {
		return MaintenanceSnapshot{}, e
	}
	defer c.Close()
	return r.inspect(ctx, c)
}
func (r *DatabaseMaintenanceRepository) inspect(ctx context.Context, c *sql.Conn) (MaintenanceSnapshot, error) {
	out := MaintenanceSnapshot{Tables: []MaintenanceTable{}}
	var database, version, started, address string
	var oid int64
	var defaultSpace bool
	if e := c.QueryRowContext(ctx, `SELECT current_database(),d.oid::bigint,current_setting('server_version_num'),extract(epoch FROM pg_postmaster_start_time())::text,COALESCE(inet_server_addr()::text,''),pg_database_size(d.oid),d.dattablespace=1663 FROM pg_database d WHERE d.datname=current_database()`).Scan(&database, &oid, &version, &started, &address, &out.DatabaseBytes, &defaultSpace); e != nil {
		return out, e
	}
	out.DatabaseOID = oid
	if e := c.QueryRowContext(ctx, `SELECT pg_size_bytes(current_setting('max_wal_size')),current_setting('archive_mode')='off' AND NOT EXISTS(SELECT 1 FROM pg_replication_slots) AND NOT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid<>pg_backend_pid() AND xact_start<now()-interval '30 seconds')`).Scan(&out.WALBudgetBytes, &out.RewriteSafe); e != nil {
		return out, e
	}
	identity := fmt.Sprintf("%s\x00%d\x00%s\x00%s\x00%s\x00%s", database, oid, version, started, r.schema, r.dataDirectory)
	sum := sha256.Sum256([]byte(identity))
	out.Fingerprint = hex.EncodeToString(sum[:])
	ids := []string{"capability-event-publications", "capability-invocation-traces"}
	for _, id := range ids {
		var t MaintenanceTable
		t.ID = id
		name := pgx.Identifier{r.schema, maintenanceTables[id]}.Sanitize()
		q := `SELECT c.oid::bigint,pg_relation_size(c.oid),pg_indexes_size(c.oid),pg_total_relation_size(c.oid),COALESCE(s.n_live_tup,0),COALESCE(s.n_dead_tup,0),
 (SELECT rolsuper FROM pg_roles WHERE rolname=current_user) OR pg_has_role(current_user,c.relowner,'USAGE'),
 NOT EXISTS(SELECT 1 FROM pg_index i WHERE i.indrelid=c.oid AND (NOT i.indisvalid OR NOT i.indisready)),
 c.reltablespace IN (0,1663) AND NOT EXISTS(SELECT 1 FROM pg_index i JOIN pg_class x ON x.oid=i.indexrelid WHERE i.indrelid IN(c.oid,c.reltoastrelid) AND x.reltablespace NOT IN(0,1663)) AND NOT EXISTS(SELECT 1 FROM pg_class x WHERE x.oid=c.reltoastrelid AND x.reltablespace NOT IN(0,1663))
 FROM pg_class c LEFT JOIN pg_stat_all_tables s ON s.relid=c.oid WHERE c.oid=to_regclass($1) AND c.relkind='r' AND c.relpersistence='p' AND NOT EXISTS(SELECT 1 FROM pg_inherits WHERE inhparent=c.oid OR inhrelid=c.oid)`
		if e := c.QueryRowContext(ctx, q, name).Scan(&t.OID, &t.HeapBytes, &t.IndexBytes, &t.TotalBytes, &t.LiveRowsEstimate, &t.DeadRowsEstimate, &t.CanMaintain, &t.IndexesValid, &t.DefaultTablespace); e != nil {
			return out, e
		}
		out.Tables = append(out.Tables, t)
	}
	var serverDirectory string
	if defaultSpace && (address == "" || net.ParseIP(address).IsLoopback()) && r.dataDirectory != "" && filepath.IsAbs(r.dataDirectory) && c.QueryRowContext(ctx, `SELECT current_setting('data_directory')`).Scan(&serverDirectory) == nil {
		// 部署登记只绑定现存真实目录；不遍历 PostgreSQL 私有文件，也不跟随后替换的软链。
		info, e := os.Lstat(r.dataDirectory)
		var data, root unix.Stat_t
		canonical, resolveError := filepath.EvalSymlinks(serverDirectory)
		out.LocalDiskVerified = e == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 && resolveError == nil && canonical == r.dataDirectory && unix.Stat(r.dataDirectory, &data) == nil && unix.Stat(r.filesystem, &root) == nil && data.Dev == root.Dev && uint64(data.Dev) == r.directoryDevice && data.Ino == r.directoryInode
	}
	return out, ctx.Err()
}
func SameMaintenanceTarget(expected, actual MaintenanceSnapshot, ids []string) bool {
	if expected.Fingerprint == "" || expected.Fingerprint != actual.Fingerprint || !ValidMaintenanceTables(ids) {
		return false
	}
	for _, id := range ids {
		var a, b *MaintenanceTable
		for i := range expected.Tables {
			if expected.Tables[i].ID == id {
				a = &expected.Tables[i]
			}
		}
		for i := range actual.Tables {
			if actual.Tables[i].ID == id {
				b = &actual.Tables[i]
			}
		}
		if a == nil || b == nil || a.OID != b.OID || !b.CanMaintain || !b.IndexesValid {
			return false
		}
	}
	return true
}

// Execute 在单一固定连接上执行自动提交的维护语句；不把 VACUUM 放入事务块。
func (r *DatabaseMaintenanceRepository) Execute(ctx context.Context, runID, mode string, expected MaintenanceSnapshot, ids []string, before func(string, MaintenanceSession) error, after func(string) error) error {
	if !ValidMaintenanceTables(ids) || mode != "reuse" && mode != "compact" {
		return ErrMaintenanceTarget
	}
	p, e := r.pool()
	if e != nil {
		return e
	}
	c, e := p.Conn(ctx)
	if e != nil {
		return e
	}
	defer c.Close()
	actual, e := r.inspect(ctx, c)
	if e != nil {
		return e
	}
	if !SameMaintenanceTarget(expected, actual, ids) {
		return ErrMaintenanceTarget
	}
	if mode == "compact" && (!actual.LocalDiskVerified || !actual.RewriteSafe) {
		return ErrMaintenanceDisk
	}
	for _, t := range actual.Tables {
		for _, id := range ids {
			if t.ID == id && mode == "compact" && !t.DefaultTablespace {
				return ErrMaintenanceDisk
			}
		}
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("powerx.database.maintenance/v1\x00%d", actual.DatabaseOID)))
	key := int64(binary.BigEndian.Uint64(sum[:8]))
	var locked bool
	if e = c.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&locked); e != nil {
		return e
	}
	if !locked {
		return ErrMaintenanceBusy
	}
	// 所有会话级设置必须恢复；恢复失败时强制丢弃物理连接，不能污染业务池。
	settings := []string{"application_name", "statement_timeout", "lock_timeout", "maintenance_work_mem", "vacuum_cost_delay", "vacuum_cost_limit"}
	old := map[string]string{}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		bad := false
		for _, name := range settings {
			if value, ok := old[name]; ok {
				if _, err := c.ExecContext(clean, `SELECT set_config($1,$2,false)`, name, value); err != nil {
					bad = true
				}
			}
		}
		if _, err := c.ExecContext(clean, `SELECT pg_advisory_unlock($1)`, key); err != nil {
			bad = true
		}
		if bad {
			_ = c.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	for _, name := range settings {
		var value string
		if e = c.QueryRowContext(ctx, `SELECT current_setting($1)`, name).Scan(&value); e != nil {
			return e
		}
		old[name] = value
	}
	app := "px.dbmaint." + runID
	timeout := "60s"
	if mode == "compact" {
		timeout = "900s"
	}
	for name, value := range map[string]string{"application_name": app, "statement_timeout": timeout, "lock_timeout": "2s", "maintenance_work_mem": "128MB", "vacuum_cost_delay": "2ms", "vacuum_cost_limit": "500"} {
		if _, e = c.ExecContext(ctx, `SELECT set_config($1,$2,false)`, name, value); e != nil {
			return e
		}
	}
	var session MaintenanceSession
	session.Application = app
	if e = c.QueryRowContext(ctx, `SELECT pid,backend_start FROM pg_stat_activity WHERE pid=pg_backend_pid()`).Scan(&session.PID, &session.StartedAt); e != nil {
		return e
	}
	ordered := append([]string(nil), ids...)
	size := map[string]int64{}
	for _, t := range actual.Tables {
		size[t.ID] = t.TotalBytes
	}
	sort.Slice(ordered, func(i, j int) bool { return size[ordered[i]] < size[ordered[j]] })
	for _, id := range ordered {
		if e = before(id, session); e != nil {
			return e
		}
		command := "VACUUM (ANALYZE, TRUNCATE FALSE, PARALLEL 0) "
		if mode == "compact" {
			command = "VACUUM (FULL, ANALYZE) "
		}
		if _, e = c.ExecContext(ctx, command+pgx.Identifier{r.schema, maintenanceTables[id]}.Sanitize()); e != nil {
			return e
		}
		if e = after(id); e != nil {
			return e
		}
	}
	return nil
}

func (r *DatabaseMaintenanceRepository) SessionActive(ctx context.Context, s MaintenanceSession, cancel bool) (bool, error) {
	if s.PID <= 0 || s.StartedAt.IsZero() || len(s.Application) != len("px.dbmaint.")+36 {
		return false, ErrMaintenanceTarget
	}
	p, e := r.pool()
	if e != nil {
		return false, e
	}
	value := "true"
	if cancel {
		value = "pg_cancel_backend(pid)"
	}
	var active bool
	e = p.QueryRowContext(ctx, `SELECT `+value+` FROM pg_stat_activity WHERE pid=$1 AND backend_start=$2 AND application_name=$3 AND datid=(SELECT oid FROM pg_database WHERE datname=current_database()) AND usename=current_user`, s.PID, s.StartedAt, s.Application).Scan(&active)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	return active, e
}
func (r *DatabaseMaintenanceRepository) RootActive(ctx context.Context, actor uint64) error {
	var count int64
	e := r.db.WithContext(ctx).Table(pgx.Identifier{r.schema, "iam_user"}.Sanitize()).Where("id = ? AND is_root = ? AND status = ? AND deleted_at IS NULL", actor, true, 1).Count(&count).Error
	if e != nil {
		return e
	}
	if actor == 0 || count != 1 {
		return ErrMaintenanceUnavailable
	}
	return nil
}
