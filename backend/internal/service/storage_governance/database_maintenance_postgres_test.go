package storagegov

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	mevent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func maintenanceNative(t *testing.T) (*Service, *repo.DatabaseMaintenanceRepository) {
	t.Helper()
	db, queue := ownedControlPostgres(t)
	schema := coremodel.PowerXSchema
	for _, name := range []string{"capability_registry_invocation_traces", "capability_registry_event_publications"} {
		table := pgx.Identifier{schema, name}.Sanitize()
		require.NoError(t, db.Exec("CREATE TABLE "+table+" (id bigint PRIMARY KEY,payload text NOT NULL)").Error)
		require.NoError(t, db.Exec("INSERT INTO "+table+" SELECT n,repeat(md5(n::text),32) FROM generate_series(1,8192) n").Error)
		require.NoError(t, db.Exec("DELETE FROM "+table+" WHERE id>1024").Error)
		require.NoError(t, db.Exec("ANALYZE "+table).Error)
	}
	require.NoError(t, db.Exec("CREATE TABLE "+pgx.Identifier{schema, "iam_user"}.Sanitize()+" (id bigint PRIMARY KEY,is_root boolean,status smallint,deleted_at timestamptz); INSERT INTO "+pgx.Identifier{schema, "iam_user"}.Sanitize()+" VALUES(7,true,1,NULL)").Error)
	var dataDirectory string
	require.NoError(t, db.Raw("SHOW data_directory").Scan(&dataDirectory).Error)
	root := t.TempDir()
	scope := ControlScope{HostID: "owned-host", FilesystemID: "owned-disk", InstanceID: "owned-install"}
	c, e := budget.New(budget.Config{Directory: filepath.Join(root, "runtime"), FilesystemPath: root, HostID: scope.HostID, FilesystemID: scope.FilesystemID, InstanceID: scope.InstanceID, MinFreeBytes: 1 << 20, CriticalFreeBytes: 1, MaxConcurrent: 1})
	require.NoError(t, e)
	ctrl, e := NewControlService(db, ControlOptions{Scope: scope, Queue: queue})
	require.NoError(t, e)
	s, e := NewService(Options{DB: db, Control: ctrl, Budget: c, Collector: &fixtureCollector{}, FilesystemPath: root, MaintenanceDataDirectory: dataDirectory, Identity: Identity{HostID: scope.HostID, FilesystemID: scope.FilesystemID, InstanceID: scope.InstanceID, Environment: "test"}, ControlWorkerEnabled: true, DatabaseRetentionAllowed: true})
	require.NoError(t, e)
	return s, s.maintenance.backend.(*repo.DatabaseMaintenanceRepository)
}
func maintenanceDataDigest(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	var value string
	require.NoError(t, db.Raw("SELECT md5(string_agg(md5(entry::text),'' ORDER BY id)) FROM "+pgx.Identifier{coremodel.PowerXSchema, name}.Sanitize()+" entry").Scan(&value).Error)
	return value
}

func TestDatabaseMaintenanceNativePostgresLoopbackTCPVerifiesLocalDisk(t *testing.T) {
	s, _ := maintenanceNative(t)
	var listeners string
	require.NoError(t, s.opts.DB.Raw("SHOW listen_addresses").Scan(&listeners).Error)
	if !strings.Contains(listeners, "127.0.0.1") {
		t.Skip("owned PostgreSQL fixture does not listen on IPv4 loopback")
	}
	// Use the TCP listener of the same isolated cluster as the owned socket.
	cfg, err := pgx.ParseConfig(os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN"))
	require.NoError(t, err)
	cfg.Host = "127.0.0.1"
	cfg.Fallbacks = nil
	pool := stdlib.OpenDB(*cfg)
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var directory, address string
	require.NoError(t, pool.QueryRowContext(ctx, "SELECT current_setting('data_directory'), inet_server_addr()::text").Scan(&directory, &address))
	require.Equal(t, s.opts.MaintenanceDataDirectory, directory)
	require.Equal(t, "127.0.0.1/32", address, "exercise the inet-to-text form that previously failed the IP check")

	tcp, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}), &gorm.Config{})
	require.NoError(t, err)
	r := repo.NewDatabaseMaintenanceRepository(tcp, directory, s.opts.FilesystemPath)
	snapshot, err := r.Inspect(ctx)
	require.NoError(t, err)
	require.True(t, snapshot.LocalDiskVerified, "same-device maintenance must admit a local TCP connection")

	r = repo.NewDatabaseMaintenanceRepository(tcp, t.TempDir(), s.opts.FilesystemPath)
	snapshot, err = r.Inspect(ctx)
	require.NoError(t, err)
	require.False(t, snapshot.LocalDiskVerified, "loopback alone must not admit an unrelated data directory")
}

