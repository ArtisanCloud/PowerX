package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// StartDatabaseMaintenance 启动当前安装的正式 TaskBus 消费者，不创建第二套队列。
func (s *Service) StartDatabaseMaintenance(ctx context.Context, onError func(error)) {
	if s.maintenance == nil || !s.opts.ControlWorkerEnabled || !s.opts.DatabaseRetentionAllowed || s.maintenance.queue == nil {
		return
	}
	s.maintenance.once.Do(func() {
		s.maintenance.started.Store(true)
		go func() {
			defer close(s.maintenance.done)
			if e := s.maintenanceReady(ctx); e != nil {
				if onError != nil {
					onError(e)
				}
				return
			}
			// 老进程可能在任一已记录步骤中退出；只标记待核对，绝不恢复成 queued 重做。
			if e := s.reconcileInterruptedMaintenance(ctx); e != nil {
				if onError != nil {
					onError(e)
				}
				return
			}
			recoverNeeded := true
			for ctx.Err() == nil {
				select {
				case <-s.maintenance.wake:
					recoverNeeded = true
				default:
				}
				if recoverNeeded {
					if e := s.maintenance.queue.Recover(ctx, maintenanceTenant, s.maintenanceSubscriber()); e != nil {
						if onError != nil {
							onError(maintenanceError(e))
						}
						if !controlBackoff(ctx) {
							return
						}
						continue
					}
					recoverNeeded = false
				}
				deliveries, e := s.maintenance.queue.Dequeue(ctx, maintenanceTenant, s.maintenanceSubscriber(), "storage-maintenance-v1", s.opts.Identity.InstanceID, 1, 5*time.Second)
				if e != nil {
					if ctx.Err() != nil {
						return
					}
					recoverNeeded = true
					if onError != nil {
						onError(maintenanceError(e))
					}
					if !controlBackoff(ctx) {
						return
					}
					continue
				}
				for _, item := range deliveries {
					if e = s.applyDatabaseMaintenance(ctx, item); e != nil {
						if ctx.Err() != nil {
							return
						}
						_ = s.maintenance.queue.Reject(ctx, item, "DATABASE_MAINTENANCE_DELIVERY_UNCONFIRMED", false)
						if onError != nil {
							onError(maintenanceError(e))
						}
					}
				}
			}
		}()
	})
}
func (s *Service) reconcileInterruptedMaintenance(ctx context.Context) error {
	row, e := s.maintenanceLedger().Active(ctx, s.opts.DB)
	if e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil
		}
		return e
	}
	v, e := decodeMaintenance(row, s.opts.Control.opts.Scope.ID())
	if e != nil {
		return e
	}
	if v.State == "running" || v.State == "cancel_requested" {
		_, e = s.maintenanceChange(ctx, v.ID, func(x *DatabaseMaintenanceRun) error {
			if x.State == "running" || x.State == "cancel_requested" {
				x.State = "unconfirmed"
				x.Problem = "DATABASE_MAINTENANCE_INTERRUPTED"
			}
			return nil
		})
	}
	return e
}

func (s *Service) applyDatabaseMaintenance(ctx context.Context, item event_bus.StreamTaskDelivery) error {
	var in struct {
		ScopeID string `json:"scope_id"`
		RunID   string `json:"run_id"`
	}
	x := json.NewDecoder(bytes.NewReader(item.Message.Payload))
	x.DisallowUnknownFields()
	if x.Decode(&in) != nil || x.Decode(new(any)) != io.EOF || in.ScopeID != s.opts.Control.opts.Scope.ID() || !validTargetUUID(in.RunID) || item.Message.ID != in.RunID || item.Message.TenantKey != maintenanceTenant || item.Message.SubscriberID != s.maintenanceSubscriber() || item.Message.Topic != maintenanceTopic {
		return controlError(400, "DATABASE_MAINTENANCE_DELIVERY_INVALID")
	}
	row, e := s.maintenanceLedger().Get(ctx, s.opts.DB, in.RunID)
	if e != nil {
		return e
	}
	v, e := decodeMaintenance(row, in.ScopeID)
	if e != nil {
		return e
	}
	if v.State == "running" || v.State == "cancel_requested" {
		// 同一任务的活跃投递不能被第二个消费者提前 ACK 或重做。
		if v.Session != nil {
			active, e := s.maintenance.backend.SessionActive(ctx, *v.Session, false)
			if e != nil {
				return e
			}
			if active {
				return repo.ErrMaintenanceBusy
			}
		}
		if _, e = s.maintenanceDeliveryChange(ctx, item, v.ID, true, func(x *DatabaseMaintenanceRun) error {
			x.State = "unconfirmed"
			x.Problem = "DATABASE_MAINTENANCE_INTERRUPTED"
			return nil
		}); e != nil {
			return e
		}
		return s.maintenance.queue.Ack(ctx, item)
	}
	if v.State != "queued" {
		if e = s.maintenance.queue.Complete(ctx, item, func(*gorm.DB) error { return nil }); e != nil && !errors.Is(e, taskqueue.ErrTaskTerminal) {
			return e
		}
		return s.maintenance.queue.Ack(ctx, item)
	}
	if e = s.executeDatabaseMaintenance(ctx, item, v); e != nil {
		return e
	}
	return s.maintenance.queue.Ack(ctx, item)
}

