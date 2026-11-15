package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T) (*pgx.Conn, time.Time) {
	t.Helper()
	dsn := os.Getenv("PX_IDLE_FIXTURE_DSN")
	if dsn == "" {
		t.Fatal("isolated fixture DSN required")
	}
	c, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close(context.Background()) })
	var db string
	if err = c.QueryRow(context.Background(), "SELECT current_database()").Scan(&db); err != nil || db != "px_idle_fixture" {
		t.Fatal("refuse non-fixture database")
	}
	_, err = c.Exec(context.Background(), `DROP TABLE IF EXISTS public.foreign_ref,public.capability_registry_invocation_traces,public.capability_registry_event_publications,public.iam_tenant CASCADE;
 CREATE TABLE public.iam_tenant(uuid uuid PRIMARY KEY,deleted_at timestamptz);
 CREATE TABLE public.capability_registry_event_publications(id bigint PRIMARY KEY,uuid uuid UNIQUE NOT NULL,created_at timestamptz,updated_at timestamptz,deleted_at timestamptz,tenant_uuid char(36),topic text,payload jsonb,status text,attempts bigint,last_error text,trace_id text,publish_time timestamptz,opaque bytea);
 CREATE TABLE public.capability_registry_invocation_traces(id bigint PRIMARY KEY,uuid uuid UNIQUE NOT NULL,created_at timestamptz,updated_at timestamptz,deleted_at timestamptz,trace_id text,tenant_uuid char(36),plugin_id text,capability_id text,protocol_used text,fallback_used bool,status text,request_payload jsonb,response_payload jsonb,error_summary text,event_published bool,event_publication_id uuid,caller_subject text,opaque bytea);
 CREATE INDEX idx_capability_trace_publication ON public.capability_registry_invocation_traces(event_publication_id);
 INSERT INTO public.iam_tenant VALUES('00000000-0000-0000-0000-000000000001',NULL);
 INSERT INTO public.capability_registry_event_publications
 SELECT g,md5('event'||g)::uuid,now()-interval '10 days',now()-interval '10 days',NULL,'00000000-0000-0000-0000-000000000001','_topic.integration.gateway.invocation.succeeded',
 jsonb_build_object('tenant_uuid','00000000-0000-0000-0000-000000000001','trace_id','trace-'||g,'capability_id','com.corex.runtime.taskqueue.service_consume','plugin_id','corex.platform','status','completed'),'pending',0,NULL,'trace-'||g,NULL,decode('000102ff','hex') FROM generate_series(1,14)g;
 INSERT INTO public.capability_registry_invocation_traces
 SELECT g,md5('trace'||g)::uuid,now()-interval '10 days',now()-interval '10 days',NULL,'trace-'||g,'00000000-0000-0000-0000-000000000001','corex.platform','com.corex.runtime.taskqueue.service_consume','core_internal',false,'completed',
 '{"body":{"contract_version":"taskqueue.v2","operation":"dequeue","tenant_key":"00000000-0000-0000-0000-000000000001","subscriber_id":"fixture","worker_id":"fixture","max_items":1,"wait_timeout_ms":50}}'::jsonb,jsonb_build_object('contract_version','taskqueue.v2','messages','[]'::jsonb,'unicode','安全验证','escaped',E'newline\nbackslash\\quote"'),'',false,md5('event'||g)::uuid,'sts:com.powerx.plugins.ai-craft:fixture',decode('000102ff','hex') FROM generate_series(1,14)g;
 UPDATE public.capability_registry_invocation_traces SET created_at=now(),updated_at=now() WHERE id=2;
 UPDATE public.capability_registry_invocation_traces SET status='failed' WHERE id=3;
 UPDATE public.capability_registry_invocation_traces SET caller_subject='other-plugin' WHERE id=4;
 UPDATE public.capability_registry_event_publications SET payload=payload||'{"tenant_uuid":"foreign"}'::jsonb WHERE id=5;
 UPDATE public.capability_registry_invocation_traces SET event_publication_id=md5('event6')::uuid,tenant_uuid='00000000-0000-0000-0000-000000000002' WHERE id=7;
 UPDATE public.capability_registry_invocation_traces SET request_payload='{"body":{"operation":"set_baselines","request":{"schema":"powerx.iam.authorization-baseline-query/v1"}}}'::jsonb WHERE id=8;
 UPDATE public.capability_registry_invocation_traces SET tenant_uuid='00000000-0000-0000-0000-000000000002' WHERE id=9;
 UPDATE public.capability_registry_event_publications SET status='published' WHERE id=10;
 UPDATE public.capability_registry_invocation_traces SET updated_at=now() WHERE id=11;
 UPDATE public.capability_registry_invocation_traces SET deleted_at=now() WHERE id=12;
 UPDATE public.capability_registry_invocation_traces SET event_publication_id=NULL WHERE id=13;
 ALTER TABLE public.capability_registry_invocation_traces ADD COLUMN record_class text DEFAULT 'legacy_unclassified', ADD COLUMN completed_at timestamptz;
 ALTER TABLE public.capability_registry_event_publications ADD COLUMN record_class text DEFAULT 'legacy_unclassified', ADD COLUMN terminal_at timestamptz, ADD COLUMN delivery_receipt text DEFAULT '';`)
	if err != nil {
		t.Fatal(err)
	}
	return c, time.Now().Add(-7 * 24 * time.Hour)
}
func newRunner(t *testing.T, c *pgx.Conn, cut time.Time) *runner {
	t.Helper()
	r := &runner{conn: c, out: t.TempDir(), cutoff: cut, batch: 1, span: 10, minFree: 0, maxBackup: 1 << 30}
	if err := r.preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r
}
func snapshot(t *testing.T, c *pgx.Conn, table string) map[int64]string {
	t.Helper()
	rows, err := c.Query(context.Background(), "SELECT id,to_jsonb(t)::text FROM "+table+" t ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var s string
		if err = rows.Scan(&id, &s); err != nil {
			t.Fatal(err)
		}
		out[id] = s
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	return out
}
func assertUnchanged(t *testing.T, before, after map[int64]string, removed map[int64]bool) {
	t.Helper()
	if len(after) != len(before)-len(removed) {
		t.Fatalf("row count before=%d after=%d", len(before), len(after))
	}
	for id, s := range before {
		if removed[id] {
			if _, ok := after[id]; ok {
				t.Fatalf("expected removal %d", id)
			}
		} else if after[id] != s {
			t.Fatalf("protected row changed %d", id)
		}
	}
}

func TestApplyAndRecoverExactBinaryRows(t *testing.T) {
	c, cut := fixture(t)
	beforeT := snapshot(t, c, traces)
	beforeE := snapshot(t, c, events)
	r := newRunner(t, c, cut)
	if err := r.run(context.Background(), "apply", 14, 0); err != nil {
		t.Fatal(err)
	}
	if r.p.Pairs != 2 {
		t.Fatalf("pairs=%d", r.p.Pairs)
	}
	assertUnchanged(t, beforeT, snapshot(t, c, traces), map[int64]bool{1: true, 14: true})
	assertUnchanged(t, beforeE, snapshot(t, c, events), map[int64]bool{1: true, 14: true})
	tx, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), "CREATE TEMP TABLE px_recovered (LIKE "+traces+" INCLUDING DEFAULTS) ON COMMIT DROP"); err != nil {
		t.Fatal(err)
	}
	if err = restoreCopy(context.Background(), tx, filepath.Join(r.out, "batch-00000001", "traces.copy.gz"), "px_recovered", 1); err != nil {
		t.Fatal(err)
	}
	var recovered string
	if err = tx.QueryRow(context.Background(), "SELECT to_jsonb(t)::text FROM pg_temp.px_recovered t WHERE id=1").Scan(&recovered); err != nil {
		t.Fatal(err)
	}
	if recovered != beforeT[1] {
		t.Fatal("recovered row differs after original deletion")
	}
	var receipt batchReceipt
	b, err := os.ReadFile(filepath.Join(r.out, "batch-00000001", "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.State != "committed" || !receipt.RestoreVerified || len(receipt.Archives) != 2 {
		t.Fatal("invalid durable receipt")
	}
}

