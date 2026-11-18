package storagegovrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	registry "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sys/unix"
)

const (
	// IncrementalMaintenanceMode moves tuples within one fixed system table.
	IncrementalMaintenanceMode = "incremental"
	// IncrementalMaintenanceTable is the only relation eligible for this mode.
	IncrementalMaintenanceTable = "capability-event-publications"
	// IncrementalMaintenanceWorkingBytes leaves space for small writes, WAL and index maintenance.
	IncrementalMaintenanceWorkingBytes = 192 << 20
	incrementalBlockBytes              = 8192
	incrementalPages                   = 1024
	incrementalPagesPerTransaction     = 32
	incrementalRowsPerTransaction      = 500
	incrementalVersionsPerTransaction  = 1200
)

var (
	// These errors report an intentional stop with a known outcome.
	ErrIncrementalUnsupported = errors.New("database_maintenance_incremental_unsupported")
	ErrIncrementalFSM         = errors.New("database_maintenance_incremental_fsm_required")
	ErrIncrementalCapacity    = errors.New("database_maintenance_incremental_no_headroom")
	ErrIncrementalProtected   = errors.New("database_maintenance_incremental_protected_tail")
	ErrIncrementalLimit       = errors.New("database_maintenance_incremental_limit")
	ErrIncrementalNoReclaim   = errors.New("database_maintenance_incremental_no_reclaim")
)

// ValidMaintenanceRequest checks mode and relation IDs, including recovered runs.
func ValidMaintenanceRequest(mode string, ids []string) bool {
	switch mode {
	case "reuse", "compact":
		return ValidMaintenanceTables(ids)
	case IncrementalMaintenanceMode:
		return len(ids) == 1 && ids[0] == IncrementalMaintenanceTable
	default:
		return false
	}
}

// IncrementalMaintenanceError identifies bounded, deliberate stops that need no session reconciliation.
func IncrementalMaintenanceError(e error) bool {
	return errors.Is(e, ErrIncrementalUnsupported) || errors.Is(e, ErrIncrementalFSM) ||
		errors.Is(e, ErrIncrementalCapacity) || errors.Is(e, ErrIncrementalProtected) ||
		errors.Is(e, ErrIncrementalLimit) || errors.Is(e, ErrIncrementalNoReclaim)
}

type incrementalQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Lock the relation before rechecking this catalog contract in every write transaction.
// Exclude dependencies that could execute SQL or write to another relation during UPDATE.
func incrementalTableSafe(ctx context.Context, c incrementalQueryer, name string, oid int64) (bool, error) {
	var safe bool
	e := c.QueryRowContext(ctx, `
SELECT EXISTS(
 SELECT FROM pg_catalog.pg_class c JOIN pg_catalog.pg_am am ON am.oid=c.relam
 WHERE c.oid=$1 AND c.oid=pg_catalog.to_regclass($2)
 AND c.relkind='r' AND c.relpersistence='p' AND am.amname='heap' AND c.reltablespace IN(0,1663)
 AND NOT c.relrowsecurity AND NOT c.relforcerowsecurity AND NOT c.relhasrules
 AND NOT EXISTS(SELECT FROM pg_catalog.pg_inherits WHERE c.oid IN(inhparent,inhrelid))
 AND NOT EXISTS(SELECT FROM pg_catalog.pg_trigger WHERE tgrelid=c.oid)
 AND NOT EXISTS(SELECT FROM pg_catalog.pg_constraint WHERE c.oid IN(conrelid,confrelid) AND contype NOT IN('p','u'))
 AND NOT EXISTS(SELECT FROM pg_catalog.pg_attribute WHERE attrelid=c.oid AND attnum>0 AND NOT attisdropped
  AND (attgenerated<>'' OR atttypid NOT IN ('pg_catalog.int4'::regtype,'pg_catalog.int8'::regtype,'pg_catalog.uuid'::regtype,
     'pg_catalog.varchar'::regtype,'pg_catalog.bpchar'::regtype,'pg_catalog.text'::regtype,'pg_catalog.timestamptz'::regtype,'pg_catalog.jsonb'::regtype)))
 AND (SELECT count(*) FROM pg_catalog.pg_attribute WHERE attrelid=c.oid AND NOT attisdropped
  AND (attname IN('record_class','status','topic') AND atttypid='pg_catalog.varchar'::regtype
   OR attname='payload' AND atttypid='pg_catalog.jsonb'::regtype AND attnotnull
   OR attname='terminal_at' AND atttypid='pg_catalog.timestamptz'::regtype
   OR attname='deleted_at' AND atttypid='pg_catalog.timestamptz'::regtype))=6
 AND (SELECT count(*) FROM pg_catalog.pg_index WHERE indrelid=c.oid)<=8
 AND NOT EXISTS(
   SELECT FROM pg_catalog.pg_index i JOIN pg_catalog.pg_class ix ON ix.oid=i.indexrelid
   JOIN pg_catalog.pg_am iam ON iam.oid=ix.relam
   WHERE i.indrelid=c.oid AND (iam.amname<>'btree' OR NOT i.indisvalid OR NOT i.indisready
    OR i.indexprs IS NOT NULL OR i.indpred IS NOT NULL OR ix.reltablespace NOT IN(0,1663)
    OR EXISTS(SELECT FROM pg_catalog.pg_opclass oc WHERE oc.oid=ANY(i.indclass)
             AND oc.opcnamespace<>'pg_catalog'::regnamespace)))
)`, oid, name).Scan(&safe)
	return safe, e
}