func TestDatabaseMaintenanceNativePostgresTaskBusCompactionPreservesRowsAndScope(t *testing.T) {
	s, _ := maintenanceNative(t)
	db := s.opts.DB
	before := map[string]string{}
	for _, name := range []string{"capability_registry_invocation_traces", "capability_registry_event_publications"} {
		before[name] = maintenanceDataDigest(t, db, name)
	}
	ctx, stop := context.WithCancel(context.Background())
	s.StartControl(ctx, nil)
	s.StartDatabaseMaintenance(ctx, nil)
	t.Cleanup(func() {
		stop()
		select {
		case <-s.maintenance.done:
		case <-time.After(8 * time.Second):
			t.Error("maintenance worker did not exit")
		}
		select {
		case <-s.opts.Control.Done():
		case <-time.After(8 * time.Second):
			t.Error("control worker did not exit")
		}
	})
	preview, e := s.PreviewDatabaseMaintenance(controlRoot(), DatabaseMaintenancePreview{Mode: "compact", Tables: []string{"capability-invocation-traces", "capability-event-publications"}})
	require.NoError(t, e)
	require.True(t, preview.Before.LocalDiskVerified)
	r, e := s.StartDatabaseMaintenanceRequest(controlRoot(), DatabaseMaintenanceRequest{RequestID: uuid.NewString(), PreviewID: preview.ID, Confirm: true, ExclusiveLockConfirmed: true})
	require.NoError(t, e)
	var final DatabaseMaintenanceRun
	require.Eventually(t, func() bool {
		var e error
		final, e = s.DatabaseMaintenanceRun(controlRoot(), r.ID)
		return e == nil && final.State == "completed"
	}, 12*time.Second, 50*time.Millisecond)
	require.Len(t, final.Steps, 2)
	var task mevent.TaskHistory
	require.NoError(t, db.Where("task_id = ? AND subscriber_id = ?", r.ID, s.maintenanceSubscriber()).First(&task).Error)
	require.Equal(t, mevent.TaskHistoryStatusCompleted, task.Status, "TaskBus and maintenance completion must commit together")
	require.NotNil(t, final.After)
	var oldTotal, newTotal int64
	for _, x := range final.Before.Tables {
		oldTotal += x.TotalBytes
	}
	for _, x := range final.After.Tables {
		newTotal += x.TotalBytes
	}
	require.Less(t, newTotal, oldTotal)
	for name, digest := range before {
		require.Equal(t, digest, maintenanceDataDigest(t, db, name))
	}
	_, e = repo.NewMaintenanceRunRepository(db, "other-installation").Get(context.Background(), db, r.ID)
	require.ErrorIs(t, e, gorm.ErrRecordNotFound)
	again, e := s.StartDatabaseMaintenanceRequest(controlRoot(), DatabaseMaintenanceRequest{RequestID: r.ID, PreviewID: preview.ID, Confirm: true, ExclusiveLockConfirmed: true})
	require.NoError(t, e)
	require.Equal(t, final.ID, again.ID)
}
func TestDatabaseMaintenanceNativePostgresAutocommitAndConnectionSettingsRestored(t *testing.T) {
	s, r := maintenanceNative(t)
	pool, e := s.opts.DB.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	read := func() []string {
		rows, e := pool.QueryContext(context.Background(), "SELECT current_setting('application_name'),current_setting('statement_timeout'),current_setting('lock_timeout'),current_setting('vacuum_cost_delay')")
		require.NoError(t, e)
		defer rows.Close()
		require.True(t, rows.Next())
		v := make([]string, 4)
		require.NoError(t, rows.Scan(&v[0], &v[1], &v[2], &v[3]))
		return v
	}
	before := read()
	snapshot, e := r.Inspect(context.Background())
	require.NoError(t, e)
	var session repo.MaintenanceSession
	require.NoError(t, r.Execute(context.Background(), uuid.NewString(), "reuse", snapshot, []string{"capability-invocation-traces"}, 0, func(_ string, x repo.MaintenanceSession) error { session = x; return nil }, func(string) error { return nil }, nil, nil))
	require.Equal(t, before, read())
	active, e := r.SessionActive(context.Background(), session, false)
	require.NoError(t, e)
	require.False(t, active)
}
func TestDatabaseMaintenanceNativePostgresCancellationUsesPinnedSessionAndPreservesRows(t *testing.T) {
	s, r := maintenanceNative(t)
	db := s.opts.DB
	pool, e := db.DB()
	require.NoError(t, e)
	digest := maintenanceDataDigest(t, db, "capability_registry_invocation_traces")
	holder, e := pool.BeginTx(context.Background(), nil)
	require.NoError(t, e)
	defer holder.Rollback()
	_, e = holder.Exec("LOCK TABLE " + pgx.Identifier{coremodel.PowerXSchema, "capability_registry_invocation_traces"}.Sanitize() + " IN ACCESS SHARE MODE")
	require.NoError(t, e)
	snapshot, e := r.Inspect(context.Background())
	require.NoError(t, e)
	ready := make(chan repo.MaintenanceSession, 1)
	result := make(chan error, 1)
	go func() {
		result <- r.Execute(context.Background(), uuid.NewString(), "compact", snapshot, []string{"capability-invocation-traces"}, 0, func(_ string, x repo.MaintenanceSession) error { ready <- x; return nil }, func(string) error { return nil }, nil, nil)
	}()
	session := <-ready
	require.Eventually(t, func() bool {
		var waiting bool
		e := db.Raw("SELECT wait_event_type='Lock' FROM pg_stat_activity WHERE pid=?", session.PID).Scan(&waiting).Error
		return e == nil && waiting
	}, time.Second, 10*time.Millisecond)
	wrong := session
	wrong.StartedAt = wrong.StartedAt.Add(time.Second)
	active, e := r.SessionActive(context.Background(), wrong, true)
	require.NoError(t, e)
	require.False(t, active)
	active, e = r.SessionActive(context.Background(), session, true)
	require.NoError(t, e)
	require.True(t, active)
	e = <-result
	var pgError *pgconn.PgError
	require.True(t, errors.As(e, &pgError))
	require.Equal(t, "57014", pgError.Code)
	require.NoError(t, holder.Rollback())
	require.Equal(t, digest, maintenanceDataDigest(t, db, "capability_registry_invocation_traces"))
	active, e = r.SessionActive(context.Background(), session, true)
	require.NoError(t, e)
	require.False(t, active, "connection returned to pool must not remain cancellable as this job")
}

