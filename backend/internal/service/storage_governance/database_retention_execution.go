package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"time"
)

func (s *Service) retentionLedger() *repo.RetentionRunRepository {
	return repo.NewRetentionRunRepository(s.opts.DB, s.opts.Control.opts.Scope.ID())
}
func (s *Service) retentionCutoffs(p DatabaseRetentionPolicy, now time.Time) caprepo.RetentionCutoffs {
	return caprepo.RetentionCutoffs{Success: now.AddDate(0, 0, -p.SuccessDays), Failure: now.AddDate(0, 0, -p.FailureDays), Pending: now.Add(-time.Duration(p.PendingHours) * time.Hour), Quarantine: now.AddDate(0, 0, -p.QuarantineDays), Limit: p.BatchLimit, AdoptLegacy: p.AdoptLegacy}
}
func decodeRetentionRun(row *m.CleanupRun) (DatabaseRetentionRun, error) {
	var r DatabaseRetentionRun
	if row == nil || json.Unmarshal(row.Document, &r) != nil || row.Version != 1 || r.ID != row.RunID || r.Actor != row.ActorID || r.PolicySnapshot == nil || !validDatabasePolicy(*r.PolicySnapshot) || r.Revision < 1 || r.PolicySnapshot.Revision != r.Revision || r.Cutoffs == nil || r.Cutoffs.Limit != r.PolicySnapshot.BatchLimit || r.Counts.Traces < 0 || r.Counts.Events < 0 || r.Counts.Quarantined < 0 || r.Counts.Classified < 0 || r.State != "preview" && r.State != "completed" {
		return r, ErrStateUnavailable
	}
	return r, nil
}

