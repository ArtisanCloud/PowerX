package storagegov

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	mevent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type maintenanceFixtureBackend struct {
	executed  atomic.Int32
	peakBytes atomic.Int64
	active    atomic.Bool
	revoked   atomic.Bool
	onExecute func(context.Context) error
	snapshot  repo.MaintenanceSnapshot
}

func (f *maintenanceFixtureBackend) Inspect(context.Context) (repo.MaintenanceSnapshot, error) {
	return f.snapshot, nil
}
func (f *maintenanceFixtureBackend) RootActive(context.Context, uint64) error {
	if f.revoked.Load() {
		return errors.New("revoked")
	}
	return nil
}
func (f *maintenanceFixtureBackend) SessionActive(_ context.Context, _ repo.MaintenanceSession, cancel bool) (bool, error) {
	active := f.active.Load()
	if cancel {
		f.active.Store(false)
	}
	return active, nil
}
func (f *maintenanceFixtureBackend) CheckIncremental(context.Context, repo.MaintenanceSnapshot, string) error {
	return nil
}
func (f *maintenanceFixtureBackend) IncrementalRecoveryReady(context.Context, *repo.IncrementalWALStop, int64) (bool, error) {
	return true, nil
}

func (f *maintenanceFixtureBackend) Execute(ctx context.Context, id, mode string, expected repo.MaintenanceSnapshot, ids []string, peakBytes int64, before func(string, repo.MaintenanceSession) error, after func(string) error, _ func(context.Context) error, _ func(repo.MaintenanceIncrementalReport) error, _ ...repo.MaintenanceIncrementalPlan) error {
	f.peakBytes.Store(peakBytes)
	f.active.Store(true)
	defer f.active.Store(false)
	for _, table := range ids {
		if e := before(table, repo.MaintenanceSession{PID: 99, StartedAt: time.Now().UTC(), Application: "px.dbmaint." + id}); e != nil {
			return e
		}
		f.executed.Add(1)
		if f.onExecute != nil {
			if e := f.onExecute(ctx); e != nil {
				return e
			}
		}
		if e := after(table); e != nil {
			return e
		}
	}
	return nil
}

type maintenanceFixtureBus struct {
	db            *gorm.DB
	renewError    atomic.Bool
	renewed       atomic.Int32
	completed     atomic.Int32
	register      func(context.Context, *gorm.DB, event_bus.TaskMessage) error
	registerError error
	dispatchError error
	acked         atomic.Int32
}

func (f *maintenanceFixtureBus) Apply(ctx context.Context, _ event_bus.StreamTaskDelivery, fn func(*gorm.DB) error) error {
	if f.renewError.Load() {
		return event_bus.ErrLeaseStale
	}
	return f.db.WithContext(ctx).Transaction(fn)
}
func (f *maintenanceFixtureBus) Complete(ctx context.Context, item event_bus.StreamTaskDelivery, fn func(*gorm.DB) error) error {
	if e := f.Apply(ctx, item, fn); e != nil {
		return e
	}
	f.completed.Add(1)
	return nil
}
func (f *maintenanceFixtureBus) Renew(context.Context, event_bus.StreamTaskDelivery) error {
	f.renewed.Add(1)
	if f.renewError.Load() {
		return event_bus.ErrLeaseStale
	}
	return nil
}

