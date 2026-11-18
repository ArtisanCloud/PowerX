package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"

	storagebudget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const maintenanceTenant = "platform:storage-maintenance"
const maintenanceTopic = "powerx.storage.database.maintenance.v1"

func newDatabaseMaintenance(s *Service) *databaseMaintenance {
	x := &databaseMaintenance{backend: s.opts.MaintenanceBackend, done: make(chan struct{}), wake: make(chan struct{}, 1)}
	if x.backend == nil && s.opts.DB != nil {
		x.backend = repo.NewDatabaseMaintenanceRepository(s.opts.DB, s.opts.MaintenanceDataDirectory, s.opts.FilesystemPath)
	}
	if s.opts.Control != nil && s.opts.Control.opts.Queue != nil {
		x.queue = s.opts.Control.opts.Queue
	}
	return x
}
func (s *Service) maintenanceLedger() *repo.MaintenanceRunRepository {
	return repo.NewMaintenanceRunRepository(s.opts.DB, s.opts.Control.opts.Scope.ID())
}
func (s *Service) maintenanceSubscriber() string {
	return "core.storage.maintenance." + s.opts.Control.opts.Scope.ID()
}
func (s *Service) maintenanceReady(ctx context.Context) error {
	if s.maintenance == nil || s.maintenance.backend == nil || s.opts.DB == nil || s.opts.Control == nil || s.opts.Budget == nil || s.maintenance.queue == nil || !s.opts.DatabaseRetentionAllowed {
		return controlError(503, "DATABASE_MAINTENANCE_UNAVAILABLE")
	}
	return s.opts.Control.CheckReady(ctx)
}
func (s *Service) maintenanceRootActive(ctx context.Context) error {
	if e := s.maintenance.backend.RootActive(ctx, rootActor(ctx)); e != nil {
		return controlError(403, "DATABASE_MAINTENANCE_ROOT_REVOKED")
	}
	return nil
}
func maintenanceError(e error) error {
	if e == nil {
		return nil
	}
	if dto.CodeOf(e) != "" {
		return e
	}
	code := "DATABASE_MAINTENANCE_UNAVAILABLE"
	switch {
	case errors.Is(e, repo.ErrMaintenanceTarget):
		code = "DATABASE_MAINTENANCE_TARGET_CHANGED"
	case errors.Is(e, repo.ErrMaintenanceDisk):
		code = "DATABASE_MAINTENANCE_DISK_UNVERIFIED"
	case errors.Is(e, repo.ErrMaintenanceIndexes):
		code = "DATABASE_MAINTENANCE_INDEXES_INVALID"
	case errors.Is(e, repo.ErrMaintenanceBusy):
		code = "DATABASE_MAINTENANCE_BUSY"
	case errors.Is(e, repo.ErrIncrementalUnsupported):
		code = "DATABASE_MAINTENANCE_INCREMENTAL_UNSUPPORTED"
	case errors.Is(e, repo.ErrIncrementalFSM):
		code = "DATABASE_MAINTENANCE_INCREMENTAL_FSM_REQUIRED"
	case errors.Is(e, repo.ErrIncrementalCapacity):
		code = "DATABASE_MAINTENANCE_INCREMENTAL_NO_HEADROOM"
	case errors.Is(e, repo.ErrIncrementalProtected):
		code = "DATABASE_MAINTENANCE_INCREMENTAL_PROTECTED_TAIL"
	case errors.Is(e, repo.ErrIncrementalLimit):
		code = "DATABASE_MAINTENANCE_INCREMENTAL_LIMIT"
	case errors.Is(e, repo.ErrIncrementalNoReclaim):
		code = "DATABASE_MAINTENANCE_INCREMENTAL_NO_RECLAIM"
	case errors.Is(e, repo.ErrRevisionConflict):
		code = "DATABASE_MAINTENANCE_CONFLICT"
	}
	return controlError(409, code)
}
func (s *Service) maintenanceGateTx(ctx context.Context, tx *gorm.DB, fn func() error) error {
	return s.opts.Control.repo.WithExecutionTx(ctx, tx, s.opts.Control.opts.Scope.ID(), func(row *m.ControlConfiguration) error {
		if s.opts.Control.executionFence != nil {
			if e := s.opts.Control.executionFence(ctx, tx, row); e != nil {
				return e
			}
		}
		return fn()
	})
}
func (s *Service) maintenanceGate(ctx context.Context) error {
	return s.withRecoveryGuard(ctx, func() error {
		return s.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.maintenanceGateTx(ctx, tx, func() error { return nil }) })
	})
}
func decodeMaintenance(row *m.CleanupRun, scope string) (DatabaseMaintenanceRun, error) {
	var out DatabaseMaintenanceRun
	if row == nil || json.Unmarshal(row.Document, &out) != nil || out.ID != row.RunID || out.Version != row.Version || out.State != row.State || out.Actor != row.ActorID || out.ScopeID != scope || !validTargetUUID(out.ID) || !repo.ValidMaintenanceRequest(out.Mode, out.Tables) || out.Version < 1 || out.Before.Fingerprint == "" || out.PeakBytes < 0 {
		return out, ErrStateUnavailable
	}
	if out.Session != nil && (out.Session.PID <= 0 || out.Session.StartedAt.IsZero() || out.Session.Application != "px.dbmaint."+out.ID) {
		return out, ErrStateUnavailable
	}
	switch out.State {
	case "preview", "consumed", "queued", "running", "cancel_requested", "cancelled", "completed", "failed", "unconfirmed", "reviewed":
		return out, nil
	}
	return out, ErrStateUnavailable
}
func maintenanceRow(run DatabaseMaintenanceRun) *m.CleanupRun {
	raw, _ := json.Marshal(run)
	expiry := run.At.Add(30 * 24 * time.Hour)
	if run.ExpiresAt != nil {
		expiry = *run.ExpiresAt
	}
	return &m.CleanupRun{RunID: run.ID, Version: run.Version, ActorID: run.Actor, State: run.State, Document: datatypes.JSON(raw), ExpiresAt: expiry}
}
func (s *Service) maintenanceChange(ctx context.Context, id string, fn func(*DatabaseMaintenanceRun) error) (DatabaseMaintenanceRun, error) {
	var out DatabaseMaintenanceRun
	e := s.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e error
		out, e = s.maintenanceChangeTx(ctx, tx, id, fn)
		return e
	})
	if e == nil {
		s.PublishChange("database_maintenance_changed")
	}
	return out, maintenanceError(e)
}