// DatabaseRetentionRunRequest 只读取原执行回执，不因 404 再提交清理。
func (s *Service) DatabaseRetentionRunRequest(ctx context.Context, key string) (DatabaseRetentionRun, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return DatabaseRetentionRun{}, ErrRootRequired
	}
	if s.opts.Control == nil || s.opts.DB == nil || !validTargetUUID(key) {
		return DatabaseRetentionRun{}, ErrPolicyInvalid
	}
	row, e := s.retentionLedger().Get(ctx, s.opts.DB, key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return DatabaseRetentionRun{}, controlError(404, "DATABASE_RETENTION_RUN_NOT_FOUND")
	}
	if e != nil {
		return DatabaseRetentionRun{}, ErrStateUnavailable
	}
	r, e := decodeRetentionRun(row)
	if e != nil {
		return r, e
	}
	if row.State != "completed" || r.State != "completed" || r.Actor != rootActor(ctx) {
		return DatabaseRetentionRun{}, controlError(403, "DATABASE_RETENTION_RUN_FORBIDDEN")
	}
	return r, nil
}
func (s *Service) PreviewDatabaseRetention(ctx context.Context, revision int64) (DatabaseRetentionRun, error) {
	return s.databaseBatch(ctx, revision, "", false, false)
}
func (s *Service) RunConfirmedDatabaseRetention(ctx context.Context, revision int64, key, preview string) (DatabaseRetentionRun, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return DatabaseRetentionRun{}, ErrRootRequired
	}
	if !validTargetUUID(preview) {
		return DatabaseRetentionRun{}, controlError(428, "DATABASE_RETENTION_PREVIEW_REQUIRED")
	}
	return s.databaseBatchConfirmed(ctx, revision, key, preview, true, false)
}
func (s *Service) databaseBatch(ctx context.Context, revision int64, key string, apply, scheduled bool) (DatabaseRetentionRun, error) {
	return s.databaseBatchConfirmed(ctx, revision, key, "", apply, scheduled)
}
func (s *Service) databaseBatchConfirmed(ctx context.Context, revision int64, key, preview string, apply, scheduled bool) (DatabaseRetentionRun, error) {
	out := DatabaseRetentionRun{ID: key, Revision: revision, At: s.opts.Now().UTC(), Actor: rootActor(ctx), State: "preview", Trigger: "manual", PreviewID: preview}
	if !scheduled && !auth.IsPlatformRootSession(ctx) {
		return out, ErrRootRequired
	}
	if s.opts.Control == nil || s.opts.Budget == nil || s.dbRetention == nil || !s.opts.Control.Ready(ctx) {
		return out, ErrStateUnavailable
	}
	if apply && (!validTargetUUID(key) || !scheduled && !validTargetUUID(preview)) {
		return out, controlError(428, "DATABASE_RETENTION_PREVIEW_REQUIRED")
	}
	if !apply {
		out.ID = uuid.NewString()
	}
	if scheduled {
		out.Trigger = "scheduled"
	}
	run, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if e := s.dbRetention.Ready(run); e != nil {
		return out, e
	}
	ledger := s.retentionLedger()
	if !ledger.Ready(run) {
		return out, controlError(503, "DATABASE_RETENTION_LEDGER_UNAVAILABLE")
	}
	e := s.opts.DB.WithContext(run).Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() != "postgres" {
			return controlError(503, "DATABASE_RETENTION_POSTGRES_REQUIRED")
		}
		if e := tx.Exec("SET LOCAL lock_timeout='1s'").Error; e != nil {
			return e
		}
		if e := tx.Exec("SET LOCAL statement_timeout='8s'").Error; e != nil {
			return e
		}
		return s.opts.Control.WithExecutionTx(run, tx, ControlDatabaseRetention, func(row *m.ControlConfiguration) error {
			if apply {
				old, e := ledger.Get(run, tx, key)
				if e == nil {
					r, e := decodeRetentionRun(old)
					if e != nil {
						return e
					}
					if old.State != "completed" || r.State != "completed" || r.Actor != out.Actor || r.Trigger != out.Trigger || r.PreviewID != preview || !scheduled && r.Revision != revision {
						return controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
					}
					out = r
					return nil
				}
				if !errors.Is(e, gorm.ErrRecordNotFound) {
					return e
				}
			}
			p, c, e := s.retentionControlTx(run, tx, row)
			if e != nil {
				return e
			}
			if c.Authority != "database" {
				return controlError(428, "DATABASE_RETENTION_ADOPTION_REQUIRED")
			}
			if !c.ExecutionReady {
				return controlError(409, "DATABASE_RETENTION_PUBLICATION_PENDING")
			}
			if !scheduled && p.Revision != revision {
				return controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
			}
			out.Revision = p.Revision
			out.PolicySnapshot = &p
			if !s.opts.DatabaseRetentionAllowed {
				return ErrStateUnavailable
			}
			if apply && !p.Enabled {
				if scheduled {
					return nil
				}
				return controlError(409, "DATABASE_RETENTION_DISABLED")
			}
			if scheduled && !s.opts.LifecycleWorkerEnabled {
				return ErrStateUnavailable
			}
			cutoffs := s.retentionCutoffs(p, out.At)
			if apply && !scheduled {
				previous, e := ledger.Get(run, tx, preview)
				if e != nil {
					return controlError(409, "DATABASE_RETENTION_PREVIEW_EXPIRED")
				}
				r, e := decodeRetentionRun(previous)
				if e != nil {
					return e
				}
				if previous.State != "preview" || r.Actor != out.Actor || r.Revision != revision || r.ExpiresAt == nil || out.At.After(*r.ExpiresAt) || out.At.Before(r.At) {
					return controlError(409, "DATABASE_RETENTION_PREVIEW_EXPIRED")
				}
				cutoffs = *r.Cutoffs
				if cutoffs.Limit != p.BatchLimit || cutoffs.AdoptLegacy != p.AdoptLegacy {
					return ErrStateUnavailable
				}
				if e = ledger.Consume(run, tx, preview, out.Actor); e != nil {
					return e
				}
			}
			out.Cutoffs = &cutoffs
			out.Counts, e = caprepo.NewRetentionRepository(tx).Batch(run, cutoffs, apply)
			if e != nil {
				return e
			}
			if apply {
				out.State = "completed"
			} else {
				expiry := out.At.Add(5 * time.Minute)
				out.ExpiresAt = &expiry
			}
			if scheduled && out.Counts == (caprepo.RetentionBatch{}) {
				return nil
			}
			if e = ledger.Trim(run, tx, out.At); e != nil {
				return e
			}
			raw, e := json.Marshal(out)
			if e != nil {
				return e
			}
			expiry := out.At.AddDate(0, 0, 30)
			if out.ExpiresAt != nil {
				expiry = *out.ExpiresAt
			}
			return ledger.Create(run, tx, &m.CleanupRun{RunID: out.ID, Version: 1, State: out.State, ActorID: out.Actor, ExpiresAt: expiry, Document: datatypes.JSON(raw)})
		})
	})
	if e != nil {
		return DatabaseRetentionRun{ID: out.ID, State: "unconfirmed", At: out.At}, e
	}
	if apply {
		s.PublishChange("database_retention_completed")
	}
	return out, nil
}