func (f *maintenanceFixtureBus) Register(ctx context.Context, tx *gorm.DB, m event_bus.TaskMessage) error {
	if f.registerError != nil {
		return f.registerError
	}
	return f.register(ctx, tx, m)
}
func (f *maintenanceFixtureBus) Enqueue(context.Context, event_bus.TaskMessage) error {
	return f.dispatchError
}
func (f *maintenanceFixtureBus) Recover(context.Context, string, string) error { return nil }
func (f *maintenanceFixtureBus) Dequeue(ctx context.Context, a, b, c, d string, n int64, wait time.Duration) ([]event_bus.StreamTaskDelivery, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (f *maintenanceFixtureBus) Ack(context.Context, event_bus.StreamTaskDelivery) error {
	f.acked.Add(1)
	return nil
}
func (f *maintenanceFixtureBus) Reject(context.Context, event_bus.StreamTaskDelivery, string, bool) error {
	return nil
}

func maintenanceFixture(t *testing.T, configure ...func(*budget.Config)) (*Service, *maintenanceFixtureBackend, *maintenanceFixtureBus) {
	t.Helper()
	old := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = old })
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, db.AutoMigrate(&m.ControlConfiguration{}, &m.ControlRevision{}, &m.CleanupRun{}, &mevent.TaskHistory{}))
	queue := taskqueue.NewStreamDriver(db, nil)
	scope := ControlScope{HostID: "owned-host", FilesystemID: "owned-disk", InstanceID: "owned-install"}
	ctrl, e := NewControlService(db, ControlOptions{Scope: scope, Queue: queue})
	require.NoError(t, e)
	root := t.TempDir()
	budgetConfig := budget.Config{Directory: filepath.Join(root, "runtime"), FilesystemPath: root, HostID: scope.HostID, FilesystemID: scope.FilesystemID, InstanceID: scope.InstanceID, MinFreeBytes: 1 << 20, CriticalFreeBytes: 1, MaxConcurrent: 1}
	for _, fn := range configure {
		fn(&budgetConfig)
	}
	coordinator, e := budget.New(budgetConfig)
	require.NoError(t, e)
	fake := &maintenanceFixtureBackend{snapshot: repo.MaintenanceSnapshot{DatabaseOID: 1, Fingerprint: strings.Repeat("a", 64), DatabaseBytes: 1 << 20, LocalDiskVerified: true, WALBudgetBytes: 1 << 30, RewriteSafe: true, Tables: []repo.MaintenanceTable{{ID: "capability-event-publications", OID: 1, TotalBytes: 8192, CanMaintain: true, IndexesValid: true, DefaultTablespace: true}, {ID: "capability-invocation-traces", OID: 2, TotalBytes: 8192, CanMaintain: true, IndexesValid: true, DefaultTablespace: true}}}}
	s, e := NewService(Options{DB: db, Control: ctrl, Budget: coordinator, FilesystemPath: root, Collector: &fixtureCollector{}, Identity: Identity{HostID: scope.HostID, FilesystemID: scope.FilesystemID, InstanceID: scope.InstanceID, Environment: "test"}, ControlWorkerEnabled: true, DatabaseRetentionAllowed: true, MaintenanceBackend: fake})
	require.NoError(t, e)
	bus := &maintenanceFixtureBus{register: queue.Register, db: db}
	s.maintenance.queue = bus
	s.maintenance.started.Store(true)
	return s, fake, bus
}
func maintenancePreviewAndQueue(t *testing.T, s *Service, mode string) DatabaseMaintenanceRun {
	t.Helper()
	p, e := s.PreviewDatabaseMaintenance(controlRoot(), DatabaseMaintenancePreview{Mode: mode, Tables: []string{"capability-invocation-traces"}})
	require.NoError(t, e)
	r, e := s.StartDatabaseMaintenanceRequest(controlRoot(), DatabaseMaintenanceRequest{RequestID: uuid.NewString(), PreviewID: p.ID, Confirm: true, ExclusiveLockConfirmed: mode != "reuse"})
	require.NoError(t, e)
	require.Equal(t, "queued", r.State)
	return r
}
func maintenanceDelivery(s *Service, r DatabaseMaintenanceRun) event_bus.StreamTaskDelivery {
	return event_bus.StreamTaskDelivery{Message: s.maintenanceMessage(r)}
}