func incrementalClusterSafe(ctx context.Context, c incrementalQueryer) (bool, error) {
	var safe bool
	e := c.QueryRowContext(ctx, `
SELECT current_setting('server_version_num')::int BETWEEN 160000 AND 169999
 AND current_setting('block_size')::int=8192
 AND pg_size_bytes(current_setting('wal_segment_size'))<=16777216
 AND current_setting('archive_mode')='off'
 AND pg_has_role(current_user,'pg_read_all_stats','USAGE')
 AND NOT EXISTS(SELECT FROM pg_catalog.pg_replication_slots)
 AND NOT EXISTS(SELECT FROM pg_catalog.pg_prepared_xacts)
 AND NOT EXISTS(SELECT FROM pg_catalog.pg_stat_activity WHERE pid<>pg_backend_pid() AND xact_start<now()-interval '30 seconds')
`).Scan(&safe)
	return safe, e
}

func (r *DatabaseMaintenanceRepository) incrementalDiskSafe() bool {
	var data, wal, disk unix.Stat_t
	walPath := filepath.Join(r.dataDirectory, "pg_wal")
	walInfo, e := os.Lstat(walPath)
	return e == nil && walInfo.IsDir() && unix.Stat(r.dataDirectory, &data) == nil &&
		unix.Stat(walPath, &wal) == nil && unix.Stat(r.filesystem, &disk) == nil &&
		data.Ino == r.directoryInode && uint64(data.Dev) == r.directoryDevice && data.Dev == wal.Dev && data.Dev == disk.Dev
}

func incrementalFSM(ctx context.Context, c incrementalQueryer) (string, error) {
	var schema string
	e := c.QueryRowContext(ctx, `
SELECT n.nspname
FROM pg_catalog.pg_extension e JOIN pg_catalog.pg_depend d ON d.refobjid=e.oid
JOIN pg_catalog.pg_proc p ON d.objid=p.oid
JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace
WHERE e.extname='pg_freespacemap' AND d.deptype='e' AND d.refclassid='pg_extension'::regclass
 AND d.classid='pg_proc'::regclass AND p.proname='pg_freespace'
 AND p.proargtypes='2205 20'::oidvector AND p.probin='$libdir/pg_freespacemap'
 AND p.prosrc='pg_freespace' AND pg_catalog.has_function_privilege(p.oid,'EXECUTE')
 AND pg_catalog.has_schema_privilege(n.oid,'USAGE')
`).Scan(&schema)
	if errors.Is(e, sql.ErrNoRows) {
		return "", ErrIncrementalFSM
	}
	return pgx.Identifier{schema, "pg_freespace"}.Sanitize(), e
}