func (s *Service) maintenanceChangeTx(ctx context.Context, tx *gorm.DB, id string, fn func(*DatabaseMaintenanceRun) error) (DatabaseMaintenanceRun, error) {
	var out DatabaseMaintenanceRun
	ledger := s.maintenanceLedger()
	if e := ledger.Lock(ctx, tx); e != nil {
		return out, e
	}
	row, e := ledger.Get(ctx, tx, id)
	if e != nil {
		return out, e
	}
	out, e = decodeMaintenance(row, s.opts.Control.opts.Scope.ID())
	if e != nil {
		return out, e
	}
	if e = fn(&out); e != nil {
		return out, e
	}
	out.Version++
	out.UpdatedAt = s.opts.Now().UTC()
	return out, ledger.Update(ctx, tx, maintenanceRow(out), row.Version)
}

// DatabaseMaintenance 返回当前安装的实测信息和账本，不将未知磁盘归属解释为零。
func (s *Service) DatabaseMaintenance(ctx context.Context) (DatabaseMaintenanceStatus, error) {
	out := DatabaseMaintenanceStatus{WorkerEnabled: s.opts.ControlWorkerEnabled, Runs: []DatabaseMaintenanceRun{}}
	if !auth.IsPlatformRootSession(ctx) {
		return out, ErrRootRequired
	}
	if e := s.maintenanceReady(ctx); e != nil {
		out.Problem = dto.CodeOf(maintenanceError(e))
		return out, nil
	}
	if e := s.maintenanceRootActive(ctx); e != nil {
		return out, e
	}
	out.ScopeID = s.opts.Control.opts.Scope.ID()
	out.WorkerRunning = s.maintenance.started.Load()
	select {
	case <-s.maintenance.done:
		out.WorkerRunning = false
	default:
	}
	run, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	snapshot, e := s.maintenance.backend.Inspect(run)
	if e != nil {
		out.Problem = "DATABASE_MAINTENANCE_OBSERVATION_UNAVAILABLE"
		return out, nil
	}
	out.Snapshot = &snapshot
	rows, e := s.maintenanceLedger().List(run)
	if e != nil {
		out.Problem = "DATABASE_MAINTENANCE_LEDGER_UNAVAILABLE"
		return out, nil
	}
	for i := range rows {
		v, e := decodeMaintenance(&rows[i], s.opts.Control.opts.Scope.ID())
		if e != nil {
			out.Problem = "DATABASE_MAINTENANCE_LEDGER_UNAVAILABLE"
			return out, nil
		}
		if v.State != "preview" && v.State != "consumed" {
			out.Runs = append(out.Runs, v)
		}
	}
	row, e := s.maintenanceLedger().Active(run, s.opts.DB)
	if e == nil {
		v, e := decodeMaintenance(row, s.opts.Control.opts.Scope.ID())
		if e != nil {
			return out, e
		}
		out.Active = &v
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, maintenanceError(e)
	}
	if !out.WorkerRunning {
		out.Problem = "DATABASE_MAINTENANCE_WORKER_UNAVAILABLE"
		return out, nil
	}
	if e = s.maintenanceGate(run); e != nil {
		out.Problem = dto.CodeOf(maintenanceError(e))
		return out, nil
	}
	out.Ready = true
	return out, nil
}