// maintenanceDeliveryChange 让步骤和终态受正式 TaskBus 租约约束；提交后才推送变更。
func (s *Service) maintenanceDeliveryChange(ctx context.Context, item event_bus.StreamTaskDelivery, id string, complete bool, fn func(*DatabaseMaintenanceRun) error) (DatabaseMaintenanceRun, error) {
	var out DatabaseMaintenanceRun
	apply := s.maintenance.queue.Apply
	if complete {
		apply = s.maintenance.queue.Complete
	}
	e := apply(ctx, item, func(tx *gorm.DB) error {
		var e error
		out, e = s.maintenanceChangeTx(ctx, tx, id, fn)
		return e
	})
	if e == nil {
		s.PublishChange("database_maintenance_changed")
	}
	return out, e
}

func (s *Service) executeDatabaseMaintenance(ctx context.Context, item event_bus.StreamTaskDelivery, v DatabaseMaintenanceRun) error {
	// running 在任何外部 SQL 之前持久化。队列重投只核对终态，不重复执行。
	var e error
	v, e = s.maintenanceDeliveryChange(ctx, item, v.ID, false, func(x *DatabaseMaintenanceRun) error {
		if x.State != "queued" {
			return repo.ErrRevisionConflict
		}
		x.State = "running"
		return nil
	})
	if e != nil {
		return e
	}
	timed, stopTimeout := context.WithTimeout(ctx, 32*time.Minute)
	defer stopTimeout()
	run, stopRun := context.WithCancelCause(timed)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-run.Done():
				return
			case <-tick.C:
				check, cancel := context.WithTimeout(run, 2*time.Second)
				e := s.maintenance.queue.Renew(check, item)
				cancel()
				if e != nil {
					stopRun(controlError(409, "DATABASE_MAINTENANCE_DELIVERY_LEASE_LOST"))
					return
				}
			}
		}
	}()
	defer func() { stopRun(nil); <-heartbeatDone }()
	finalState := "failed"
	problem := ""
	var lease *budget.Lease
	finish := func(cause error) error {
		if cause != nil && problem == "" {
			problem = dto.CodeOf(maintenanceError(cause))
			if problem == "" {
				problem = "DATABASE_MAINTENANCE_FAILED"
			}
		}
		measure, stopMeasure := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		after, measureError := s.maintenance.backend.Inspect(measure)
		var available *int64
		if v.Before.LocalDiskVerified {
			if snap, e := s.opts.Budget.Status(measure); e == nil {
				available = &snap.AvailableBytes
			}
		}
		stopMeasure()
		persist, stop := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
		defer stop()
		_, saveError := s.maintenanceDeliveryChange(persist, item, v.ID, true, func(x *DatabaseMaintenanceRun) error {
			if x.State == "cancel_requested" && finalState != "unconfirmed" && finalState != "completed" {
				finalState = "cancelled"
			}
			x.State = finalState
			x.Problem = problem
			if measureError == nil && repo.SameMaintenanceTarget(x.Before, after, x.Tables) {
				x.After = &after
			}
			x.FilesystemAvailableAfter = available
			return nil
		})
		if lease != nil && finalState != "unconfirmed" && saveError == nil {
			if releaseError := lease.Release(persist); releaseError != nil {
				return releaseError
			}
		}
		return saveError
	}
	if e = s.maintenance.backend.RootActive(run, v.Actor); e != nil {
		problem = "DATABASE_MAINTENANCE_ROOT_REVOKED"
		return finish(e)
	}
	if e = s.maintenanceGate(run); e != nil {
		return finish(e)
	}
	actual, e := s.maintenance.backend.Inspect(run)
	if e != nil {
		return finish(e)
	}
	if !repo.SameMaintenanceTarget(v.Before, actual, v.Tables) {
		return finish(repo.ErrMaintenanceTarget)
	}
	peak, e := maintenanceRequirement(v.Mode, actual, v.Tables)
	if e != nil {
		return finish(e)
	}
	if peak > v.PeakBytes {
		return finish(controlError(409, "DATABASE_MAINTENANCE_PREVIEW_STALE"))
	}
	lease, e = s.opts.Budget.ReserveReclamation(run, maintenanceOperation(v.Mode), v.PeakBytes, 35*time.Minute)
	if e != nil {
		problem = "DATABASE_MAINTENANCE_SPACE_INSUFFICIENT"
		return finish(e)
	}
	snapshot, e := s.opts.Budget.Status(run)
	if e != nil {
		return finish(e)
	}
	v, e = s.maintenanceDeliveryChange(run, item, v.ID, false, func(x *DatabaseMaintenanceRun) error {
		x.ReservationID = lease.ID()
		if x.Before.LocalDiskVerified {
			x.FilesystemAvailableBefore = &snapshot.AvailableBytes
		}
		return nil
	})
	if e != nil {
		finalState = "unconfirmed"
		return finish(e)
	}
	command, cancelCommand := context.WithCancelCause(run)
	defer cancelCommand(nil)
	done := make(chan struct{})
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-command.Done():
				return
			case <-tick.C:
				check, stop := context.WithTimeout(command, 2*time.Second)
				checkError := s.maintenanceSafety(check, v, lease.ID())
				stop()
				if checkError != nil {
					cancelCommand(checkError)
					return
				}
			}
		}
	}()
	e = s.maintenance.backend.Execute(command, v.ID, v.Mode, v.Before, v.Tables, func(id string, session repo.MaintenanceSession) error {
		if e := s.maintenanceSafety(command, v, lease.ID()); e != nil {
			return e
		}
		_, e := s.maintenanceDeliveryChange(command, item, v.ID, false, func(x *DatabaseMaintenanceRun) error {
			if x.State == "cancel_requested" {
				return controlError(409, "DATABASE_MAINTENANCE_CANCELLED")
			}
			if x.State != "running" {
				return repo.ErrRevisionConflict
			}
			x.Session = &session
			x.Steps = append(x.Steps, DatabaseMaintenanceStep{TableID: id, State: "running", StartedAt: s.opts.Now().UTC()})
			return nil
		})
		return e
	}, func(id string) error {
		_, e := s.maintenanceDeliveryChange(command, item, v.ID, false, func(x *DatabaseMaintenanceRun) error {
			for i := range x.Steps {
				if x.Steps[i].TableID == id && x.Steps[i].State == "running" {
					now := s.opts.Now().UTC()
					x.Steps[i].State = "completed"
					x.Steps[i].FinishedAt = &now
					return nil
				}
			}
			return ErrStateUnavailable
		})
		return e
	})
	close(done)
	<-monitorDone
	if e == nil {
		finalState = "completed"
	} else {
		var pgError *pgconn.PgError
		switch {
		case ctx.Err() != nil:
			finalState = "unconfirmed"
			problem = "DATABASE_MAINTENANCE_INTERRUPTED"
		case context.Cause(command) != nil:
			finalState = "cancelled"
			problem = dto.CodeOf(maintenanceError(context.Cause(command)))
			if problem == "DATABASE_MAINTENANCE_DELIVERY_LEASE_LOST" {
				finalState = "unconfirmed"
			}
		case errors.As(e, &pgError):
			if pgError.Code == "57014" {
				problem = "DATABASE_MAINTENANCE_COMMAND_CANCELLED"
			} else if pgError.Code == "55P03" {
				problem = "DATABASE_MAINTENANCE_LOCK_TIMEOUT"
			} else {
				problem = "DATABASE_MAINTENANCE_COMMAND_FAILED"
			}
		case dto.CodeOf(e) != "" || errors.Is(e, repo.ErrMaintenanceTarget) || errors.Is(e, repo.ErrMaintenanceBusy) || errors.Is(e, repo.ErrMaintenanceDisk):
			finalState = "failed"
		default:
			finalState = "unconfirmed"
			problem = "DATABASE_MAINTENANCE_RESULT_UNCONFIRMED"
		}
	}
	return finish(e)
}
func (s *Service) maintenanceSafety(ctx context.Context, v DatabaseMaintenanceRun, leaseID string) error {
	if e := s.maintenanceGate(ctx); e != nil {
		return e
	}
	if e := s.maintenance.backend.RootActive(ctx, v.Actor); e != nil {
		return controlError(403, "DATABASE_MAINTENANCE_ROOT_REVOKED")
	}
	row, e := s.maintenanceLedger().Get(ctx, s.opts.DB, v.ID)
	if e != nil {
		return controlError(503, "DATABASE_MAINTENANCE_LEDGER_UNAVAILABLE")
	}
	current, e := decodeMaintenance(row, v.ScopeID)
	if e != nil {
		return e
	}
	if current.State == "cancel_requested" {
		return controlError(409, "DATABASE_MAINTENANCE_CANCELLED")
	}
	if current.State != "running" {
		return controlError(409, "DATABASE_MAINTENANCE_RESULT_UNCONFIRMED")
	}
	snap, e := s.opts.Budget.Status(ctx)
	if e != nil {
		return controlError(503, "DATABASE_MAINTENANCE_BUDGET_UNAVAILABLE")
	}
	var own int64
	found := false
	for _, r := range snap.Active {
		if r.ID == leaseID {
			own = r.PeakBytes - r.WrittenBytes
			found = true
		}
	}
	if !found {
		return controlError(409, "DATABASE_MAINTENANCE_LEASE_UNCONFIRMED")
	}
	if v.Mode == "compact" && snap.AvailableBytes-(snap.ReservedBytes-own) < snap.CriticalFreeBytes+(1<<30) {
		return controlError(409, "DATABASE_MAINTENANCE_SPACE_INSUFFICIENT")
	}
	return nil
}