func (r *DatabaseMaintenanceRepository) incrementalReady(ctx context.Context, c *sql.Conn, snap MaintenanceSnapshot) (MaintenanceTable, string, error) {
	if !snap.LocalDiskVerified || !r.incrementalDiskSafe() {
		return MaintenanceTable{}, "", ErrMaintenanceDisk
	}
	clusterSafe, e := incrementalClusterSafe(ctx, c)
	if e != nil {
		return MaintenanceTable{}, "", e
	}
	if !snap.RewriteSafe || !clusterSafe {
		return MaintenanceTable{}, "", ErrIncrementalUnsupported
	}
	for _, t := range snap.Tables {
		if t.ID != IncrementalMaintenanceTable {
			continue
		}
		name := pgx.Identifier{r.schema, maintenanceTables[t.ID]}.Sanitize()
		safe, e := incrementalTableSafe(ctx, c, name, t.OID)
		if e != nil {
			return MaintenanceTable{}, "", e
		}
		if !safe || !t.DefaultTablespace {
			return MaintenanceTable{}, "", ErrIncrementalUnsupported
		}
		fsm, e := incrementalFSM(ctx, c)
		return t, fsm, e
	}
	return MaintenanceTable{}, "", ErrMaintenanceTarget
}

// CheckIncremental validates the current tail and samples earlier FSM pages without changing tuples.
func (r *DatabaseMaintenanceRepository) CheckIncremental(ctx context.Context, snap MaintenanceSnapshot) error {
	pool, e := r.pool()
	if e != nil {
		return e
	}
	c, e := pool.Conn(ctx)
	if e != nil {
		return e
	}
	defer c.Close()
	table, fsm, e := r.incrementalReady(ctx, c, snap)
	if e != nil {
		return e
	}
	name := pgx.Identifier{r.schema, maintenanceTables[table.ID]}.Sanitize()
	end := table.HeapBytes / incrementalBlockBytes
	return incrementalPrecheck(ctx, c, name, fsm, table.OID, incrementalStart(end), end)
}

func incrementalStart(end int64) int64 {
	return max(int64(1), end-incrementalPages)
}

// Watch WAL alongside the service's disk/lease monitor. WAL is cluster-wide,
// so a concurrent write burst also cancels the batch. This is a stop threshold,
// not a hard upper bound on what PostgreSQL or other processes can write.
func incrementalWAL(ctx context.Context, c incrementalQueryer, from string) (int64, error) {
	var bytes int64
	e := c.QueryRowContext(ctx, `SELECT pg_wal_lsn_diff(pg_current_wal_insert_lsn(),$1::pg_lsn)::bigint`, from).Scan(&bytes)
	return bytes, e
}