func TestConfirmedCursorKeepsEarlierEligibleRows(t *testing.T) {
	c, cut := fixture(t)
	r := newRunner(t, c, cut)
	r.p.Cursor = 1
	if err := r.run(context.Background(), "apply", 14, 0); err != nil {
		t.Fatal(err)
	}
	remaining := snapshot(t, c, traces)
	if _, ok := remaining[1]; !ok {
		t.Fatal("confirmed cursor reprocessed an earlier row")
	}
	if _, ok := remaining[14]; ok {
		t.Fatal("eligible row after cursor was not processed")
	}
}

func TestRuntimeBudgetStopsWithoutStartingTransaction(t *testing.T) {
	c, cut := fixture(t)
	before := snapshot(t, c, traces)
	r := newRunner(t, c, cut)
	r.stopBefore = time.Now().Add(-time.Second)
	if err := r.run(context.Background(), "apply", 14, 0); err != nil {
		t.Fatal(err)
	}
	if r.p.State != "bounded_complete" || r.p.Pairs != 0 {
		t.Fatal("runtime budget did not stop before work")
	}
	assertUnchanged(t, before, snapshot(t, c, traces), map[int64]bool{})
}
func TestPreviewWithDatabaseReadOnlyAndProtectedRows(t *testing.T) {
	c, cut := fixture(t)
	before := snapshot(t, c, traces)
	r := newRunner(t, c, cut)
	if _, err := c.Exec(context.Background(), "SET default_transaction_read_only=on"); err != nil {
		t.Fatal(err)
	}
	if err := r.run(context.Background(), "preview", 14, 0); err != nil {
		t.Fatal(err)
	}
	if r.p.Pairs != 2 {
		t.Fatalf("pairs=%d", r.p.Pairs)
	}
	assertUnchanged(t, before, snapshot(t, c, traces), nil)
	dirs, _ := filepath.Glob(filepath.Join(r.out, "batch-*"))
	if len(dirs) != 0 {
		t.Fatal("preview wrote data backup")
	}
}
func TestFailureAfterArchiveRollsBackBothTables(t *testing.T) {
	c, cut := fixture(t)
	bt := snapshot(t, c, traces)
	be := snapshot(t, c, events)
	r := newRunner(t, c, cut)
	r.afterArchive = func(string) error { return errors.New("fixture injected failure") }
	if err := r.run(context.Background(), "apply", 14, 0); err == nil {
		t.Fatal("expected failure")
	}
	assertUnchanged(t, bt, snapshot(t, c, traces), nil)
	assertUnchanged(t, be, snapshot(t, c, events), nil)
}
func TestNewForeignReferenceRollsBackWholeBatch(t *testing.T) {
	c, cut := fixture(t)
	bt := snapshot(t, c, traces)
	be := snapshot(t, c, events)
	r := newRunner(t, c, cut)
	r.afterArchive = func(string) error {
		_, err := c.Exec(context.Background(), "INSERT INTO "+traces+" SELECT (jsonb_populate_record(NULL::"+traces+",to_jsonb(t)||jsonb_build_object('id',100,'uuid',md5('extra')::uuid))).* FROM "+traces+" t WHERE id=1")
		return err
	}
	if err := r.run(context.Background(), "apply", 14, 0); err == nil {
		t.Fatal("expected reference mismatch")
	}
	assertUnchanged(t, bt, snapshot(t, c, traces), nil)
	assertUnchanged(t, be, snapshot(t, c, events), nil)
}
func TestFreeSpaceAndBackupBudgetFailClosed(t *testing.T) {
	for _, kind := range []string{"free", "backup"} {
		t.Run(kind, func(t *testing.T) {
			c, cut := fixture(t)
			before := snapshot(t, c, traces)
			r := newRunner(t, c, cut)
			if kind == "free" {
				r.minFree = ^uint64(0)
			} else {
				r.maxBackup = 1
			}
			if err := r.run(context.Background(), "apply", 14, 0); err == nil {
				t.Fatal("expected safety stop")
			}
			// pgconn deliberately closes a COPY TO connection if its writer refuses
			// data. Verify rollback through a fresh connection, without resetting data.
			if c.IsClosed() {
				fresh, err := pgx.Connect(context.Background(), os.Getenv("PX_IDLE_FIXTURE_DSN"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { fresh.Close(context.Background()) })
				c = fresh
			}
			assertUnchanged(t, before, snapshot(t, c, traces), nil)
		})
	}
}
func TestForeignKeyPreflightRequiresReview(t *testing.T) {
	c, cut := fixture(t)
	if _, err := c.Exec(context.Background(), "CREATE TABLE public.foreign_ref(id bigint REFERENCES "+traces+"(id))"); err != nil {
		t.Fatal(err)
	}
	r := &runner{conn: c, cutoff: cut}
	if err := r.preflight(context.Background()); err == nil {
		t.Fatal("foreign key should stop cleanup")
	}
}

func TestHostFloorProtectsLokiAndExistingGuard(t *testing.T) {
	for _, v := range []struct{ total, reserved uint64 }{{126424895488, 5368516608}, {1 << 40, 10 << 30}} {
		floor := hostFloor(v.total, v.reserved)
		if floor < 8<<30 {
			t.Fatal("weakened existing critical guard")
		}
		used := v.total - v.reserved - floor
		if used > v.total*9/10-(1<<30) {
			t.Fatal("archive may enter Loki's refusal zone")
		}
	}
}
func TestArchiveWriteCannotConsumeProtectedSpaceOrExceedBudget(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "guard")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := &guardedArchiveWriter{file: f, path: filepath.Dir(f.Name()), floor: ^uint64(0), allowance: 10}
	if n, e := w.Write([]byte("123")); e == nil || n != 0 {
		t.Fatal("wrote below protected free-space floor")
	}
	w.floor = 0
	w.allowance = 4
	if n, e := w.Write([]byte("12345")); e == nil || n != 0 {
		t.Fatal("exceeded backup budget before write")
	}
	if n, e := w.Write([]byte("123")); e != nil || n != 3 {
		t.Fatal("valid bounded write failed")
	}
	if n, e := w.Write([]byte("45")); e == nil || n != 0 {
		t.Fatal("cumulative backup budget exceeded")
	}
	st, _ := f.Stat()
	if st.Size() != 3 {
		t.Fatal("failed guard write changed file")
	}
}