func TestDatabaseMaintenanceWALStopPersistsMeasuredLimit(t *testing.T) {
	s, backend, _ := maintenanceFixture(t)
	stop := &repo.IncrementalWALStop{
		Phase: "vacuum", Metric: "wal_file_bytes", ObservedBytes: 864 << 20, LimitBytes: 848 << 20,
	}
	backend.onExecute = func(context.Context) error { return stop }
	run := maintenancePreviewAndQueue(t, s, repo.IncrementalMaintenanceMode)

	require.NoError(t, s.applyDatabaseMaintenance(context.Background(), maintenanceDelivery(s, run)))
	finished, e := s.DatabaseMaintenanceRun(controlRoot(), run.ID)
	require.NoError(t, e)
	require.Equal(t, "failed", finished.State)
	require.Equal(t, "DATABASE_MAINTENANCE_INCREMENTAL_WAL_BUDGET", finished.Problem)
	require.Equal(t, stop, finished.WALStop)
	require.Equal(t, run.PeakBytes, backend.peakBytes.Load())
	status, e := s.opts.Budget.Status(context.Background())
	require.NoError(t, e)
	require.Empty(t, status.Active)
}

func TestDatabaseMaintenanceRecoversWALWithinOneApprovedTask(t *testing.T) {
	s, backend, bus := maintenanceFixture(t)
	stop := &repo.IncrementalWALStop{
		Phase: "vacuum", Metric: "wal_file_bytes", ObservedBytes: 848 << 20, LimitBytes: 848 << 20,
	}
	backend.onExecute = func(context.Context) error {
		if backend.executed.Load() == 1 {
			return stop
		}
		return nil
	}
	actor := controlRoot()
	preview, err := s.PreviewDatabaseMaintenance(actor, DatabaseMaintenancePreview{
		Mode:          repo.IncrementalMaintenanceMode,
		Tables:        []string{repo.IncrementalMaintenanceTable},
		WindowMinutes: 12,
	})
	require.NoError(t, err)
	request := DatabaseMaintenanceRequest{RequestID: uuid.NewString(), PreviewID: preview.ID, Confirm: true, ExclusiveLockConfirmed: true}
	run, err := s.StartDatabaseMaintenanceRequest(actor, request)
	require.NoError(t, err)
	ctx, stopTest := context.WithTimeout(context.Background(), 40*time.Second)
	defer stopTest()
	done := make(chan error, 1)
	go func() {
		done <- s.applyDatabaseMaintenance(ctx, maintenanceDelivery(s, run))
	}()
	require.Eventually(t, func() bool {
		waiting, err := s.DatabaseMaintenanceRun(actor, run.ID)
		return err == nil && waiting.State == "running" && waiting.Wait != nil &&
			waiting.Wait.Code == "DATABASE_MAINTENANCE_INCREMENTAL_WAL_BUDGET" &&
			waiting.Wait.WAL != nil && *waiting.Wait.WAL == *stop && waiting.Session == nil
	}, 3*time.Second, 20*time.Millisecond)
	require.False(t, backend.active.Load(), "confirmed SQL session must be gone while waiting")
	require.EqualValues(t, 1, backend.executed.Load())
	require.Zero(t, bus.completed.Load())
	require.NoError(t, <-done)
	final, err := s.DatabaseMaintenanceRun(actor, run.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", final.State)
	require.Nil(t, final.Wait)
	require.Len(t, final.Steps, 1)
	require.Equal(t, "completed", final.Steps[0].State)
	require.EqualValues(t, 2, backend.executed.Load())
	require.EqualValues(t, 1, bus.completed.Load())
	require.EqualValues(t, 1, bus.acked.Load())
	status, err := s.opts.Budget.Status(context.Background())
	require.NoError(t, err)
	require.Empty(t, status.Active)
}

func TestDatabaseMaintenanceLeaseLossStopsCommandAndNeverReplaysSQL(t *testing.T) {
	s, f, bus := maintenanceFixture(t)
	r := maintenancePreviewAndQueue(t, s, "reuse")
	f.onExecute = func(ctx context.Context) error {
		bus.renewError.Store(true)
		<-ctx.Done()
		return ctx.Err()
	}
	require.Error(t, s.applyDatabaseMaintenance(context.Background(), maintenanceDelivery(s, r)))
	require.Positive(t, bus.renewed.Load())
	require.Zero(t, bus.completed.Load(), "stale lease cannot accept a terminal result")
	got, e := s.DatabaseMaintenanceRun(controlRoot(), r.ID)
	require.NoError(t, e)
	require.Equal(t, "running", got.State)
	bus.renewError.Store(false)
	require.NoError(t, s.applyDatabaseMaintenance(context.Background(), maintenanceDelivery(s, r)))
	got, e = s.DatabaseMaintenanceRun(controlRoot(), r.ID)
	require.NoError(t, e)
	require.Equal(t, "unconfirmed", got.State)
	require.EqualValues(t, 1, f.executed.Load())
	require.EqualValues(t, 1, bus.completed.Load())
}

func TestDatabaseMaintenanceMetadataRetentionPreservesUnknownIntent(t *testing.T) {
	s, _, _ := maintenanceFixture(t)
	p, e := s.PreviewDatabaseMaintenance(controlRoot(), DatabaseMaintenancePreview{Mode: "reuse", Tables: []string{"capability-invocation-traces"}})
	require.NoError(t, e)
	ledger := s.maintenanceLedger()
	unknown := p
	unknown.ID, unknown.State = uuid.NewString(), "unconfirmed"
	require.NoError(t, ledger.Create(context.Background(), s.opts.DB, maintenanceRow(unknown)))
	consumed := p
	consumed.ID, consumed.State = uuid.NewString(), "consumed"
	require.NoError(t, ledger.Create(context.Background(), s.opts.DB, maintenanceRow(consumed)))
	require.NoError(t, s.opts.DB.Model(&m.CleanupRun{}).Where("run_id IN ?", []string{unknown.ID, consumed.ID}).Update("updated_at", time.Now().Add(-31*24*time.Hour)).Error)
	_, e = s.PreviewDatabaseMaintenance(controlRoot(), DatabaseMaintenancePreview{Mode: "reuse", Tables: p.Tables})
	require.NoError(t, e)
	_, e = ledger.Get(context.Background(), s.opts.DB, consumed.ID)
	require.ErrorIs(t, e, gorm.ErrRecordNotFound)
	_, e = ledger.Get(context.Background(), s.opts.DB, unknown.ID)
	require.NoError(t, e)
	rows, e := ledger.List(context.Background())
	require.NoError(t, e)
	require.Len(t, rows, 1)
	require.Equal(t, unknown.ID, rows[0].RunID)
}

func TestDatabaseMaintenanceRequiresRootFixedTablesAndPreview(t *testing.T) {
	s, _, _ := maintenanceFixture(t)
	_, e := s.PreviewDatabaseMaintenance(context.Background(), DatabaseMaintenancePreview{Mode: "reuse", Tables: []string{"capability-invocation-traces"}})
	require.ErrorIs(t, e, ErrRootRequired)
	for _, input := range []DatabaseMaintenancePreview{{Mode: "sql", Tables: []string{"capability-invocation-traces"}}, {Mode: "compact", Tables: []string{"iam_user"}}, {Mode: "reuse", Tables: []string{"capability-invocation-traces", "capability-invocation-traces"}}} {
		_, e = s.PreviewDatabaseMaintenance(controlRoot(), input)
		require.Error(t, e)
	}
	_, e = s.StartDatabaseMaintenanceRequest(controlRoot(), DatabaseMaintenanceRequest{RequestID: uuid.NewString(), Confirm: true})
	require.Equal(t, "DATABASE_MAINTENANCE_PREVIEW_REQUIRED", dto.CodeOf(e))
}

func TestDatabaseMaintenanceVacuumPreviewAndWorkerUseSameCriticalFloor(t *testing.T) {
	s, f, _ := maintenanceFixture(t, func(c *budget.Config) {
		var st unix.Statfs_t
		require.NoError(t, unix.Statfs(c.FilesystemPath, &st))
		available := int64(st.Bavail) * int64(st.Bsize)
		c.MinFreeBytes = available + (1 << 30)
		c.CriticalFreeBytes = available / 2
	})
	ctx := context.Background()
	_, e := s.PreviewDatabaseMaintenance(controlRoot(), DatabaseMaintenancePreview{Mode: "compact", Tables: []string{"capability-invocation-traces"}})
	require.Equal(t, "DATABASE_MAINTENANCE_SPACE_INSUFFICIENT", dto.CodeOf(e))
	r := maintenancePreviewAndQueue(t, s, "reuse")
	require.NoError(t, s.applyDatabaseMaintenance(ctx, maintenanceDelivery(s, r)))
	require.EqualValues(t, 1, f.executed.Load())
}

func TestDatabaseMaintenanceRunningVacuumHonorsCriticalAndOtherReservations(t *testing.T) {
	snap := budget.Snapshot{AvailableBytes: 15 << 30, CriticalFreeBytes: 10 << 30, MinFreeBytes: 20 << 30, ReservedBytes: 3 << 30, Active: []budget.Reservation{{ID: "own", PeakBytes: 1 << 30}, {ID: "other", PeakBytes: 2 << 30}}}
	require.NoError(t, maintenanceBudgetSafety(snap, "reuse", "own"))
	snap.AvailableBytes = 11 << 30
	require.Equal(t, "DATABASE_MAINTENANCE_SPACE_INSUFFICIENT", dto.CodeOf(maintenanceBudgetSafety(snap, "reuse", "own")))
	require.Equal(t, "DATABASE_MAINTENANCE_LEASE_UNCONFIRMED", dto.CodeOf(maintenanceBudgetSafety(snap, "reuse", "missing")))
}

func TestDatabaseMaintenanceIncrementalUsesBoundedReservationAndRuntimeMargin(t *testing.T) {
	s, _, _ := maintenanceFixture(t, func(c *budget.Config) {
		var st unix.Statfs_t
		require.NoError(t, unix.Statfs(c.FilesystemPath, &st))
		available := int64(st.Bavail) * int64(st.Bsize)
		c.MinFreeBytes = available + 1<<30
		c.CriticalFreeBytes = available - 620<<20
	})
	input := DatabaseMaintenancePreview{Mode: repo.IncrementalMaintenanceMode, Tables: []string{repo.IncrementalMaintenanceTable}}
	preview, e := s.PreviewDatabaseMaintenance(controlRoot(), input)
	require.NoError(t, e)
	require.EqualValues(t, 512<<20, preview.PeakBytes)
	snapshot := budget.Snapshot{
		CriticalFreeBytes: 10 << 30, AvailableBytes: (10 << 30) + (96 << 20),
		Active:        []budget.Reservation{{ID: "own", PeakBytes: preview.PeakBytes}, {ID: "other", PeakBytes: 32 << 20}},
		ReservedBytes: preview.PeakBytes + 32<<20,
	}
	require.NoError(t, maintenanceBudgetSafety(snapshot, input.Mode, "own"))
	snapshot.AvailableBytes--
	require.Equal(t, "DATABASE_MAINTENANCE_SPACE_INSUFFICIENT", dto.CodeOf(maintenanceBudgetSafety(snapshot, input.Mode, "own")))
}

func TestDatabaseMaintenanceIncrementalPeakUsesAvailableDiskAndWALSetting(t *testing.T) {
	_, backend, _ := maintenanceFixture(t)
	snapshot := backend.snapshot
	tables := []string{repo.IncrementalMaintenanceTable}
	for _, test := range []struct {
		name, problem     string
		room, other, peak int64
	}{
		{name: "prod observation", room: 599, peak: 512},
		{name: "counts other reservations", room: 599, other: 128, peak: 384},
		{name: "legacy minimum", room: 256, peak: 256},
		{name: "below minimum", room: 255, problem: "DATABASE_MAINTENANCE_SPACE_INSUFFICIENT"},
		{name: "bounded by planned WAL growth", room: 4096, peak: 1152},
	} {
		t.Run(test.name, func(t *testing.T) {
			const floor int64 = 10 << 30
			peak, e := maintenancePeak(repo.IncrementalMaintenanceMode, snapshot, tables, budget.Snapshot{
				CriticalFreeBytes: floor,
				AvailableBytes:    floor + test.room<<20,
				ReservedBytes:     test.other << 20,
			})
			if test.problem != "" {
				require.Equal(t, test.problem, dto.CodeOf(e))
				return
			}
			require.NoError(t, e)
			require.Equal(t, test.peak<<20, peak)
		})
	}
}
func TestDatabaseMaintenanceIdempotencyRegistrationAndSingleActiveJob(t *testing.T) {
	s, f, bus := maintenanceFixture(t)
	bus.dispatchError = errors.New("offline")
	r := maintenancePreviewAndQueue(t, s, "reuse")
	again, e := s.StartDatabaseMaintenanceRequest(controlRoot(), DatabaseMaintenanceRequest{RequestID: r.ID, PreviewID: r.PreviewID, Confirm: true})
	require.NoError(t, e)
	require.Equal(t, r.ID, again.ID)
	_, e = s.StartDatabaseMaintenanceRequest(controlRoot(), DatabaseMaintenanceRequest{RequestID: r.ID, PreviewID: uuid.NewString(), Confirm: true})
	require.Equal(t, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	p, e := s.PreviewDatabaseMaintenance(controlRoot(), DatabaseMaintenancePreview{Mode: "reuse", Tables: r.Tables})
	require.NoError(t, e)
	_, e = s.StartDatabaseMaintenanceRequest(controlRoot(), DatabaseMaintenanceRequest{RequestID: uuid.NewString(), PreviewID: p.ID, Confirm: true})
	require.Equal(t, "DATABASE_MAINTENANCE_BUSY", dto.CodeOf(e))
	require.NoError(t, s.applyDatabaseMaintenance(context.Background(), maintenanceDelivery(s, r)))
	require.EqualValues(t, 1, f.executed.Load())
	require.NoError(t, s.applyDatabaseMaintenance(context.Background(), maintenanceDelivery(s, r)))
	require.EqualValues(t, 1, f.executed.Load())
	got, e := s.DatabaseMaintenanceRun(controlRoot(), r.ID)
	require.NoError(t, e)
	require.Equal(t, "completed", got.State)
	require.NotNil(t, got.After)
	status, e := s.opts.Budget.Status(context.Background())
	require.NoError(t, e)
	require.Empty(t, status.Active)
}
func TestDatabaseMaintenanceRegistrationFailureRollsBackPreviewConsumption(t *testing.T) {
	s, _, bus := maintenanceFixture(t)
	p, e := s.PreviewDatabaseMaintenance(controlRoot(), DatabaseMaintenancePreview{Mode: "reuse", Tables: []string{"capability-invocation-traces"}})
	require.NoError(t, e)
	bus.registerError = errors.New("registry failed")
	_, e = s.StartDatabaseMaintenanceRequest(controlRoot(), DatabaseMaintenanceRequest{RequestID: uuid.NewString(), PreviewID: p.ID, Confirm: true})
	require.Error(t, e)
	v, e := s.DatabaseMaintenanceRun(controlRoot(), p.ID)
	require.NoError(t, e)
	require.Equal(t, "preview", v.State)
	_, e = s.maintenanceLedger().Active(context.Background(), s.opts.DB)
	require.ErrorIs(t, e, gorm.ErrRecordNotFound)
}
func TestDatabaseMaintenancePreviewExpiryActorAndExclusiveLockConsent(t *testing.T) {
	s, _, _ := maintenanceFixture(t)
	p, e := s.PreviewDatabaseMaintenance(controlRoot(), DatabaseMaintenancePreview{Mode: "compact", Tables: []string{"capability-invocation-traces"}})
	require.NoError(t, e)
	_, e = s.StartDatabaseMaintenanceRequest(controlRoot(), DatabaseMaintenanceRequest{RequestID: uuid.NewString(), PreviewID: p.ID, Confirm: true})
	require.Equal(t, "DATABASE_MAINTENANCE_LOCK_CONFIRMATION_REQUIRED", dto.CodeOf(e))
	other := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 8, Scope: "access"})
	_, e = s.StartDatabaseMaintenanceRequest(other, DatabaseMaintenanceRequest{RequestID: uuid.NewString(), PreviewID: p.ID, Confirm: true, ExclusiveLockConfirmed: true})
	require.Equal(t, "DATABASE_MAINTENANCE_PREVIEW_EXPIRED", dto.CodeOf(e))
	s.opts.Now = func() time.Time { return p.At.Add(6 * time.Minute) }
	_, e = s.StartDatabaseMaintenanceRequest(controlRoot(), DatabaseMaintenanceRequest{RequestID: uuid.NewString(), PreviewID: p.ID, Confirm: true, ExclusiveLockConfirmed: true})
	require.Equal(t, "DATABASE_MAINTENANCE_PREVIEW_EXPIRED", dto.CodeOf(e))
}
func TestDatabaseMaintenanceUnknownCommandCannotBeReplayedAndRequiresReview(t *testing.T) {
	s, f, _ := maintenanceFixture(t)
	f.onExecute = func(context.Context) error { return errors.New("connection lost after command") }
	r := maintenancePreviewAndQueue(t, s, "compact")
	require.NoError(t, s.applyDatabaseMaintenance(context.Background(), maintenanceDelivery(s, r)))
	got, e := s.DatabaseMaintenanceRun(controlRoot(), r.ID)
	require.NoError(t, e)
	require.Equal(t, "unconfirmed", got.State)
	require.NoError(t, s.applyDatabaseMaintenance(context.Background(), maintenanceDelivery(s, r)))
	require.EqualValues(t, 1, f.executed.Load())
	f.active.Store(true)
	_, e = s.ReviewDatabaseMaintenance(controlRoot(), r.ID)
	require.Error(t, e)
	f.active.Store(false)
	got, e = s.ReviewDatabaseMaintenance(controlRoot(), r.ID)
	require.NoError(t, e)
	require.Equal(t, "reviewed", got.State)
	status, e := s.opts.Budget.Status(context.Background())
	require.NoError(t, e)
	require.Empty(t, status.Active)
}
func TestDatabaseMaintenanceQueuedCancellationAndLateCancellation(t *testing.T) {
	s, f, _ := maintenanceFixture(t)
	r := maintenancePreviewAndQueue(t, s, "reuse")
	got, e := s.CancelDatabaseMaintenance(controlRoot(), r.ID)
	require.NoError(t, e)
	require.Equal(t, "cancelled", got.State)
	require.NoError(t, s.applyDatabaseMaintenance(context.Background(), maintenanceDelivery(s, r)))
	require.Zero(t, f.executed.Load())
	f.onExecute = func(context.Context) error {
		_, e := s.CancelDatabaseMaintenance(controlRoot(), r2ForFixture(s, t))
		return e
	}
	r = maintenancePreviewAndQueue(t, s, "reuse")
	require.NoError(t, s.applyDatabaseMaintenance(context.Background(), maintenanceDelivery(s, r)))
	got, e = s.DatabaseMaintenanceRun(controlRoot(), r.ID)
	require.NoError(t, e)
	require.Equal(t, "completed", got.State, "late cancellation must not hide an already completed command")
}
func r2ForFixture(s *Service, t *testing.T) string {
	row, e := s.maintenanceLedger().Active(context.Background(), s.opts.DB)
	require.NoError(t, e)
	return row.RunID
}
func TestDatabaseMaintenancePauseCancelsRunningStep(t *testing.T) {
	s, f, _ := maintenanceFixture(t)
	r := maintenancePreviewAndQueue(t, s, "compact")
	var paused atomic.Bool
	s.opts.Control.executionFence = func(context.Context, *gorm.DB, *m.ControlConfiguration) error {
		if paused.Load() {
			return controlError(423, "STORAGE_EXECUTION_PAUSED")
		}
		return nil
	}
	f.onExecute = func(ctx context.Context) error { paused.Store(true); <-ctx.Done(); return ctx.Err() }
	require.NoError(t, s.applyDatabaseMaintenance(context.Background(), maintenanceDelivery(s, r)))
	got, e := s.DatabaseMaintenanceRun(controlRoot(), r.ID)
	require.NoError(t, e)
	require.Equal(t, "cancelled", got.State)
	require.Equal(t, "STORAGE_EXECUTION_PAUSED", got.Problem)
}
func TestDatabaseMaintenanceTargetChangeAndRevokedActorNeverExecute(t *testing.T) {
	for _, reason := range []string{"target", "root"} {
		t.Run(reason, func(t *testing.T) {
			s, f, _ := maintenanceFixture(t)
			r := maintenancePreviewAndQueue(t, s, "reuse")
			if reason == "target" {
				f.snapshot.Fingerprint = strings.Repeat("b", 64)
			} else {
				f.revoked.Store(true)
			}
			require.NoError(t, s.applyDatabaseMaintenance(context.Background(), maintenanceDelivery(s, r)))
			f.revoked.Store(false)
			got, e := s.DatabaseMaintenanceRun(controlRoot(), r.ID)
			require.NoError(t, e)
			require.Equal(t, "failed", got.State)
			require.Zero(t, f.executed.Load())
		})
	}
}