func maintenancePeak(mode string, snapshot repo.MaintenanceSnapshot, ids []string) (int64, error) {
	need, e := maintenanceRequirement(mode, snapshot, ids)
	if e != nil {
		return 0, e
	}
	const unit = int64(64 << 20)
	if need > math.MaxInt64-2*unit {
		return 0, ErrStateUnavailable
	}
	return (need+unit-1)/unit*unit + unit, nil
}
func maintenanceRequirement(mode string, snapshot repo.MaintenanceSnapshot, ids []string) (int64, error) {
	if !repo.ValidMaintenanceRequest(mode, ids) {
		return 0, controlError(400, "DATABASE_MAINTENANCE_INPUT_INVALID")
	}
	peak := int64(64 << 20)
	for _, id := range ids {
		found := false
		for _, t := range snapshot.Tables {
			if t.ID != id {
				continue
			}
			found = true
			if !t.CanMaintain {
				return 0, controlError(403, "DATABASE_MAINTENANCE_OWNER_REQUIRED")
			}
			if !t.IndexesValid {
				return 0, repo.ErrMaintenanceIndexes
			}
			if mode == "compact" {
				if !snapshot.LocalDiskVerified || !t.DefaultTablespace {
					return 0, repo.ErrMaintenanceDisk
				}
				if t.TotalBytes < 0 || t.TotalBytes > (math.MaxInt64-peak)/2 {
					return 0, ErrStateUnavailable
				}
				peak += t.TotalBytes * 2
			}
		}
		if !found {
			return 0, repo.ErrMaintenanceTarget
		}
	}
	if mode == repo.IncrementalMaintenanceMode {
		if !snapshot.LocalDiskVerified {
			return 0, repo.ErrMaintenanceDisk
		}
		if snapshot.IncrementalProblem != "" {
			return 0, controlError(409, snapshot.IncrementalProblem)
		}
		return repo.IncrementalMaintenanceWorkingBytes, nil
	}
	if mode == "compact" {
		if !snapshot.RewriteSafe || snapshot.WALBudgetBytes <= 0 {
			return 0, controlError(409, "DATABASE_MAINTENANCE_WAL_PROTECTED")
		}
		if snapshot.WALBudgetBytes > (math.MaxInt64-peak)/2 {
			return 0, ErrStateUnavailable
		}
		peak += snapshot.WALBudgetBytes * 2
	}
	return peak, nil
}
func (s *Service) PreviewDatabaseMaintenance(ctx context.Context, in DatabaseMaintenancePreview) (DatabaseMaintenanceRun, error) {
	var out DatabaseMaintenanceRun
	if !auth.IsPlatformRootSession(ctx) {
		return out, ErrRootRequired
	}
	if e := s.maintenanceReady(ctx); e != nil {
		return out, e
	}
	if e := s.maintenanceRootActive(ctx); e != nil {
		return out, e
	}
	run, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if e := s.maintenanceGate(run); e != nil {
		return out, e
	}
	snapshot, e := s.maintenance.backend.Inspect(run)
	if e != nil {
		return out, maintenanceError(e)
	}
	peak, e := maintenancePeak(in.Mode, snapshot, in.Tables)
	if e != nil {
		return out, maintenanceError(e)
	}
	budget, e := s.opts.Budget.Status(run)
	if e != nil {
		return out, maintenanceError(e)
	}
	floor, e := storagebudget.ReclamationFloor(maintenanceOperation(in.Mode), budget)
	if e != nil {
		return out, maintenanceError(e)
	}
	if budget.AvailableBytes-budget.ReservedBytes-floor < peak {
		return out, controlError(409, "DATABASE_MAINTENANCE_SPACE_INSUFFICIENT")
	}
	if in.Mode == repo.IncrementalMaintenanceMode {
		if e = s.maintenance.backend.CheckIncremental(run, snapshot); e != nil {
			return out, maintenanceError(e)
		}
	}
	ids := append([]string(nil), in.Tables...)
	sort.Strings(ids)
	now := s.opts.Now().UTC()
	expiry := now.Add(5 * time.Minute)
	out = DatabaseMaintenanceRun{ID: uuid.NewString(), Version: 1, ScopeID: s.opts.Control.opts.Scope.ID(), State: "preview", Mode: in.Mode, Actor: rootActor(ctx), At: now, UpdatedAt: now, ExpiresAt: &expiry, Tables: ids, Before: snapshot, PeakBytes: peak, Steps: []DatabaseMaintenanceStep{}}
	e = s.maintenanceLedger().Create(run, s.opts.DB, maintenanceRow(out))
	return out, maintenanceError(e)
}
func (s *Service) StartDatabaseMaintenanceRequest(ctx context.Context, in DatabaseMaintenanceRequest) (DatabaseMaintenanceRun, error) {
	var out DatabaseMaintenanceRun
	if !auth.IsPlatformRootSession(ctx) {
		return out, ErrRootRequired
	}
	if !in.Confirm || !validTargetUUID(in.PreviewID) || !validTargetUUID(in.RequestID) {
		return out, controlError(428, "DATABASE_MAINTENANCE_PREVIEW_REQUIRED")
	}
	if e := s.maintenanceReady(ctx); e != nil {
		return out, e
	}
	if e := s.maintenanceRootActive(ctx); e != nil {
		return out, e
	}
	if !s.maintenance.started.Load() {
		return out, controlError(503, "DATABASE_MAINTENANCE_WORKER_UNAVAILABLE")
	}
	select {
	case <-s.maintenance.done:
		return out, controlError(503, "DATABASE_MAINTENANCE_WORKER_UNAVAILABLE")
	default:
	}
	ledger := s.maintenanceLedger()
	run, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	e := s.withRecoveryGuard(run, func() error {
		return s.opts.DB.WithContext(run).Transaction(func(tx *gorm.DB) error {
			return s.maintenanceGateTx(run, tx, func() error {
				if e := ledger.Lock(run, tx); e != nil {
					return e
				}
				old, e := ledger.Get(run, tx, in.RequestID)
				if e == nil {
					v, e := decodeMaintenance(old, s.opts.Control.opts.Scope.ID())
					if e != nil {
						return e
					}
					if v.PreviewID != in.PreviewID || v.Actor != rootActor(ctx) {
						return controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
					}
					out = v
					return nil
				}
				if !errors.Is(e, gorm.ErrRecordNotFound) {
					return e
				}
				if _, e = ledger.Active(run, tx); e == nil {
					return repo.ErrMaintenanceBusy
				}
				if !errors.Is(e, gorm.ErrRecordNotFound) {
					return e
				}
				row, e := ledger.Get(run, tx, in.PreviewID)
				if e != nil {
					return controlError(409, "DATABASE_MAINTENANCE_PREVIEW_EXPIRED")
				}
				preview, e := decodeMaintenance(row, s.opts.Control.opts.Scope.ID())
				if e != nil {
					return e
				}
				now := s.opts.Now().UTC()
				if preview.State != "preview" || preview.Actor != rootActor(ctx) || preview.ExpiresAt == nil || now.Before(preview.At) || !now.Before(*preview.ExpiresAt) {
					return controlError(409, "DATABASE_MAINTENANCE_PREVIEW_EXPIRED")
				}
				if preview.Mode != "reuse" && !in.ExclusiveLockConfirmed {
					return controlError(428, "DATABASE_MAINTENANCE_LOCK_CONFIRMATION_REQUIRED")
				}
				out = preview
				out.ID = in.RequestID
				out.PreviewID = preview.ID
				out.State = "queued"
				out.ExpiresAt = nil
				out.At = now
				out.UpdatedAt = now
				preview.State = "consumed"
				preview.Version++
				preview.UpdatedAt = now
				if e = ledger.Update(run, tx, maintenanceRow(preview), row.Version); e != nil {
					return e
				}
				if e = ledger.Create(run, tx, maintenanceRow(out)); e != nil {
					return e
				}
				return s.maintenance.queue.Register(run, tx, s.maintenanceMessage(out))
			})
		})
	})
	if e != nil {
		return DatabaseMaintenanceRun{}, maintenanceError(e)
	}
	if out.State != "queued" {
		return out, nil
	}
	// 数据库已登记即保留原请求身份；派发失败由正式 TaskBus 恢复，不重建第二项任务。
	dispatch, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stop()
	if e = s.maintenance.queue.Enqueue(dispatch, s.maintenanceMessage(out)); e != nil {
		select {
		case s.maintenance.wake <- struct{}{}:
		default:
		}
	}
	s.PublishChange("database_maintenance_changed")
	return out, nil
}
func (s *Service) maintenanceMessage(run DatabaseMaintenanceRun) event_bus.TaskMessage {
	payload, _ := json.Marshal(struct {
		ScopeID string `json:"scope_id"`
		RunID   string `json:"run_id"`
	}{run.ScopeID, run.ID})
	// 重投仅用于核对账本；running 永不退回 queued，不会重复执行 SQL。
	return event_bus.TaskMessage{ID: run.ID, TenantKey: maintenanceTenant, SubscriberID: s.maintenanceSubscriber(), Topic: maintenanceTopic, Payload: payload, Metadata: map[string]string{"kind": "storage_database_maintenance", "max_attempts": "5"}}
}
func (s *Service) DatabaseMaintenanceRun(ctx context.Context, id string) (DatabaseMaintenanceRun, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return DatabaseMaintenanceRun{}, ErrRootRequired
	}
	if e := s.maintenanceReady(ctx); e != nil {
		return DatabaseMaintenanceRun{}, e
	}
	if e := s.maintenanceRootActive(ctx); e != nil {
		return DatabaseMaintenanceRun{}, e
	}
	if !validTargetUUID(id) {
		return DatabaseMaintenanceRun{}, ErrPolicyInvalid
	}
	row, e := s.maintenanceLedger().Get(ctx, s.opts.DB, id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return DatabaseMaintenanceRun{}, controlError(404, "DATABASE_MAINTENANCE_RUN_NOT_FOUND")
	}
	if e != nil {
		return DatabaseMaintenanceRun{}, maintenanceError(e)
	}
	return decodeMaintenance(row, s.opts.Control.opts.Scope.ID())
}
func (s *Service) CancelDatabaseMaintenance(ctx context.Context, id string, previewID ...string) (DatabaseMaintenanceRun, error) {
	if _, e := s.DatabaseMaintenanceRun(ctx, id); e != nil {
		if dto.CodeOf(e) != "DATABASE_MAINTENANCE_RUN_NOT_FOUND" || len(previewID) != 1 || !validTargetUUID(previewID[0]) {
			return DatabaseMaintenanceRun{}, e
		}
		v, e := s.cancelUnregisteredMaintenance(ctx, id, previewID[0])
		if e != nil {
			return v, e
		}
		if v.State == "cancelled" {
			return v, nil
		}
	}
	out, e := s.maintenanceChange(ctx, id, func(v *DatabaseMaintenanceRun) error {
		switch v.State {
		case "queued":
			v.State = "cancelled"
		case "running":
			v.State = "cancel_requested"
		case "unconfirmed", "cancel_requested", "cancelled":
		default:
			return controlError(409, "DATABASE_MAINTENANCE_NOT_CANCELLABLE")
		}
		v.CancelledBy = rootActor(ctx)
		return nil
	})
	if e != nil {
		return out, e
	}
	if out.Session != nil && (out.State == "cancel_requested" || out.State == "unconfirmed") {
		if _, e = s.maintenance.backend.SessionActive(ctx, *out.Session, true); e != nil {
			return out, maintenanceError(e)
		}
	}
	return out, nil
}

