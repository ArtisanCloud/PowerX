package backup_ops

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *JobService) retentionLedger() (*recovery.Store, error) {
	if a, ok := s.authority.(CleanupRetainAuthority); ok && a.RetentionLedger() != nil {
		return a.RetentionLedger(), nil
	}
	return nil, controlFailure(503, "BACKUP_RETAIN_LEDGER_UNAVAILABLE")
}
func ledgerError(e error) error {
	if errors.Is(e, recovery.ErrRetentionConflict) {
		return controlFailure(409, "BACKUP_RETAIN_IDEMPOTENCY_CONFLICT")
	}
	if errors.Is(e, recovery.ErrRetentionFull) {
		return controlFailure(507, "BACKUP_RETAIN_LEDGER_FULL")
	}
	return controlFailure(503, "BACKUP_RETAIN_LEDGER_UNAVAILABLE")
}
func retentionRecord(row m.BackupCleanupRetention, generation string) recovery.RetentionRecord {
	r := recovery.RetentionRecord{RequestID: row.RequestID.String(), Scope: row.ScopeFingerprint, InputDigest: row.InputDigest, ReviewVersion: row.ReviewVersion, Generation: generation, ArtifactID: row.ArtifactID, ActorID: row.ActorID, Token: row.CleanupToken, Reason: row.Reason, CreatedAt: row.CreatedAt.UTC().Truncate(time.Microsecond), State: row.State, Problem: row.Problem}
	if row.CompletedAt != nil {
		v := row.CompletedAt.UTC().Truncate(time.Microsecond)
		r.CompletedAt = &v
	}
	return r
}
func recordMatchesRow(r recovery.RetentionRecord, row m.BackupCleanupRetention) bool {
	return r.Operation == "" && !row.DeletedAt.Valid && r.RequestID == row.RequestID.String() && r.Scope == row.ScopeFingerprint && r.InputDigest == row.InputDigest && r.ReviewVersion == row.ReviewVersion && r.ArtifactID == row.ArtifactID && r.ActorID == row.ActorID && r.Token == row.CleanupToken && r.Reason == row.Reason && r.CreatedAt.Equal(row.CreatedAt)
}
func (s *JobService) recoveryReceipt(ctx context.Context, r recovery.RetentionRecord, problem string) CleanupRetainReceipt {
	return CleanupRetainReceipt{RequestID: r.RequestID, ArtifactID: strconv.FormatUint(r.ArtifactID, 10), Token: r.Token, ReviewVersion: r.ReviewVersion, Reason: r.Reason, State: r.State, Problem: problem, ActorID: strconv.FormatUint(r.ActorID, 10), CreatedAt: r.CreatedAt, CompletedAt: r.CompletedAt, Protected: r.State == "retained", CleanupLockPreserved: true, RecordSource: "maintenance", RecoveryBlocked: true}
}