func (r *DatabaseMaintenanceRepository) executeIncremental(ctx context.Context, c *sql.Conn, snap MaintenanceSnapshot, guard func(context.Context) error) error {
	table, fsm, e := r.incrementalReady(ctx, c, snap)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	if _, e = c.ExecContext(ctx, `SELECT set_config('search_path','pg_catalog,pg_temp',false),
set_config('lock_timeout','500ms',false),set_config('vacuum_cost_delay','5ms',false),
set_config('vacuum_cost_limit','200',false),set_config('statement_timeout','180s',false)`); e != nil {
		return e
	}
	var startLSN string
	if e = c.QueryRowContext(ctx, `SELECT pg_current_wal_insert_lsn()::text`).Scan(&startLSN); e != nil {
		return e
	}
	name := pgx.Identifier{r.schema, maintenanceTables[table.ID]}.Sanitize()
	n := table.HeapBytes / incrementalBlockBytes
	start := incrementalStart(n)
	// PostgreSQL considers spilling to the end if its FSM cannot find room.
	// A bounded sample only establishes that some earlier pages have room.
	// Every actual move is checked independently before it commits.
	if e = incrementalPrecheck(ctx, c, name, fsm, table.OID, start, n); e != nil {
		return e
	}
	check := func(checkCtx context.Context) error {
		if e := guard(checkCtx); e != nil {
			return e
		}
		if !r.incrementalDiskSafe() {
			return ErrMaintenanceDisk
		}
		bytes, e := incrementalWAL(checkCtx, c, startLSN)
		if e != nil {
			return e
		}
		if bytes >= 96<<20 {
			return ErrIncrementalLimit
		}
		safe, e := incrementalClusterSafe(checkCtx, c)
		if e != nil {
			return e
		}
		if !safe {
			return ErrIncrementalUnsupported
		}
		return nil
	}
	for end := n; end > start; {
		if e = check(ctx); e != nil {
			return e
		}
		low := max(start, end-incrementalPagesPerTransaction)
		if e = moveIncrementalWindow(ctx, c, name, table, low, end, startLSN); e != nil {
			return e
		}
		end = low
	}
	if e = check(ctx); e != nil {
		return e
	}
	safe, e := incrementalTableSafe(ctx, c, name, table.OID)
	if e != nil {
		return e
	}
	if !safe {
		return ErrIncrementalUnsupported
	}
	// VACUUM independently verifies the tail and acquires its own short
	// exclusive lock for truncation. Constrain work to this heap and its indexes.
	if e = r.vacuumIncrementalTail(ctx, c, name, startLSN); e != nil {
		return e
	}
	var heap, total int64
	if e = c.QueryRowContext(ctx, `SELECT pg_relation_size(oid),pg_total_relation_size(oid) FROM pg_class
WHERE oid=to_regclass($1) AND oid=$2`, name, table.OID).Scan(&heap, &total); e != nil {
		return e
	}
	if heap >= table.HeapBytes || total >= table.TotalBytes {
		return ErrIncrementalNoReclaim
	}
	return nil
}

func (r *DatabaseMaintenanceRepository) vacuumIncrementalTail(ctx context.Context, c *sql.Conn, name, startLSN string) error {
	pool, e := r.pool()
	if e != nil {
		return e
	}
	work, stop := context.WithCancelCause(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-work.Done():
				return
			case <-tick.C:
				check, cancel := context.WithTimeout(work, time.Second)
				wal, e := incrementalWAL(check, pool, startLSN)
				cancel()
				if work.Err() != nil {
					return
				}
				if e != nil || wal >= 160<<20 {
					stop(ErrIncrementalLimit)
					return
				}
				if !r.incrementalDiskSafe() {
					stop(ErrMaintenanceDisk)
					return
				}
			}
		}
	}()
	_, e = c.ExecContext(work, `VACUUM (TRUNCATE TRUE, INDEX_CLEANUP ON, PROCESS_TOAST FALSE,
SKIP_DATABASE_STATS TRUE, PARALLEL 0, BUFFER_USAGE_LIMIT '2MB') `+name)
	cause := context.Cause(work)
	stop(nil)
	<-done
	if cause != nil {
		return cause
	}
	return e
}

const incrementalEligible = `COALESCE(record_class=$3 AND terminal_at IS NOT NULL AND deleted_at IS NULL
 AND status IN ('published','dispatched','failed','unrouted','quarantined')
 AND topic IN ('_topic.integration.gateway.invocation.succeeded','_topic.integration.gateway.invocation.failed')
 AND pg_column_size(payload)<=8192 AND pg_column_size(entry)<=8192,false)`

const incrementalRange = `ctid >= $1::tid AND ctid < $2::tid`

func incrementalTID(block int64) string {
	return fmt.Sprintf("(%d,0)", block)
}

