package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const traces = "public.capability_registry_invocation_traces"
const events = "public.capability_registry_event_publications"

type pair struct{ trace, event int64 }
type archive struct {
	File   string `json:"file"`
	Rows   int64  `json:"rows"`
	Bytes  int64  `json:"compressed_bytes"`
	SHA    string `json:"sha256"`
	RawSHA string `json:"binary_copy_sha256"`
}
type batchReceipt struct {
	State           string            `json:"state"`
	At              time.Time         `json:"at"`
	Cutoff          time.Time         `json:"cutoff"`
	Rows            int               `json:"rows"`
	FirstID         int64             `json:"first_trace_id"`
	LastID          int64             `json:"last_trace_id"`
	Archives        []archive         `json:"archives"`
	Schema          map[string]string `json:"schema_signatures"`
	RestoreVerified bool              `json:"restore_verified"`
}
type progress struct {
	State          string    `json:"state"`
	At             time.Time `json:"at"`
	Mode           string    `json:"mode"`
	Cutoff         time.Time `json:"cutoff"`
	Highwater      int64     `json:"highwater"`
	Cursor         int64     `json:"cursor"`
	Pairs          int64     `json:"pairs"`
	Batches        int       `json:"batches"`
	BackupBytes    int64     `json:"backup_bytes"`
	AvailableBytes uint64    `json:"available_bytes"`
	Problem        string    `json:"problem,omitempty"`
}
type runner struct {
	conn               *pgx.Conn
	out                string
	cutoff             time.Time
	batch              int
	span               int64
	minFree            uint64
	maxBackup          int64
	schema             map[string]string
	p                  progress
	afterArchive       func(string) error
	stopBefore         time.Time
	runtimeGuard       func() error
	devUnroutedReceipt bool
	devScope           string
}