// resolveRetention never repairs SQL or the file ledger from a GET. A terminal
// independent receipt is historical authority, never permission to re-protect.
func (s *JobService) resolveRetention(ctx context.Context, row m.BackupCleanupRetention, sqlErr error, r *recovery.RetentionRecord, generation string) (CleanupRetainReceipt, error) {
	if r != nil {
		if r.Scope != s.retentionScope() {
			return CleanupRetainReceipt{}, controlFailure(403, "BACKUP_RETAIN_SCOPE_MISMATCH")
		}
		if r.State != "checking" {
			return s.recoveryReceipt(ctx, *r, r.Problem), nil
		}
		store, e := s.retentionLedger()
		if e != nil {
			return CleanupRetainReceipt{}, e
		}
		dispositions, e := store.CleanupResolutions(ctx, r.Scope, r.ArtifactID, r.Token)
		if e != nil {
			return CleanupRetainReceipt{}, ledgerError(e)
		}
		for _, d := range dispositions {
			if d.State != "rejected" {
				return s.recoveryReceipt(ctx, *r, "BACKUP_RETAIN_NEW_DISPOSITION_REVIEW_REQUIRED"), nil
			}
		}
		if sqlErr != nil {
			problem := "BACKUP_RETAIN_DATABASE_HISTORY_UNAVAILABLE"
			if errors.Is(sqlErr, gorm.ErrRecordNotFound) {
				problem = "BACKUP_RETAIN_DATABASE_HISTORY_MISSING"
			}
			return s.recoveryReceipt(ctx, *r, problem), nil
		}
		if !recordMatchesRow(*r, row) {
			return s.recoveryReceipt(ctx, *r, "BACKUP_RETAIN_DATABASE_HISTORY_CONFLICT"), nil
		}
		if row.State == "checking" && (generation == "" || r.Generation == "" || generation != r.Generation) {
			return s.recoveryReceipt(ctx, *r, "BACKUP_RETAIN_RECOVERY_REVIEW_REQUIRED"), nil
		}
	} else if sqlErr != nil {
		if errors.Is(sqlErr, gorm.ErrRecordNotFound) {
			return CleanupRetainReceipt{}, controlFailure(404, "BACKUP_RETAIN_REQUEST_NOT_FOUND")
		}
		return CleanupRetainReceipt{}, controlFailure(503, "BACKUP_RETAIN_UNAVAILABLE")
	}
	if row.ScopeFingerprint != s.retentionScope() {
		return CleanupRetainReceipt{}, controlFailure(403, "BACKUP_RETAIN_SCOPE_MISMATCH")
	}
	out, e := s.retainReceipt(ctx, row)
	if e != nil {
		return out, e
	}
	if r == nil && row.State == "checking" {
		out.CanResume = false
		out.RecoveryBlocked = true
		out.Problem = "BACKUP_RETAIN_LEGACY_HISTORY_ONLY"
	}
	return out, nil
}
func (s *JobService) readRetentionRequest(ctx context.Context, key string) (CleanupRetainReceipt, error) {
	store, e := s.retentionLedger()
	if e != nil {
		return CleanupRetainReceipt{}, e
	}
	r, le := store.Retention(ctx, key)
	if le == nil && r.Operation != "" {
		return CleanupRetainReceipt{}, controlFailure(404, "BACKUP_RETAIN_REQUEST_NOT_FOUND")
	}
	if le != nil && !errors.Is(le, recovery.ErrRetentionNotFound) {
		return CleanupRetainReceipt{}, ledgerError(le)
	}
	if le == nil && r.State != "checking" {
		if r.Scope != s.retentionScope() {
			return CleanupRetainReceipt{}, controlFailure(403, "BACKUP_RETAIN_SCOPE_MISMATCH")
		}
		return s.recoveryReceipt(ctx, r, r.Problem), nil
	}
	row, se := s.retainRepo.Request(ctx, uuid.MustParse(key))
	var external *recovery.RetentionRecord
	if le == nil {
		external = &r
	}
	generation, _ := s.retentionReviewBinding(ctx)
	return s.resolveRetention(ctx, row, se, external, generation)
}
func (s *JobService) readRetentionRequests(ctx context.Context, id uint64, token string) ([]CleanupRetainReceipt, error) {
	store, e := s.retentionLedger()
	if e != nil {
		return nil, e
	}
	external, e := store.Retentions(ctx, s.retentionScope(), id, token)
	if e != nil {
		return nil, ledgerError(e)
	}
	rows, se := s.retainRepo.Recent(ctx, id, token, s.retentionScope())
	if se != nil && len(external) == 0 {
		return nil, controlFailure(503, "BACKUP_RETAIN_UNAVAILABLE")
	}
	byID := map[string]m.BackupCleanupRetention{}
	for _, row := range rows {
		byID[row.RequestID.String()] = row
	}
	generation, _ := s.retentionReviewBinding(ctx)
	out := []CleanupRetainReceipt{}
	for _, r := range external {
		row, ok := byID[r.RequestID]
		sqlErr := se
		if !ok && sqlErr == nil {
			sqlErr = gorm.ErrRecordNotFound
		}
		v, e := s.resolveRetention(ctx, row, sqlErr, &r, generation)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
		delete(byID, r.RequestID)
	}
	for _, row := range byID {
		v, e := s.resolveRetention(ctx, row, nil, nil, generation)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].RequestID > out[j].RequestID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > 20 {
		out = out[:20]
	}
	return out, nil
}

// persistRetainResult runs only on an explicitly requested POST. Disk errors
// after SQL commit remain unknown; the pre-SQL UUID reservation still blocks reuse.
func (s *JobService) persistRetainResult(ctx context.Context, key string) (CleanupRetainReceipt, error) {
	store, e := s.retentionLedger()
	if e != nil {
		return CleanupRetainReceipt{}, e
	}
	r, e := store.Retention(ctx, key)
	if e != nil {
		return CleanupRetainReceipt{}, ledgerError(e)
	}
	row, e := s.retainRepo.Request(ctx, uuid.MustParse(key))
	if e != nil || !recordMatchesRow(r, row) || row.State == "checking" {
		return CleanupRetainReceipt{}, controlFailure(503, "BACKUP_RETAIN_RESULT_UNKNOWN")
	}
	if _, e = s.retainReceipt(ctx, row); e != nil {
		return CleanupRetainReceipt{}, e
	}
	if e = store.CompleteRetention(ctx, retentionRecord(row, r.Generation)); e != nil {
		return CleanupRetainReceipt{}, controlFailure(503, "BACKUP_RETAIN_RESULT_UNKNOWN")
	}
	s.cleanupSvc.notifyChanged()
	return s.recoveryReceipt(ctx, retentionRecord(row, r.Generation), row.Problem), nil
}

func (s *JobService) checkLedgerInput(ctx context.Context, r recovery.RetentionRecord, id uint64, in CleanupRetainInput) error {
	if r.ActorID != reqctx.GetClaims(ctx).UserID {
		return controlFailure(403, "BACKUP_RETAIN_ACTOR_MISMATCH")
	}
	if r.Scope != s.retentionScope() || r.InputDigest != retainInputDigest(id, in) || r.ArtifactID != id || r.Token != in.ExpectedToken || r.ReviewVersion != in.ReviewVersion || r.Reason != in.Reason {
		return controlFailure(409, "BACKUP_RETAIN_IDEMPOTENCY_CONFLICT")
	}
	return nil
}
