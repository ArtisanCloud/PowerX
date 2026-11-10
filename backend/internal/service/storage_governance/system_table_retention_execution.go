package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"time"
)

func (s *Service) systemRetentionLedger(id string) *repo.RetentionRunRepository {
	return repo.NewSystemRetentionRunRepository(s.opts.DB, s.opts.Control.opts.Scope.ID(), id)
}
func decodeSystemRetentionRun(row *m.CleanupRun, id string) (SystemTableRetentionRun, error) {
	var r SystemTableRetentionRun
	if row == nil || json.Unmarshal(row.Document, &r) != nil || row.Version != 1 || r.ID != row.RunID || r.TableID != id || r.Actor != row.ActorID || r.Revision < 1 || !validSystemTablePolicy(r.Policy) || r.Rows < 0 || r.Rows > int64(r.Policy.BatchLimit) || r.State != "preview" && r.State != "completed" {
		return r, ErrStateUnavailable
	}
	return r, nil
}
func (s *Service) SystemTableRetentionRunRequest(ctx context.Context, id, key string) (SystemTableRetentionRun, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return SystemTableRetentionRun{}, ErrRootRequired
	}
	if !repo.SystemRetentionSupported(id) {
		return SystemTableRetentionRun{}, ErrSystemTableNotFound
	}
	if s.opts.Control == nil || s.opts.DB == nil || !validTargetUUID(key) {
		return SystemTableRetentionRun{}, ErrPolicyInvalid
	}
	row, e := s.systemRetentionLedger(id).Get(ctx, s.opts.DB, key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return SystemTableRetentionRun{}, controlError(404, "SYSTEM_RETENTION_RUN_NOT_FOUND")
	}
	if e != nil {
		return SystemTableRetentionRun{}, ErrStateUnavailable
	}
	r, e := decodeSystemRetentionRun(row, id)
	if e != nil {
		return r, e
	}
	if row.State != "completed" || r.State != "completed" || r.Actor != rootActor(ctx) {
		return SystemTableRetentionRun{}, controlError(403, "SYSTEM_RETENTION_RUN_FORBIDDEN")
	}
	return r, nil
}
func (s *Service) PreviewSystemTableRetention(ctx context.Context, id string, revision int64) (SystemTableRetentionRun, error) {
	return s.systemTableBatch(ctx, id, revision, "", "", false, false)
}
func (s *Service) RunSystemTableRetention(ctx context.Context, id string, revision int64, key, preview string) (SystemTableRetentionRun, error) {
	return s.systemTableBatch(ctx, id, revision, key, preview, true, false)
}
func (s *Service) systemTableBatch(ctx context.Context, id string, revision int64, key, preview string, apply, scheduled bool) (SystemTableRetentionRun, error) {
	out := SystemTableRetentionRun{ID: key, TableID: id, At: s.opts.Now().UTC(), Revision: revision, State: "preview", Actor: rootActor(ctx), PreviewID: preview, Trigger: "manual"}
	if !scheduled && !auth.IsPlatformRootSession(ctx) {
		return out, ErrRootRequired
	}
	if !repo.SystemRetentionSupported(id) {
		return out, ErrSystemTableNotFound
	}
	if s.opts.Control == nil || s.opts.Budget == nil || s.systemRetention == nil || !s.opts.Control.Ready(ctx) {
		return out, ErrStateUnavailable
	}
	if apply && (!validTargetUUID(key) || !scheduled && !validTargetUUID(preview)) {
		return out, controlError(428, "SYSTEM_RETENTION_PREVIEW_REQUIRED")
	}
	if !apply {
		out.ID = uuid.NewString()
	}
	if scheduled {
		out.Trigger = "scheduled"
	}
	run, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ledger := s.systemRetentionLedger(id)
	if !ledger.Ready(run) {
		return out, controlError(503, "SYSTEM_RETENTION_LEDGER_UNAVAILABLE")
	}
	e := s.opts.DB.WithContext(run).Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() != "postgres" {
			return ErrStateUnavailable
		}
		if e := tx.Exec("SET LOCAL lock_timeout='1s'").Error; e != nil {
			return e
		}
		if e := tx.Exec("SET LOCAL statement_timeout='8s'").Error; e != nil {
			return e
		}
		return s.opts.Control.WithExecutionTx(run, tx, systemRetentionDomain(id), func(row *m.ControlConfiguration) error {
			if apply {
				old, e := ledger.Get(run, tx, key)
				if e == nil {
					r, e := decodeSystemRetentionRun(old, id)
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
			p, c, e := s.systemRetentionControlTx(run, tx, id, row)
			if e != nil {
				return e
			}
			if c.Authority != "database" {
				if scheduled {
					return nil
				}
				return controlError(428, "SYSTEM_RETENTION_ADOPTION_REQUIRED")
			}
			if !c.ExecutionReady {
				if scheduled {
					return nil
				}
				return controlError(409, "SYSTEM_RETENTION_PUBLICATION_PENDING")
			}
			if !scheduled && revision != c.DesiredRevision {
				return controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
			}
			out.Revision = c.DesiredRevision
			out.Policy = p
			if !s.opts.DatabaseRetentionAllowed {
				return ErrStateUnavailable
			}
			if apply && !p.Enabled {
				if scheduled {
					return nil
				}
				return controlError(409, "SYSTEM_RETENTION_DISABLED")
			}
			if scheduled && !s.opts.LifecycleWorkerEnabled {
				return ErrStateUnavailable
			}
			out.Cutoff = out.At.AddDate(0, 0, -p.RetentionDays)
			if apply && !scheduled {
				previous, e := ledger.Get(run, tx, preview)
				if e != nil {
					return controlError(409, "SYSTEM_RETENTION_PREVIEW_EXPIRED")
				}
				r, e := decodeSystemRetentionRun(previous, id)
				if e != nil {
					return e
				}
				if previous.State != "preview" || r.Actor != out.Actor || r.Revision != revision || r.ExpiresAt == nil || !out.At.Before(*r.ExpiresAt) || out.At.Before(r.At) || systemRetentionSettings(r.Policy) != systemRetentionSettings(p) || r.Policy.Enabled != p.Enabled || r.Policy.Actor != p.Actor || !r.Policy.UpdatedAt.Equal(p.UpdatedAt) {
					return controlError(409, "SYSTEM_RETENTION_PREVIEW_EXPIRED")
				}
				out.Cutoff = r.Cutoff
				if e = ledger.Consume(run, tx, preview, out.Actor); e != nil {
					return e
				}
			}
			out.Rows, e = repo.NewSystemRetentionRepository(tx).Batch(run, id, out.Cutoff, out.At, p.BatchLimit, apply)
			if e != nil {
				return e
			}
			if apply {
				out.State = "completed"
			} else {
				expiry := out.At.Add(5 * time.Minute)
				out.ExpiresAt = &expiry
			}
			if scheduled && out.Rows == 0 {
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
		return SystemTableRetentionRun{ID: out.ID, TableID: id, State: "unconfirmed", At: out.At}, e
	}
	if apply {
		s.PublishChange("system_table_retention_completed")
	}
	return out, nil
}
