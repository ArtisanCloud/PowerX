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
	require.NoError(t, db.Raw("SELECT md5(string_agg(id::text||':'||payload,',' ORDER BY id)) FROM "+pgx.Identifier{coremodel.PowerXSchema, name}.Sanitize()).Scan(&value).Error)
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
	require.NoError(t, r.Execute(context.Background(), uuid.NewString(), "reuse", snapshot, []string{"capability-invocation-traces"}, func(_ string, x repo.MaintenanceSession) error { session = x; return nil }, func(string) error { return nil }, nil))
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
		result <- r.Execute(context.Background(), uuid.NewString(), "compact", snapshot, []string{"capability-invocation-traces"}, func(_ string, x repo.MaintenanceSession) error { ready <- x; return nil }, func(string) error { return nil }, nil)
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
 'quarantined',md5(n::text),CASE WHEN n=80000 THEN 'legacy_unclassified' ELSE 'capability_invocation_notification' END,now()
FROM generate_series(1,80000) n`).Error)
	for i, columns := range []string{"uuid", "trace_id", "status", "topic", "deleted_at", "tenant_uuid", "tenant_uuid,status,terminal_at"} {
		require.NoError(t, db.Exec(fmt.Sprintf("CREATE INDEX notification_test_%d ON %s (%s)", i, name, columns)).Error)
	}
	require.NoError(t, db.Exec("DELETE FROM "+name+" WHERE id<=47000").Error)
	require.NoError(t, db.Exec("VACUUM (TRUNCATE FALSE, INDEX_CLEANUP ON, PARALLEL 0) "+name).Error)
	require.NoError(t, db.Exec("CREATE TABLE "+business+" (id int PRIMARY KEY, amount int); INSERT INTO "+business+" VALUES(901,123)").Error)

	var version string
	require.NoError(t, db.Raw("SHOW server_version").Scan(&version).Error)
	if !strings.HasPrefix(version, "16.") {
		t.Skip("incremental compaction requires PostgreSQL 16")
	}
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS pg_freespacemap").Error)
	businessBefore := maintenanceDataDigest(t, db, "capability_registry_invocation_traces")
	digest := func() string {
		var out string
		require.NoError(t, db.Raw("SELECT md5(string_agg(md5(entry::text),'' ORDER BY id)) FROM "+name+" entry").Scan(&out).Error)
		return out
	}
	protectedDigest := digest()
	input := DatabaseMaintenancePreview{Mode: repo.IncrementalMaintenanceMode, Tables: []string{repo.IncrementalMaintenanceTable}}
	for _, ids := range [][]string{{"orders"}, {"capability-invocation-traces"}, {repo.IncrementalMaintenanceTable, "capability-invocation-traces"}} {
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
	var previous int64
	for batch := 1; batch <= 2; batch++ {
		preview, e := s.PreviewDatabaseMaintenance(ctx, input)
		require.NoError(t, e)
		require.EqualValues(t, 256<<20, preview.PeakBytes)
		req := DatabaseMaintenanceRequest{RequestID: uuid.NewString(), PreviewID: preview.ID, Confirm: true}
		_, e = s.StartDatabaseMaintenanceRequest(ctx, req)
		require.Equal(t, "DATABASE_MAINTENANCE_LOCK_CONFIRMATION_REQUIRED", dto.CodeOf(e))
		req.ExclusiveLockConfirmed = true
		_, e = s.StartDatabaseMaintenanceRequest(ctx, req)
		require.NoError(t, e)
		var final DatabaseMaintenanceRun
		require.Eventually(t, func() bool {
			final, e = s.DatabaseMaintenanceRun(ctx, req.RequestID)
			return e == nil && (final.State == "completed" || final.State == "failed" || final.State == "unconfirmed" || final.State == "cancelled")
		}, 30*time.Second, 50*time.Millisecond)
		require.Equal(t, "completed", final.State, "%+v", final)
		require.Len(t, final.Steps, 1)
		require.Equal(t, repo.IncrementalMaintenanceTable, final.Steps[0].TableID)
		require.NotNil(t, final.After)
		beforeBytes, afterBytes := heapBytes(final.Before), heapBytes(*final.After)
		if batch > 1 {
			require.Equal(t, previous, beforeBytes)
		}
		require.Positive(t, afterBytes)
		require.Less(t, afterBytes, beforeBytes)
		previous = afterBytes
		t.Logf("batch %d notification heap: %d -> %d bytes", batch, beforeBytes, afterBytes)
		require.Equal(t, expected, digest())
		require.Equal(t, businessBefore, maintenanceDataDigest(t, db, "capability_registry_invocation_traces"))
	}
	var amount int
	require.NoError(t, db.Raw("SELECT amount FROM "+business+" WHERE id=901").Scan(&amount).Error)
	require.Equal(t, 123, amount)
}