func TestDatabaseMaintenanceNativePostgresIncrementalReclaimsOnlyNotificationTail(t *testing.T) {
	s, r := maintenanceNative(t)
	db := s.opts.DB
	ctx := controlRoot()
	name := pgx.Identifier{coremodel.PowerXSchema, "capability_registry_event_publications"}.Sanitize()
	business := pgx.Identifier{coremodel.PowerXSchema, "orders"}.Sanitize()
	require.NoError(t, db.Exec("DROP TABLE "+name+"; CREATE TABLE "+name+` (
 id bigserial PRIMARY KEY, uuid uuid, created_at timestamptz, updated_at timestamptz, deleted_at timestamptz,
 tenant_uuid char(36) NOT NULL, topic varchar(256) NOT NULL, payload jsonb NOT NULL,
 status varchar(32) NOT NULL, attempts bigint NOT NULL DEFAULT 0, last_error text,
 trace_id varchar(128) NOT NULL, publish_time timestamptz, record_class varchar(64) NOT NULL,
 terminal_at timestamptz, delivery_receipt varchar(512) NOT NULL DEFAULT ''
) WITH (autovacuum_enabled=false)`).Error)
	require.NoError(t, db.Exec("INSERT INTO "+name+` (id,uuid,created_at,updated_at,tenant_uuid,topic,payload,status,trace_id,record_class,terminal_at)
SELECT n,md5(n::text)::uuid,now(),now(),md5('fixture-tenant')::uuid::text,
 '_topic.integration.gateway.invocation.succeeded',jsonb_build_object('body',repeat(md5(n::text),12),'terminal',true),
 'quarantined',md5(n::text),CASE WHEN n=160000 THEN 'legacy_unclassified' ELSE 'capability_invocation_notification' END,now()
FROM generate_series(1,160000) n`).Error)
	for i, columns := range []string{"uuid", "trace_id", "status", "topic", "deleted_at", "tenant_uuid", "tenant_uuid,status,terminal_at"} {
		require.NoError(t, db.Exec(fmt.Sprintf("CREATE INDEX notification_test_%d ON %s (%s)", i, name, columns)).Error)
	}
	require.NoError(t, db.Exec("DELETE FROM "+name+" WHERE id<=96000").Error)
	require.NoError(t, db.Exec("VACUUM (TRUNCATE FALSE, INDEX_CLEANUP ON, PARALLEL 0) "+name).Error)
	require.NoError(t, db.Exec("CREATE TABLE "+business+" (id int PRIMARY KEY, amount int); INSERT INTO "+business+" VALUES(901,123)").Error)

	var version string
	require.NoError(t, db.Raw("SHOW server_version").Scan(&version).Error)
	if !strings.HasPrefix(version, "16.") {
		t.Skip("incremental compaction requires PostgreSQL 16")
	}
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS pg_freespacemap").Error)
	auditBefore := maintenanceDataDigest(t, db, "capability_registry_invocation_traces")
	businessBefore := maintenanceDataDigest(t, db, "orders")
	digest := func() string {
		var out string
		require.NoError(t, db.Raw("SELECT md5(string_agg(md5(entry::text),'' ORDER BY id)) FROM "+name+" entry").Scan(&out).Error)
		return out
	}
	protectedDigest := digest()
	input := DatabaseMaintenancePreview{Mode: repo.IncrementalMaintenanceMode, Tables: []string{repo.IncrementalMaintenanceTable}}
	for _, ids := range [][]string{{"orders"}, {repo.IncrementalMaintenanceTable, "capability-invocation-traces"}} {
		_, e := s.PreviewDatabaseMaintenance(ctx, DatabaseMaintenancePreview{Mode: input.Mode, Tables: ids})
		require.Equal(t, "DATABASE_MAINTENANCE_INPUT_INVALID", dto.CodeOf(e))
	}
	// A dependency on a business table must prevent any write, even for valid notifications.
	require.NoError(t, db.Exec("ALTER TABLE "+name+" ADD COLUMN order_id int REFERENCES "+business+"(id)").Error)
	_, e := s.PreviewDatabaseMaintenance(ctx, input)
	require.Equal(t, "DATABASE_MAINTENANCE_INCREMENTAL_UNSUPPORTED", dto.CodeOf(e), "%v", e)
	require.NoError(t, db.Exec("ALTER TABLE "+name+" DROP COLUMN order_id").Error)
	require.Equal(t, protectedDigest, digest())

	snapshot, e := r.Inspect(ctx)
	require.NoError(t, e)
	require.Empty(t, snapshot.IncrementalProblem)
	_, e = s.PreviewDatabaseMaintenance(ctx, input)
	require.Equal(t, "DATABASE_MAINTENANCE_INCREMENTAL_PROTECTED_TAIL", dto.CodeOf(e), "%v", e)
	require.Equal(t, protectedDigest, digest())
	require.NoError(t, db.Exec("UPDATE "+name+" SET record_class='capability_invocation_notification' WHERE record_class='legacy_unclassified'").Error)
	expected := digest()
	input.WindowMinutes = 60
	input.TargetDatabaseBytes = snapshot.DatabaseBytes - (36 << 20)

	runCtx, stop := context.WithCancel(context.Background())
	s.StartControl(runCtx, nil)
	s.StartDatabaseMaintenance(runCtx, nil)
	t.Cleanup(func() {
		stop()
		<-s.maintenance.done
		<-s.opts.Control.Done()
	})
	heapBytes := func(snapshot repo.MaintenanceSnapshot) int64 {
		for _, table := range snapshot.Tables {
			if table.ID == repo.IncrementalMaintenanceTable {
				return table.HeapBytes
			}
		}
		t.Fatal("notification table measurement missing")
		return 0
	}
	// Generate WAL while the index pass runs on the owned test server. The
	// fixture leaves recycled segments unavailable so the WAL files grow.
	emitWAL := func(megabytes int) {
		t.Helper()
		require.NoError(t, db.Exec(`SELECT count(pg_logical_emit_message(false,'powerx-test-only',repeat('w',1048576)))
FROM generate_series(1,?)`, megabytes).Error)
	}
	walFileBytes := func() int64 {
		t.Helper()
		var size int64
		require.NoError(t, db.Raw("SELECT COALESCE(sum(size),0)::bigint FROM pg_catalog.pg_ls_waldir()").Scan(&size).Error)
		return size
	}

	// Keep the first index cleanup long enough to observe the WAL workload
	// while VACUUM is executing. Reopen the fixture's idle connections.
	require.NoError(t, db.Exec("ALTER ROLE CURRENT_USER SET vacuum_cost_page_dirty=200").Error)
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxIdleConns(0)
	pool.SetMaxIdleConns(4)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("ALTER ROLE CURRENT_USER RESET vacuum_cost_page_dirty").Error)
	})
	preview, e := s.PreviewDatabaseMaintenance(ctx, input)
	require.NoError(t, e)
	require.EqualValues(t, 1152<<20, preview.PeakBytes)
	initialWALFiles := walFileBytes()
	var vacuumCount int64
	require.NoError(t, db.Raw(`SELECT vacuum_count FROM pg_stat_user_tables WHERE relid=to_regclass(?)`, name).Scan(&vacuumCount).Error)
	req := DatabaseMaintenanceRequest{RequestID: uuid.NewString(), PreviewID: preview.ID, Confirm: true}
	_, e = s.StartDatabaseMaintenanceRequest(ctx, req)
	require.Equal(t, "DATABASE_MAINTENANCE_LOCK_CONFIRMATION_REQUIRED", dto.CodeOf(e))
	req.ExclusiveLockConfirmed = true
	confirmed, e := s.StartDatabaseMaintenanceRequest(ctx, req)
	require.NoError(t, e)
	require.NotNil(t, confirmed.WindowEndsAt)
	require.Equal(t, confirmed.At.Add(time.Hour), *confirmed.WindowEndsAt)
	require.Equal(t, input.TargetDatabaseBytes, confirmed.TargetDatabaseBytes)
	var vacuumStart string
	require.Eventually(t, func() bool {
		return db.Raw(`SELECT pg_current_wal_insert_lsn()::text
FROM pg_stat_progress_vacuum v JOIN pg_stat_activity a USING (pid)
WHERE a.application_name=? AND v.phase IN ('vacuuming indexes','scanning heap')`,
			"px.dbmaint."+req.RequestID).Scan(&vacuumStart).Error == nil && vacuumStart != ""
	}, 20*time.Second, 10*time.Millisecond)
	// A concurrent reader lets the index/heap cleanup finish but prevents the
	// first VACUUM from acquiring the exclusive lock for tail truncation.
	reader := db.Begin()
	require.NoError(t, reader.Error)
	defer reader.Rollback()
	require.NoError(t, reader.Exec("LOCK TABLE ONLY "+name+" IN ACCESS SHARE MODE").Error)
	emitWAL(192)
	var walBytes int64
	require.NoError(t, db.Raw("SELECT pg_wal_lsn_diff(pg_current_wal_insert_lsn(), ?::pg_lsn)::bigint", vacuumStart).Scan(&walBytes).Error)
	require.Greater(t, walBytes, int64(160<<20))
	fileGrowth := walFileBytes() - initialWALFiles
	require.Greater(t, fileGrowth, int64(128<<20))
	t.Logf("WAL generated after VACUUM started: %d bytes, WAL files grew by %d bytes", walBytes, fileGrowth)
	require.Eventually(t, func() bool {
		var waiting bool
		return db.Raw(`SELECT EXISTS(SELECT FROM pg_stat_activity WHERE application_name=? AND wait_event='VacuumTruncate')`,
			"px.dbmaint."+req.RequestID).Scan(&waiting).Error == nil && waiting
	}, 18*time.Second, 10*time.Millisecond, "first VACUUM must attempt to truncate with the reader still holding the lock")
	require.Eventually(t, func() bool {
		var count int64
		return db.Raw(`SELECT vacuum_count FROM pg_stat_user_tables WHERE relid=to_regclass(?)`, name).Scan(&count).Error == nil && count > vacuumCount
	}, 8*time.Second, 20*time.Millisecond, "first VACUUM must return without truncating")
	var blockedHeapBytes int64
	require.NoError(t, reader.Raw(`SELECT pg_relation_size(to_regclass(?))`, name).Scan(&blockedHeapBytes).Error)
	require.Equal(t, heapBytes(preview.Before), blockedHeapBytes)
	require.NoError(t, reader.Commit().Error)

	var final DatabaseMaintenanceRun
	var workloadErr error
	var workloadSamples int
	var longestWorkloadSample time.Duration
	require.Eventually(t, func() bool {
		final, e = s.DatabaseMaintenanceRun(ctx, req.RequestID)
		if e == nil && (final.State == "completed" || final.State == "failed" || final.State == "unconfirmed" || final.State == "cancelled") {
			return true
		}
		// Keep a business read and a write to the selected system table active
		// while later slices run, using only test rows with unchanged values.
		started := time.Now()
		query, stopQuery := context.WithTimeout(ctx, time.Second)
		defer stopQuery()
		var amount int
		workloadErr = db.WithContext(query).Raw("SELECT amount FROM " + business + " WHERE id=901").Scan(&amount).Error
		if workloadErr != nil {
			return true
		}
		write := db.WithContext(query).Exec("UPDATE ONLY " + name + " SET last_error=last_error WHERE id=96001")
		workloadErr = write.Error
		if workloadErr == nil && (amount != 123 || write.RowsAffected != 1) {
			workloadErr = fmt.Errorf("unexpected fixture read/write result: amount=%d, written=%d", amount, write.RowsAffected)
		}
		workloadSamples++
		longestWorkloadSample = max(longestWorkloadSample, time.Since(started))
		return workloadErr != nil
	}, 90*time.Second, 50*time.Millisecond)
	require.NoError(t, workloadErr)
	require.GreaterOrEqual(t, workloadSamples, 2)
	require.Equal(t, "completed", final.State, "%+v", final)
	require.Equal(t, preview.PeakBytes, final.PeakBytes)
	require.Len(t, final.Steps, 1)
	require.Equal(t, repo.IncrementalMaintenanceTable, final.Steps[0].TableID)
	require.NotNil(t, final.After)
	beforeBytes, afterBytes := heapBytes(final.Before), heapBytes(*final.After)
	require.Positive(t, afterBytes)
	require.Less(t, afterBytes, beforeBytes)
	require.NotNil(t, final.Steps[0].Incremental)
	require.Equal(t, 5, final.Steps[0].Incremental.SlicesCompleted)
	require.Equal(t, beforeBytes-afterBytes, final.Steps[0].Incremental.HeapReclaimedBytes)
	batches := final.Steps[0].IncrementalBatches
	require.Len(t, batches, 5)
	require.Equal(t, 1, batches[3].Round)
	require.Equal(t, 2, batches[4].Round)
	require.Positive(t, batches[4].HeapReclaimedBytes)
	require.LessOrEqual(t, final.After.DatabaseBytes, input.TargetDatabaseBytes)
	require.GreaterOrEqual(t, batches[0].VacuumAttempts, 2, "the first slice must succeed by retrying truncation")
	require.Equal(t, 1, batches[0].TruncateLockWaits)
	require.Empty(t, batches[0].StopPhase)
	require.Greater(t, batches[0].ElapsedMS, int64(4500), "the first slice should include the failed truncation window")
	require.NotNil(t, batches[0].TableChangeBytes)
	require.Negative(t, *batches[0].TableChangeBytes)
	require.Equal(t, "target_reached", final.Steps[0].IncrementalStopReason)
	require.NotNil(t, final.FilesystemAvailableBefore)
	require.NotNil(t, final.FilesystemAvailableAfter)
	t.Logf("notification heap after %d slices: %d -> %d bytes", final.Steps[0].Incremental.SlicesCompleted, beforeBytes, afterBytes)
	t.Logf("%d concurrent read/write samples, longest %v; filesystem delta %d bytes", workloadSamples, longestWorkloadSample,
		*final.FilesystemAvailableAfter-*final.FilesystemAvailableBefore)
	require.Equal(t, expected, digest())
	require.Equal(t, auditBefore, maintenanceDataDigest(t, db, "capability_registry_invocation_traces"))
	require.Equal(t, businessBefore, maintenanceDataDigest(t, db, "orders"))
	var amount int
	require.NoError(t, db.Raw("SELECT amount FROM "+business+" WHERE id=901").Scan(&amount).Error)
	require.Equal(t, 123, amount)
}