func incrementalPrecheck(ctx context.Context, c *sql.Conn, name, fsm string, oid, start, end int64) error {
	if end <= start {
		return ErrIncrementalNoReclaim
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var nonempty, protected bool
	e := c.QueryRowContext(ctx, `SELECT count(*)>0, COALESCE(bool_or(NOT (`+incrementalEligible+`)),false)
FROM ONLY `+name+` AS entry WHERE `+incrementalRange,
		incrementalTID(start), incrementalTID(end), registry.InvocationNotificationClass).Scan(&nonempty, &protected)
	if e != nil {
		return e
	}
	if protected {
		return ErrIncrementalProtected
	}
	if !nonempty {
		return nil
	}
	var space bool
	early := min(start, end*7/10)
	step := max(int64(1), early/2048)
	e = c.QueryRowContext(ctx, `SELECT EXISTS(SELECT FROM generate_series(0::bigint,$2::bigint-1,$3::bigint) b
WHERE `+fsm+`($1::oid::regclass,b)>=2048 OFFSET 7)`, oid, early, step).Scan(&space)
	if e != nil {
		return e
	}
	if !space {
		return ErrIncrementalCapacity
	}
	return nil
}

func moveIncrementalWindow(ctx context.Context, c *sql.Conn, name string, table MaintenanceTable, start, end int64, startLSN string) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	tx, e := c.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `LOCK TABLE ONLY `+name+` IN SHARE UPDATE EXCLUSIVE MODE NOWAIT`); e != nil {
		return e
	}
	safe, e := incrementalTableSafe(ctx, tx, name, table.OID)
	if e != nil {
		return e
	}
	if !safe {
		return ErrIncrementalUnsupported
	}
	var positions string
	var remaining int
	var protected bool
	lo, hi := incrementalTID(start), incrementalTID(end)
	e = tx.QueryRowContext(ctx, `WITH candidates AS MATERIALIZED (
 SELECT ctid AS position,(`+incrementalEligible+`) AS eligible FROM ONLY `+name+` AS entry
 WHERE `+incrementalRange+` LIMIT $4 FOR UPDATE NOWAIT
)
SELECT COALESCE(array_agg(position),'{}')::text,count(*),COALESCE(bool_or(NOT eligible),false)
FROM candidates`, lo, hi, registry.InvocationNotificationClass, incrementalRowsPerTransaction+1).Scan(&positions, &remaining, &protected)
	if e != nil {
		return e
	}
	if protected {
		return ErrIncrementalProtected
	}
	if remaining > incrementalRowsPerTransaction {
		return ErrIncrementalLimit
	}
	written := 0
	for attempt := 0; remaining > 0 && attempt < 16; attempt++ {
		written += remaining
		if written > incrementalVersionsPerTransaction {
			return ErrIncrementalLimit
		}
		var moved, pastEnd int
		e = tx.QueryRowContext(ctx, `
WITH moved AS (
 UPDATE ONLY `+name+` SET payload=payload WHERE ctid=ANY($1::tid[]) RETURNING ctid
)
SELECT COALESCE(array_agg(ctid) FILTER (WHERE ctid >= $2::tid),'{}')::text,
 count(*),count(*) FILTER (WHERE ctid >= $3::tid)
FROM moved`, positions, lo, hi).Scan(&positions, &moved, &pastEnd)
		if e != nil {
			return e
		}
		if pastEnd != 0 || moved != remaining {
			return ErrIncrementalCapacity
		}
		if e = tx.QueryRowContext(ctx, `SELECT cardinality($1::tid[])`, positions).Scan(&remaining); e != nil {
			return e
		}
		wal, e := incrementalWAL(ctx, tx, startLSN)
		if e != nil {
			return e
		}
		if wal >= 96<<20 {
			return ErrIncrementalLimit
		}
	}
	var occupied bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT FROM ONLY `+name+` WHERE `+incrementalRange+`)
 OR pg_relation_size($3::oid)>$4 OR pg_total_relation_size($3::oid)>$5`,
		lo, hi, table.OID, table.HeapBytes, table.TotalBytes+(32<<20)).Scan(&occupied)
	if e != nil {
		return e
	}
	if remaining > 0 || occupied {
		return ErrIncrementalCapacity
	}
	return tx.Commit()
}