// cancelUnregisteredMaintenance 原子保存取消墓碑；网络延迟的原启动请求随后只能返回 cancelled。
func (s *Service) cancelUnregisteredMaintenance(ctx context.Context, id, previewID string) (DatabaseMaintenanceRun, error) {
	var out DatabaseMaintenanceRun
	ledger := s.maintenanceLedger()
	e := s.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := ledger.Lock(ctx, tx); e != nil {
			return e
		}
		row, e := ledger.Get(ctx, tx, id)
		if e == nil {
			out, e = decodeMaintenance(row, s.opts.Control.opts.Scope.ID())
			if e != nil {
				return e
			}
			if out.PreviewID != previewID {
				return controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		row, e = ledger.Get(ctx, tx, previewID)
		if e != nil {
			return controlError(409, "DATABASE_MAINTENANCE_PREVIEW_EXPIRED")
		}
		preview, e := decodeMaintenance(row, s.opts.Control.opts.Scope.ID())
		if e != nil {
			return e
		}
		if preview.State != "preview" || preview.Actor != rootActor(ctx) {
			return controlError(409, "DATABASE_MAINTENANCE_PREVIEW_EXPIRED")
		}
		now := s.opts.Now().UTC()
		out = preview
		out.ID = id
		out.PreviewID = previewID
		out.State = "cancelled"
		out.CancelledBy = rootActor(ctx)
		out.At = now
		out.UpdatedAt = now
		out.ExpiresAt = nil
		preview.State = "consumed"
		preview.Version++
		preview.UpdatedAt = now
		if e = ledger.Update(ctx, tx, maintenanceRow(preview), row.Version); e != nil {
			return e
		}
		return ledger.Create(ctx, tx, maintenanceRow(out))
	})
	if e == nil {
		s.PublishChange("database_maintenance_changed")
	}
	return out, maintenanceError(e)
}
func (s *Service) ReviewDatabaseMaintenance(ctx context.Context, id string) (DatabaseMaintenanceRun, error) {
	v, e := s.DatabaseMaintenanceRun(ctx, id)
	if e != nil {
		return v, e
	}
	if v.State == "reviewed" {
		return v, nil
	}
	if v.State != "unconfirmed" {
		return v, controlError(409, "DATABASE_MAINTENANCE_REVIEW_NOT_REQUIRED")
	}
	if v.Session != nil {
		active, e := s.maintenance.backend.SessionActive(ctx, *v.Session, false)
		if e != nil {
			return v, maintenanceError(e)
		}
		if active {
			return v, repo.ErrMaintenanceBusy
		}
	}
	if v.ReservationID != "" {
		if e := s.opts.Budget.ReleaseReclamation(ctx, v.ReservationID, maintenanceOperation(v.Mode)); e != nil {
			return v, e
		}
	}
	return s.maintenanceChange(ctx, id, func(v *DatabaseMaintenanceRun) error {
		if v.State != "unconfirmed" {
			return repo.ErrRevisionConflict
		}
		v.State = "reviewed"
		v.ReviewedBy = rootActor(ctx)
		return nil
	})
}
func maintenanceOperation(mode string) string {
	switch mode {
	case "compact":
		return "database-compaction"
	case repo.IncrementalMaintenanceMode:
		return "database-incremental-compaction"
	default:
		return "database-vacuum"
	}
}