func TestDatabaseMaintenanceNativePostgresIncrementalPreservesAuditOnPartialStop(t *testing.T) {
	s, _ := maintenanceNative(t)
	db := s.opts.DB
	ctx := controlRoot()
	var version string
	require.NoError(t, db.Raw("SHOW server_version").Scan(&version).Error)
	if !strings.HasPrefix(version, "16.") {
		t.Skip("incremental compaction requires PostgreSQL 16")
	}
	const auditID = "capability-invocation-traces"
	const auditName = "capability_registry_invocation_traces"
	name := pgx.Identifier{coremodel.PowerXSchema, auditName}.Sanitize()
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS pg_freespacemap").Error)
	require.NoError(t, db.Exec("DROP TABLE "+name+"; CREATE TABLE "+name+` (
 id bigint PRIMARY KEY, uuid uuid, created_at timestamptz, updated_at timestamptz, deleted_at timestamptz,
 tenant_uuid char(36) NOT NULL, trace_id varchar(128), caller_subject varchar(512), plugin_id varchar(128),
 capability_id varchar(256), protocol_used varchar(32), status varchar(32) NOT NULL, latency_ms bigint,
 fallback_used bool, event_published bool, event_publication_id uuid, request_payload jsonb, response_payload jsonb,
 error_summary text, record_class varchar(64) NOT NULL, completed_at timestamptz
) WITH (autovacuum_enabled=false)`).Error)
	require.NoError(t, db.Exec("INSERT INTO "+name+` (id,uuid,created_at,updated_at,tenant_uuid,trace_id,caller_subject,plugin_id,capability_id,
 protocol_used,status,latency_ms,fallback_used,event_published,request_payload,response_payload,record_class,completed_at)
 SELECT n,md5(n::text)::uuid,now(),now(),md5('tenant')::uuid::text,md5(n::text),'caller','plugin','capability','grpc',
 'completed',25,false,true,jsonb_build_object('body',repeat(md5(n::text),12)),NULL,
 CASE WHEN n=138000 THEN 'legacy_unclassified' ELSE 'capability_invocation_audit' END,now()
 FROM generate_series(1,160000) n`).Error)
	for i, columns := range []string{"uuid", "deleted_at", "tenant_uuid", "trace_id", "caller_subject", "plugin_id",
		"capability_id", "status", "event_publication_id", "tenant_uuid,status,completed_at"} {
		require.NoError(t, db.Exec(fmt.Sprintf("CREATE INDEX audit_test_%d ON %s (%s)", i, name, columns)).Error)
	}
	require.NoError(t, db.Exec("DELETE FROM "+name+" WHERE id<=96000").Error)
	require.NoError(t, db.Exec("VACUUM (TRUNCATE FALSE, INDEX_CLEANUP ON, PARALLEL 0) "+name).Error)
	businessName := pgx.Identifier{coremodel.PowerXSchema, "orders"}.Sanitize()
	require.NoError(t, db.Exec("CREATE TABLE "+businessName+" (id int PRIMARY KEY, amount int); INSERT INTO "+businessName+" VALUES(1,987)").Error)
	// Derive the reclaimable suffix before the run from the protected fixture row.
	var suffixPages int64
	require.NoError(t, db.Raw("SELECT pg_relation_size(?::regclass)/8192 - (ctid::text::point)[0]::bigint - 1 FROM ONLY "+name+" WHERE id=138000", name).Scan(&suffixPages).Error)
	require.Greater(t, suffixPages, int64(1024))
	require.Less(t, suffixPages, int64(2048))
	auditDigest := maintenanceDataDigest(t, db, auditName)
	businessDigest := maintenanceDataDigest(t, db, "orders")
	notificationDigest := maintenanceDataDigest(t, db, "capability_registry_event_publications")

	runCtx, stop := context.WithCancel(context.Background())
	s.StartControl(runCtx, nil)
	s.StartDatabaseMaintenance(runCtx, nil)
	t.Cleanup(func() {
		stop()
		<-s.maintenance.done
		<-s.opts.Control.Done()
	})
	preview, e := s.PreviewDatabaseMaintenance(ctx, DatabaseMaintenancePreview{Mode: repo.IncrementalMaintenanceMode, Tables: []string{auditID}})
	require.NoError(t, e)
	req := DatabaseMaintenanceRequest{RequestID: uuid.NewString(), PreviewID: preview.ID, Confirm: true, ExclusiveLockConfirmed: true}
	_, e = s.StartDatabaseMaintenanceRequest(ctx, req)
	require.NoError(t, e)
	var final DatabaseMaintenanceRun
	require.Eventually(t, func() bool {
		final, e = s.DatabaseMaintenanceRun(ctx, req.RequestID)
		return e == nil && (final.State == "failed" || final.State == "completed" || final.State == "unconfirmed" || final.State == "cancelled")
	}, 90*time.Second, 50*time.Millisecond)
	require.Equal(t, "failed", final.State, "%+v", final)
	require.Equal(t, "DATABASE_MAINTENANCE_INCREMENTAL_PROTECTED_TAIL", final.Problem)
	require.Len(t, final.Steps, 1)
	require.Equal(t, auditID, final.Steps[0].TableID)
	progress := final.Steps[0].Incremental
	require.NotNil(t, progress)
	require.Equal(t, 2, progress.SlicesCompleted)
	require.Equal(t, suffixPages*8192, progress.HeapReclaimedBytes)
	batches := final.Steps[0].IncrementalBatches
	require.Len(t, batches, 3)
	require.EqualValues(t, 1024, batches[0].TailPages)
	require.EqualValues(t, suffixPages-1024, batches[1].TailPages)
	for _, b := range batches[:2] {
		require.Empty(t, b.StopPhase)
		require.Equal(t, b.TailPages*8192, b.HeapReclaimedBytes)
	}
	require.Equal(t, "eligibility", batches[2].StopPhase)
	require.Zero(t, batches[2].VacuumAttempts)
	require.Nil(t, batches[2].TableChangeBytes)
	require.NotNil(t, final.After)
	require.Equal(t, auditDigest, maintenanceDataDigest(t, db, auditName))
	require.Equal(t, businessDigest, maintenanceDataDigest(t, db, "orders"))
	require.Equal(t, notificationDigest, maintenanceDataDigest(t, db, "capability_registry_event_publications"))
	t.Logf("audit heap reclaimed before protected tail: %d bytes", progress.HeapReclaimedBytes)
}