func safeError(err error) string {
	var p *pgconn.PgError
	if errors.As(err, &p) {
		return "postgres_sqlstate_" + p.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "operation_timeout"
	}
	return fmt.Sprintf("operation_failed_%T", err)
}
func atomicJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	f, err := os.OpenFile(path+".next", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	if err = os.Rename(path+".next", path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func freeBytes(path string) (uint64, error) {
	var s syscall.Statfs_t
	err := syscall.Statfs(path, &s)
	return uint64(s.Bavail) * uint64(s.Bsize), err
}

// Keep the host below Loki's 90% WAL limit, with one GiB of headroom,
// and never go below the existing host guard's eight-GiB critical limit.
// Filesystem reserved blocks do not count as space available to this job.
func hostFloor(total, reserved uint64) uint64 {
	floor := uint64(8 << 30)
	tenth := total / 10
	if total%10 != 0 {
		tenth++
	}
	if tenth > reserved {
		if n := tenth - reserved + (1 << 30); n > floor {
			floor = n
		}
	}
	return floor
}

type guardedArchiveWriter struct {
	file      *os.File
	path      string
	floor     uint64
	allowance int64
	written   int64
}

func (w *guardedArchiveWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.allowance-w.written {
		return 0, errors.New("backup_budget_exhausted_before_write")
	}
	if w.floor > 0 {
		a, err := freeBytes(w.path)
		if err != nil {
			return 0, err
		}
		if a < w.floor || uint64(len(p)) > a-w.floor {
			return 0, errors.New("host_guard_floor_before_archive_write")
		}
	}
	n, err := w.file.Write(p)
	w.written += int64(n)
	return n, err
}
func signature(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, table string) (string, error) {
	var s string
	err := q.QueryRow(ctx, `SELECT string_agg(attnum::text||':'||attname||':'||format_type(atttypid,atttypmod)||':'||attnotnull::text,',' ORDER BY attnum) FROM pg_attribute WHERE attrelid=$1::regclass AND attnum>0 AND NOT attisdropped`, table).Scan(&s)
	return s, err
}
func (r *runner) preflight(ctx context.Context) error {
	var n int
	if err := r.conn.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE contype='f' AND (confrelid IN ($1::regclass,$2::regclass) OR conrelid IN ($1::regclass,$2::regclass))`, traces, events).Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return errors.New("external_foreign_keys")
	}
	if err := r.conn.QueryRow(ctx, `SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgrelid IN ($1::regclass,$2::regclass)`, traces, events).Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return errors.New("custom_triggers")
	}
	var valid bool
	if err := r.conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid JOIN pg_namespace ns ON ns.oid=c.relnamespace WHERE ns.nspname='public' AND c.relname='idx_capability_trace_publication' AND i.indisvalid AND i.indisready AND i.indrelid=$1::regclass AND i.indnkeyatts=1 AND i.indkey[0]=(SELECT attnum FROM pg_attribute WHERE attrelid=$1::regclass AND attname='event_publication_id'))`, traces).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return errors.New("reference_index_not_ready")
	}
	r.schema = map[string]string{}
	for _, t := range []string{traces, events} {
		s, err := signature(ctx, r.conn, t)
		if err != nil {
			return err
		}
		r.schema[t] = s
	}
	return nil
}
func (r *runner) state(state string, err error) error {
	r.p.State = state
	r.p.At = time.Now().UTC()
	if err != nil {
		r.p.Problem = safeError(err)
	}
	r.p.AvailableBytes, _ = freeBytes(r.out)
	return atomicJSON(filepath.Join(r.out, "progress.json"), r.p)
}
func candidates(ctx context.Context, tx pgx.Tx, cursor, upper int64, cutoff time.Time, limit int, apply bool, devReceipt bool) ([]pair, error) {
	scope := scopeSQL
	if devReceipt {
		scope = developmentScopeSQL()
	}
	sql := "SELECT t.id,e.id" + scope + " ORDER BY t.id LIMIT $4"
	if apply {
		sql += " FOR UPDATE OF t,e SKIP LOCKED"
	}
	rows, err := tx.Query(ctx, sql, cursor, upper, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pair
	for rows.Next() {
		var p pair
		if err = rows.Scan(&p.trace, &p.event); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func exportCopy(ctx context.Context, tx pgx.Tx, path, query string, want int, floor uint64, allowance int64) (archive, error) {
	a := archive{File: filepath.Base(path)}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return a, err
	}
	h := sha256.New()
	rawHash := sha256.New()
	guard := &guardedArchiveWriter{file: f, path: filepath.Dir(path), floor: floor, allowance: allowance}
	zw, _ := gzip.NewWriterLevel(io.MultiWriter(guard, h), gzip.BestSpeed)
	tag, err := tx.Conn().PgConn().CopyTo(ctx, io.MultiWriter(zw, rawHash), query)
	closeErr := zw.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = f.Sync()
	}
	st, _ := f.Stat()
	fileErr := f.Close()
	if err == nil {
		err = fileErr
	}
	if err != nil {
		return a, err
	}
	a.Rows = tag.RowsAffected()
	a.Bytes = st.Size()
	a.SHA = hex.EncodeToString(h.Sum(nil))
	a.RawSHA = hex.EncodeToString(rawHash.Sum(nil))
	if a.Rows != int64(want) {
		return a, errors.New("archive_row_count_mismatch")
	}
	// Read every compressed byte and validate gzip CRC plus the original COPY stream hash.
	src, err := os.Open(path)
	if err != nil {
		return a, err
	}
	defer src.Close()
	h2 := sha256.New()
	zr, err := gzip.NewReader(io.TeeReader(src, h2))
	if err != nil {
		return a, err
	}
	hr := sha256.New()
	_, err = io.Copy(hr, zr)
	ce := zr.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return a, err
	}
	if _, err = io.Copy(io.Discard, io.TeeReader(src, h2)); err != nil {
		return a, err
	}
	if hex.EncodeToString(h2.Sum(nil)) != a.SHA || hex.EncodeToString(hr.Sum(nil)) != a.RawSHA {
		return a, errors.New("archive_readback_hash_mismatch")
	}
	return a, nil
}
func restoreCopy(ctx context.Context, tx pgx.Tx, path, temp string, want int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	tag, err := tx.Conn().PgConn().CopyFrom(ctx, zr, "COPY pg_temp."+temp+" FROM STDIN WITH (FORMAT binary)")
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(want) {
		return errors.New("restore_row_count_mismatch")
	}
	_, err = io.Copy(io.Discard, zr)
	return err
}
func (r *runner) one(ctx context.Context, cursor, upper int64, apply bool) (count int, last int64, err error) {
	opts := pgx.TxOptions{}
	if !apply {
		opts.AccessMode = pgx.ReadOnly
	}
	tx, err := r.conn.BeginTx(ctx, opts)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if apply && r.devScope != "" {
		if err = guardDevelopmentExecution(ctx, tx, r.devScope); err != nil {
			return 0, 0, err
		}
	}
	if apply {
		// 与普通写入兼容，同时固定本批的 DDL/外键边界。
		if _, err = tx.Exec(ctx, "LOCK TABLE "+traces+", "+events+" IN ROW EXCLUSIVE MODE"); err != nil {
			return 0, 0, err
		}
		if err = guardReferences(ctx, tx); err != nil {
			return 0, 0, err
		}
	}
	ps, err := candidates(ctx, tx, cursor, upper, r.cutoff, r.batch, apply, r.devUnroutedReceipt)
	if err != nil {
		return 0, 0, err
	}
	if len(ps) == 0 {
		return 0, upper, tx.Commit(ctx)
	}
	last = ps[len(ps)-1].trace
	if !apply {
		return len(ps), last, tx.Commit(ctx)
	}
	for t, want := range r.schema {
		got, e := signature(ctx, tx, t)
		if e != nil {
			return 0, 0, e
		}
		if got != want {
			return 0, 0, errors.New("schema_changed")
		}
	}
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE px_idle_ids(trace_id bigint PRIMARY KEY,event_id bigint UNIQUE NOT NULL) ON COMMIT DROP;
 CREATE TEMP TABLE px_idle_restore_t (LIKE `+traces+` INCLUDING DEFAULTS) ON COMMIT DROP;
 CREATE TEMP TABLE px_idle_restore_e (LIKE `+events+` INCLUDING DEFAULTS) ON COMMIT DROP;`); err != nil {
		return 0, 0, err
	}
	values := make([][]any, len(ps))
	for i, p := range ps {
		values[i] = []any{p.trace, p.event}
	}
	if _, err = tx.CopyFrom(ctx, pgx.Identifier{"pg_temp", "px_idle_ids"}, []string{"trace_id", "event_id"}, pgx.CopyFromRows(values)); err != nil {
		return 0, 0, err
	}
	dir := filepath.Join(r.out, fmt.Sprintf("batch-%08d", r.p.Batches+1))
	if err = os.Mkdir(dir, 0700); err != nil {
		return 0, 0, err
	}
	b := batchReceipt{State: "preparing", At: time.Now().UTC(), Cutoff: r.cutoff, Rows: len(ps), FirstID: ps[0].trace, LastID: last, Schema: r.schema}
	for _, v := range []struct{ table, key, temp, file string }{{traces, "trace_id", "px_idle_restore_t", "traces.copy.gz"}, {events, "event_id", "px_idle_restore_e", "events.copy.gz"}} {
		file := filepath.Join(dir, v.file)
		query := "COPY (SELECT s.* FROM " + v.table + " s JOIN pg_temp.px_idle_ids x ON s.id=x." + v.key + " ORDER BY s.id) TO STDOUT WITH (FORMAT binary)"
		a, e := exportCopy(ctx, tx, file, query, len(ps), r.minFree, r.maxBackup-r.p.BackupBytes)
		if e != nil {
			return 0, 0, e
		}
		b.Archives = append(b.Archives, a)
		if r.p.BackupBytes+a.Bytes > r.maxBackup {
			return 0, 0, errors.New("backup_budget_exhausted")
		}
		r.p.BackupBytes += a.Bytes
		if e = restoreCopy(ctx, tx, file, v.temp, len(ps)); e != nil {
			return 0, 0, e
		}
		var mismatches int
		if e = tx.QueryRow(ctx, "SELECT count(*) FROM pg_temp."+v.temp+" b FULL JOIN (SELECT s.* FROM "+v.table+" s JOIN pg_temp.px_idle_ids x ON s.id=x."+v.key+") s ON b.id=s.id WHERE to_jsonb(b) IS DISTINCT FROM to_jsonb(s)").Scan(&mismatches); e != nil {
			return 0, 0, e
		}
		if mismatches != 0 {
			return 0, 0, errors.New("restored_rows_differ")
		}
	}
	b.RestoreVerified = true
	b.State = "delete_intent"
	if err = atomicJSON(filepath.Join(dir, "receipt.json"), b); err != nil {
		return 0, 0, err
	}
	if r.afterArchive != nil {
		if err = r.afterArchive(dir); err != nil {
			return 0, 0, err
		}
	}
	available, e := freeBytes(r.out)
	if e != nil {
		return 0, 0, e
	}
	if available < r.minFree {
		return 0, 0, errors.New("free_space_floor")
	}
	tag, err := tx.Exec(ctx, "DELETE FROM "+traces+" t USING pg_temp.px_idle_ids x WHERE t.id=x.trace_id")
	if err != nil {
		return 0, 0, err
	}
	if tag.RowsAffected() != int64(len(ps)) {
		return 0, 0, errors.New("trace_delete_count_mismatch")
	}
	tag, err = tx.Exec(ctx, "DELETE FROM "+events+" e USING pg_temp.px_idle_ids x WHERE e.id=x.event_id AND NOT EXISTS(SELECT 1 FROM "+traces+" ref WHERE ref.event_publication_id=e.uuid)")
	if err != nil {
		return 0, 0, err
	}
	if tag.RowsAffected() != int64(len(ps)) {
		return 0, 0, errors.New("event_delete_count_mismatch")
	}
	if err = tx.Commit(ctx); err != nil {
		b.State = "commit_unknown"
		_ = atomicJSON(filepath.Join(dir, "receipt.json"), b)
		return 0, 0, errors.New("commit_outcome_unknown_do_not_retry")
	}
	b.State = "committed"
	b.At = time.Now().UTC()
	if err = atomicJSON(filepath.Join(dir, "receipt.json"), b); err != nil {
		return 0, 0, errors.New("commit_receipt_unconfirmed_do_not_retry")
	}
	return len(ps), last, nil
}
func (r *runner) run(ctx context.Context, mode string, highwater, maxPairs int64) error {
	r.p.Mode = mode
	r.p.Cutoff = r.cutoff
	r.p.Highwater = highwater
	if err := r.state("running", nil); err != nil {
		return err
	}
	for r.p.Cursor < highwater {
		if r.runtimeGuard != nil {
			if err := r.runtimeGuard(); err != nil {
				return err
			}
		}
		// 在事务外让出执行权；不因定时器切断一次提交并自动重试未知结果。
		if !r.stopBefore.IsZero() && time.Now().After(r.stopBefore) {
			return r.state("bounded_complete", nil)
		}
		available, err := freeBytes(r.out)
		if err != nil {
			return err
		}
		if available < r.minFree {
			return errors.New("free_space_floor")
		}
		if r.p.BackupBytes >= r.maxBackup {
			return errors.New("backup_budget_exhausted")
		}
		upper := r.p.Cursor + r.span
		if upper > highwater {
			upper = highwater
		}
		one, cancel := context.WithTimeout(ctx, 40*time.Second)
		n, last, err := r.one(one, r.p.Cursor, upper, mode == "apply")
		cancel()
		if err != nil {
			return err
		}
		r.p.Cursor = last
		r.p.Pairs += int64(n)
		if n > 0 {
			r.p.Batches++
		}
		if err = r.state("running", nil); err != nil {
			return err
		}
		if maxPairs > 0 && r.p.Pairs >= maxPairs {
			return r.state("bounded_complete", nil)
		}
	}
	return r.state("complete", nil)
}
func main() {
	mode := flag.String("mode", "preview", "preview or apply")
	devHours := flag.Int("dev-keep-hours", 0, "explicit dev-only exception: 24 hours; zero keeps the seven-day contract")
	devScope := flag.String("expected-dev-scope", "", "optional Core scope fence for reviewed dev maintenance")
	expectedCommit := flag.String("expected-dev-commit", "", "required deployed source commit for the dev-only 24h maintenance")
	database := flag.String("database", "powerx_pro", "fixed prod or development database")
	cut := flag.String("cutoff", "", "explicit seven-day cutoff RFC3339")
	out := flag.String("out", "", "new private job directory")
	limit := flag.Int("batch", 500, "maximum pairs per transaction")
	span := flag.Int64("span", 50000, "bounded primary-key scan span")
	end := flag.Int64("end-id", 0, "optional frozen highwater")
	start := flag.Int64("start-id", 0, "resume only from a confirmed completed cursor")
	seconds := flag.Int("max-seconds", 120, "stop before starting another bounded transaction")
	maxPairs := flag.Int64("max-pairs", 0, "optional bounded run")
	min := flag.Uint64("min-free", 8<<30, "minimum available bytes; apply enforces the host guard and Loki headroom")
	budget := flag.Int64("backup-budget", 2<<30, "maximum compressed rollback backup bytes")
	flag.Parse()
	if (*database != "powerx_pro" && *database != "powerx_dev") || (*mode != "preview" && *mode != "apply") || *out == "" || *limit < 1 || *limit > maximumRetentionBatch(*devHours) || *span < 1 || *span > 100000 || *budget < 1 || *start < 0 || *seconds < 1 || *seconds > 180 {
		fmt.Fprintln(os.Stderr, "invalid_cleanup_parameters")
		os.Exit(2)
	}
	cutoff, err := time.Parse(time.RFC3339, *cut)
	if err != nil || validRetentionCutoff(*database, *devHours, *expectedCommit, cutoff, time.Now()) != nil {
		fmt.Fprintln(os.Stderr, "retention_cutoff_or_development_exception_invalid")
		os.Exit(2)
	}
	if *devScope != "" && (*devHours != 24 || !validDevelopmentScope(*devScope)) {
		fmt.Fprintln(os.Stderr, "invalid_development_control_scope")
		os.Exit(2)
	}
	if err = os.Mkdir(*out, 0700); err != nil {
		fmt.Fprintln(os.Stderr, "out_directory_must_be_new")
		os.Exit(2)
	}
	if *mode == "apply" {
		var s syscall.Statfs_t
		if err = syscall.Statfs(*out, &s); err != nil {
			fmt.Fprintln(os.Stderr, "host_capacity_unavailable")
			os.Exit(2)
		}
		reserved := (uint64(s.Bfree) - uint64(s.Bavail)) * uint64(s.Bsize)
		if floor := hostFloor(uint64(s.Blocks)*uint64(s.Bsize), reserved); *min < floor {
			*min = floor
		}
	} else {
		*min = 0
	}
	dsn := os.Getenv("PX_CLEANUP_DSN")
	if dsn == "" {
		dsn = "postgres://postgres@/" + *database + "?host=/var/run/postgresql&sslmode=disable"
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database_config_invalid")
		os.Exit(2)
	}
	cfg.RuntimeParams["application_name"] = "powerx_idle_taskqueue_cleanup"
	// 长时间跨主键窗口扫描不能复用对空窗口估算的通用计划。
	cfg.RuntimeParams["plan_cache_mode"] = "force_custom_plan"
	cfg.RuntimeParams["statement_timeout"] = "20000"
	cfg.RuntimeParams["lock_timeout"] = "1000"
	cfg.RuntimeParams["idle_in_transaction_session_timeout"] = "60000"
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database_connect_failed")
		os.Exit(2)
	}
	r := &runner{conn: conn, out: *out, cutoff: cutoff, batch: *limit, span: *span, minFree: *min, maxBackup: *budget}
	if *devHours == 24 {
		r.devUnroutedReceipt = true
		r.devScope = *devScope
		r.runtimeGuard = func() error {
			return checkDevelopmentCommit("/opt/powerx-dev/backend/powerx.build.json", *expectedCommit)
		}
	}
	r.p.Cursor = *start
	r.stopBefore = time.Now().Add(time.Duration(*seconds) * time.Second)
	var db string
	err = conn.QueryRow(ctx, "SELECT current_database()").Scan(&db)
	if err == nil && db != *database {
		err = errors.New("unexpected_database")
	}
	if err == nil {
		err = r.preflight(ctx)
	}
	if err == nil && *end == 0 {
		err = conn.QueryRow(ctx, "SELECT COALESCE(max(id),0) FROM "+traces).Scan(end)
	}
	if err == nil {
		err = atomicJSON(filepath.Join(*out, "run-contract.json"), map[string]any{"scope": "successful typed taskqueue.v2 empty dequeue from AI Craft only; paired unrouted or never-attempted pending notifications; explicit cutoff enforced before scan; default seven days, dev-only 24h exception; original rows archived and restored before deletion", "cutoff": cutoff, "highwater": *end, "schema": r.schema, "postgres_version": conn.PgConn().ParameterStatus("server_version"), "retention_hours": effectiveRetentionHours(*devHours), "expected_dev_commit": *expectedCommit, "backup_review_after": time.Now().UTC().Add(7 * 24 * time.Hour), "automatic_backup_deletion": false})
	}
	if err == nil {
		err = r.run(ctx, *mode, *end, *maxPairs)
	}
	if err != nil {
		_ = r.state("stopped_error", err)
		fmt.Fprintln(os.Stderr, safeError(err))
	}
	_ = conn.Close(ctx)
	b, _ := json.Marshal(r.p)
	fmt.Println(strings.TrimSpace(string(b)))
	if err != nil {
		os.Exit(1)
	}
}