func TestDatabaseMaintenanceUnregisteredCancellationTombstonePreventsLateStart(t *testing.T) {
	s, f, _ := maintenanceFixture(t)
	preview, e := s.PreviewDatabaseMaintenance(controlRoot(), DatabaseMaintenancePreview{Mode: "compact", Tables: []string{"capability-invocation-traces"}})
	require.NoError(t, e)
	id := uuid.NewString()
	cancelled, e := s.CancelDatabaseMaintenance(controlRoot(), id, preview.ID)
	require.NoError(t, e)
	require.Equal(t, "cancelled", cancelled.State)
	late, e := s.StartDatabaseMaintenanceRequest(controlRoot(), DatabaseMaintenanceRequest{RequestID: id, PreviewID: preview.ID, Confirm: true, ExclusiveLockConfirmed: true})
	require.NoError(t, e)
	require.Equal(t, "cancelled", late.State)
	require.NoError(t, s.applyDatabaseMaintenance(context.Background(), maintenanceDelivery(s, late)))
	require.Zero(t, f.executed.Load())
}
func TestDatabaseMaintenanceUnknownSessionCannotTargetAnotherJob(t *testing.T) {
	s, _, _ := maintenanceFixture(t)
	r := maintenancePreviewAndQueue(t, s, "reuse")
	_, e := s.maintenanceChange(context.Background(), r.ID, func(x *DatabaseMaintenanceRun) error {
		x.State = "unconfirmed"
		x.Session = &repo.MaintenanceSession{PID: 9, StartedAt: time.Now(), Application: "px.dbmaint." + uuid.NewString()}
		return nil
	})
	require.NoError(t, e)
	_, e = s.CancelDatabaseMaintenance(controlRoot(), r.ID)
	require.Error(t, e)
}
func TestDatabaseMaintenanceNoDiskBindingOrProtectedWALCannotCompact(t *testing.T) {
	for _, reason := range []string{"directory", "wal"} {
		t.Run(reason, func(t *testing.T) {
			s, f, _ := maintenanceFixture(t)
			if reason == "directory" {
				f.snapshot.LocalDiskVerified = false
			} else {
				f.snapshot.RewriteSafe = false
			}
			_, e := s.PreviewDatabaseMaintenance(controlRoot(), DatabaseMaintenancePreview{Mode: "compact", Tables: []string{"capability-invocation-traces"}})
			require.Error(t, e)
			_, e = s.PreviewDatabaseMaintenance(controlRoot(), DatabaseMaintenancePreview{Mode: "reuse", Tables: []string{"capability-invocation-traces"}})
			require.NoError(t, e)
		})
	}
}
