package storagegov

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	mevent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, r.Execute(context.Background(), uuid.NewString(), "reuse", snapshot, []string{"capability-invocation-traces"}, func(_ string, x repo.MaintenanceSession) error { session = x; return nil }, func(string) error { return nil }))
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
		result <- r.Execute(context.Background(), uuid.NewString(), "compact", snapshot, []string{"capability-invocation-traces"}, func(_ string, x repo.MaintenanceSession) error { ready <- x; return nil }, func(string) error { return nil })
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
