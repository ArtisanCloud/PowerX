package backup_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type CleanupRetainInput struct {
	RequestID     string `json:"request_id"`
	ExpectedToken string `json:"expected_token"`
	ReviewVersion string `json:"review_version"`
	Reason        string `json:"reason"`
	Confirm       bool   `json:"confirm"`
}
type CleanupRetainReceipt struct {
	RecordSource         string     `json:"record_source"`
	RecoveryBlocked      bool       `json:"recovery_blocked"`
	RequestID            string     `json:"request_id"`
	ArtifactID           string     `json:"artifact_id"`
	Token                string     `json:"token"`
	ReviewVersion        string     `json:"review_version"`
	Reason               string     `json:"reason"`
	State                string     `json:"state"`
	Problem              string     `json:"problem,omitempty"`
	ActorID              string     `json:"actor_id"`
	CreatedAt            time.Time  `json:"created_at"`
	CompletedAt          *time.Time `json:"completed_at"`
	CanResume            bool       `json:"can_resume"`
	Protected            bool       `json:"protected"`
	CleanupLockPreserved bool       `json:"cleanup_lock_preserved"`
}
type CleanupRetainEvent struct {
	Name     string
	Receipt  *CleanupRetainReceipt
	Progress *CleanupVerification
}

// A fixed, internal safety operation. No arbitrary domain, SQL, source or policy settings.
type CleanupRetainAuthority interface {
	RetentionScope() string
	RetentionLedger() *recovery.Store
	RetentionReviewBinding(context.Context) (string, error)
	RetainGuard(context.Context, uint64, string, func(context.Context, *gorm.DB, *m.BackupPolicy, string) error) error
}

func (s *JobService) retentionScope() string {
	if authority, ok := s.authority.(CleanupRetainAuthority); ok {
		return authority.RetentionScope()
	}
	return s.ExecutionFingerprint()
}
func cleanupReviewVersion(row repo.CleanupReviewRow) string {
	row.UpdatedAt = row.UpdatedAt.UTC()
	raw, _ := json.Marshal(row)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// A review authorizes one installation generation, not merely identical restored
// SQL rows. The binding comes from the independent maintenance anchor, never UI.
func boundCleanupReviewVersion(row repo.CleanupReviewRow, binding string) string {
	sum := sha256.Sum256([]byte("powerx.backup-retain-review/v2/" + binding + "/" + cleanupReviewVersion(row)))
	return hex.EncodeToString(sum[:])
}
func (s *JobService) retentionReviewBinding(ctx context.Context) (string, error) {
	if a, ok := s.authority.(CleanupRetainAuthority); ok {
		v, e := a.RetentionReviewBinding(ctx)
		if e != nil {
			return "", e
		}
		if len(v) == 64 && cleanupReviewChecksum.MatchString(v) {
			return v, nil
		}
	}
	return "", controlFailure(503, "BACKUP_RETAIN_REVIEW_BINDING_UNAVAILABLE")
}
func retainInputDigest(id uint64, in CleanupRetainInput) string {
	raw, _ := json.Marshal(struct {
		Artifact uint64
		Input    CleanupRetainInput
	}{id, in})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (s *JobService) requireRetainRoot(ctx context.Context) error { return s.requireReviewRoot(ctx) }
func (s *JobService) requireRetainSchema(ctx context.Context) error {
	run, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if s.retainRepo == nil || !s.retainRepo.Ready(run) {
		return controlFailure(503, "BACKUP_RETAIN_SCHEMA_NOT_READY")
	}
	return nil
}
func (s *JobService) retainReceipt(ctx context.Context, r m.BackupCleanupRetention) (CleanupRetainReceipt, error) {
	if r.DeletedAt.Valid || r.RequestID == uuid.Nil || !cleanupReviewChecksum.MatchString(r.InputDigest) || !cleanupReviewChecksum.MatchString(r.ScopeFingerprint) || !cleanupReviewChecksum.MatchString(r.ReviewVersion) || r.State != "checking" && r.State != "retained" && r.State != "rejected" || r.State == "checking" && r.CompletedAt != nil || r.State != "checking" && r.CompletedAt == nil {
		return CleanupRetainReceipt{}, controlFailure(503, "BACKUP_RETAIN_RECEIPT_INVALID")
	}
	return CleanupRetainReceipt{RequestID: r.RequestID.String(), ArtifactID: strconv.FormatUint(r.ArtifactID, 10), Token: r.CleanupToken, ReviewVersion: r.ReviewVersion, Reason: r.Reason, State: r.State, Problem: r.Problem, ActorID: strconv.FormatUint(r.ActorID, 10), CreatedAt: r.CreatedAt, CompletedAt: r.CompletedAt, CanResume: r.State == "checking" && r.ActorID == reqctx.GetClaims(ctx).UserID, Protected: r.State == "retained", CleanupLockPreserved: true, RecordSource: "database"}, nil
}
func (s *JobService) RetentionRequest(ctx context.Context, key string) (CleanupRetainReceipt, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return CleanupRetainReceipt{}, e
	}
	id, e := uuid.Parse(key)
	if e != nil || id == uuid.Nil || id.String() != key {
		return CleanupRetainReceipt{}, controlFailure(400, "BACKUP_RETAIN_REQUEST_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return s.readRetentionRequest(run, id.String())
}
func (s *JobService) RetentionRequests(ctx context.Context, id uint64, token string) ([]CleanupRetainReceipt, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return nil, e
	}
	if id == 0 || !validCleanupToken(token) {
		return nil, controlFailure(400, "BACKUP_REVIEW_INTENT_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return s.readRetentionRequests(run, id, token)
}
func validateRetainInput(id uint64, in CleanupRetainInput) bool {
	return id > 0 && validCleanupToken(in.RequestID) && validCleanupToken(in.ExpectedToken) && len(in.ReviewVersion) == 64 && strings.ToLower(in.ReviewVersion) == in.ReviewVersion && cleanupReviewChecksum.MatchString(in.ReviewVersion) && in.Confirm && strings.TrimSpace(in.Reason) == in.Reason && utf8.RuneCountInString(in.Reason) >= 3 && utf8.RuneCountInString(in.Reason) <= 256
}
func retainProblem(e error) string {
	code := dto.CodeOf(e)
	if len(code) > 0 && len(code) < 96 && strings.ToUpper(code) == code && !strings.ContainsAny(code, " ./\\\n\r") {
		return code
	}
	if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return "BACKUP_RETAIN_INTERRUPTED"
	}
	return "BACKUP_RETAIN_UNAVAILABLE"
}

func (s *JobService) RetainCleanupReview(ctx context.Context, id uint64, in CleanupRetainInput, send func(CleanupRetainEvent) error) (CleanupRetainReceipt, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return CleanupRetainReceipt{}, e
	}
	if !validateRetainInput(id, in) || send == nil {
		return CleanupRetainReceipt{}, controlFailure(400, "BACKUP_RETAIN_INPUT_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	actor := reqctx.GetClaims(ctx).UserID
	key := uuid.MustParse(in.RequestID)
	digest := retainInputDigest(id, in)
	scope := s.retentionScope()
	check := func(r m.BackupCleanupRetention) error {
		if r.ActorID != actor {
			return controlFailure(403, "BACKUP_RETAIN_ACTOR_MISMATCH")
		}
		if r.InputDigest != digest || r.ArtifactID != id || r.ScopeFingerprint != scope || r.CleanupToken != in.ExpectedToken || r.ReviewVersion != in.ReviewVersion || r.Reason != in.Reason {
			return controlFailure(409, "BACKUP_RETAIN_IDEMPOTENCY_CONFLICT")
		}
		return nil
	}
	store, e := s.retentionLedger()
	if e != nil {
		return CleanupRetainReceipt{}, e
	}
	external, le := store.Retention(run, in.RequestID)
	if le != nil && !errors.Is(le, recovery.ErrRetentionNotFound) {
		return CleanupRetainReceipt{}, ledgerError(le)
	}
	if le == nil {
		if e = s.checkLedgerInput(ctx, external, id, in); e != nil {
			return CleanupRetainReceipt{}, e
		}
		if external.State != "checking" {
			return s.recoveryReceipt(ctx, external, external.Problem), nil
		}
	}
	if e = s.requireRetainSchema(run); e != nil {
		return CleanupRetainReceipt{}, e
	}
	old, e := s.retainRepo.Request(run, key)
	if e == nil {
		if e = check(old); e != nil {
			return CleanupRetainReceipt{}, e
		}
		if old.State != "checking" {
			if errors.Is(le, recovery.ErrRetentionNotFound) {
				reserve := retentionRecord(old, "")
				reserve.State = "checking"
				reserve.Problem = ""
				reserve.CompletedAt = nil
				if _, e = store.ReserveRetention(run, reserve); e != nil {
					return CleanupRetainReceipt{}, ledgerError(e)
				}
			}
			return s.persistRetainResult(run, in.RequestID)
		}
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		return CleanupRetainReceipt{}, controlFailure(503, "BACKUP_RETAIN_UNAVAILABLE")
	}
	if le == nil {
		generation, _ := s.retentionReviewBinding(run)
		v, err := s.resolveRetention(ctx, old, e, &external, generation)
		if err != nil {
			return CleanupRetainReceipt{}, err
		}
		if v.RecoveryBlocked {
			return v, nil
		}
	} else if e == nil { // Pre-v4 checking has no independent admission proof.
		return s.resolveRetention(ctx, old, nil, nil, "")
	}
	pool, e := s.db.DB()
	if e != nil || pool.Stats().MaxOpenConnections == 1 {
		return CleanupRetainReceipt{}, controlFailure(503, "BACKUP_REVIEW_DATABASE_POOL_INSUFFICIENT")
	}
	acquire, done := context.WithTimeout(run, 3*time.Second)
	takePolicy, release, e := lockCleanupRetention(acquire, s.db, in.RequestID)
	done()
	if e != nil {
		return CleanupRetainReceipt{}, e
	}
	defer release()
	if le == nil {
		current, e := s.retentionReviewBinding(run)
		if e != nil {
			return CleanupRetainReceipt{}, e
		}
		if current != external.Generation {
			return s.readRetentionRequest(run, in.RequestID)
		}
	}
	// Admit before preconditions so a failed or interrupted attempt can always be reconciled by its UUID.
	row, rowErr := s.verificationRow(run, id)
	candidate := m.BackupCleanupRetention{RequestID: key, ArtifactID: id, CleanupToken: in.ExpectedToken, ActorID: actor, InputDigest: digest, ReviewVersion: in.ReviewVersion, ScopeFingerprint: scope, PolicyID: row.PolicyID, JobID: row.JobID, Reason: in.Reason, State: "checking"}
	generation, ge := s.retentionReviewBinding(run)
	if ge != nil {
		return CleanupRetainReceipt{}, ge
	}
	if le == nil {
		generation = external.Generation
		candidate.CreatedAt = external.CreatedAt
	} else {
		candidate.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	}
	reserved, created, re := store.ReserveRetentionAdmission(run, retentionRecord(candidate, generation))
	if re != nil {
		return CleanupRetainReceipt{}, ledgerError(re)
	}
	if e = s.checkLedgerInput(ctx, reserved, id, in); e != nil {
		return CleanupRetainReceipt{}, e
	}
	if reserved.State != "checking" {
		return s.recoveryReceipt(ctx, reserved, reserved.Problem), nil
	}
	candidate.CreatedAt = reserved.CreatedAt
	if !created {
		external = reserved
		le = nil
	}
	bounded, finish := context.WithTimeout(run, 3*time.Second)
	historyChanged := errors.New("retention history changed across recovery")
	if le == nil {
		// A reservation observed before or during this attempt is never an
		// instruction to recreate SQL history. Pin the epoch while checking it.
		e = store.WithEpoch(bounded, func(recovery.Epoch) error {
			current, err := s.retentionReviewBinding(bounded)
			if err != nil {
				return err
			}
			if current != external.Generation {
				return historyChanged
			}
			old, err = s.retainRepo.Request(bounded, key)
			if err != nil || !recordMatchesRow(external, old) {
				return historyChanged
			}
			return nil
		})
	} else {
		e = s.db.WithContext(bounded).Transaction(func(tx *gorm.DB) error {
			var err error
			old, err = s.retainRepo.Admit(bounded, tx, &candidate)
			return err
		})
	}
	finish()
	if errors.Is(e, historyChanged) {
		return s.readRetentionRequest(run, in.RequestID)
	}
	if e != nil {
		return CleanupRetainReceipt{}, controlFailure(503, "BACKUP_RETAIN_RESULT_UNKNOWN")
	}
	if e = check(old); e != nil {
		return CleanupRetainReceipt{}, e
	}
	if old.State != "checking" {
		return s.persistRetainResult(run, in.RequestID)
	}
	s.cleanupSvc.notifyChanged()
	admitted, e := s.retainReceipt(ctx, old)
	if e != nil {
		return CleanupRetainReceipt{}, e
	}
	commitAttempted := false
	failure := func(err error) (CleanupRetainReceipt, error) {
		if commitAttempted {
			return CleanupRetainReceipt{}, controlFailure(503, "BACKUP_RETAIN_RESULT_UNKNOWN")
		}
		bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		// A resumed checking record might predate an unrecorded SQL commit.
		// If recovery crossed this attempt, preserve that historical uncertainty
		// instead of turning a restored checking row into a rejected history.
		finishRejected := func() error {
			return s.db.WithContext(bounded).Transaction(func(tx *gorm.DB) error { return s.retainRepo.Finish(bounded, tx, key, "rejected", retainProblem(err)) })
		}
		var finishErr error
		if le == nil {
			finishErr = store.WithEpoch(bounded, func(recovery.Epoch) error {
				current, e := s.retentionReviewBinding(bounded)
				if e != nil {
					return e
				}
				if current != external.Generation {
					return historyChanged
				}
				return finishRejected()
			})
		} else {
			finishErr = finishRejected()
		}
		if errors.Is(finishErr, historyChanged) {
			return s.readRetentionRequest(bounded, in.RequestID)
		}
		if finishErr != nil {
			return CleanupRetainReceipt{}, controlFailure(503, "BACKUP_RETAIN_RESULT_UNKNOWN")
		}
		s.cleanupSvc.notifyChanged()
		return s.persistRetainResult(bounded, in.RequestID)
	}
	if e = send(CleanupRetainEvent{Name: "accepted", Receipt: &admitted}); e != nil {
		return failure(e)
	}
	binding, bindingErr := s.retentionReviewBinding(run)
	if bindingErr != nil {
		return failure(bindingErr)
	}
	if rowErr != nil || row.CleanupToken != in.ExpectedToken || boundCleanupReviewVersion(row, binding) != in.ReviewVersion || !cleanupReviewItem(row).Inspectable || row.JobStatus != "success" || row.PolicyID == 0 {
		return failure(controlFailure(409, "BACKUP_RETAIN_REVIEW_CHANGED"))
	}
	if e = takePolicy(run, row.PolicyID); e != nil {
		return failure(controlFailure(409, "BACKUP_RETAIN_POLICY_BUSY"))
	}
	authority, ok := s.authority.(CleanupRetainAuthority)
	if !ok {
		return failure(controlFailure(428, "BACKUP_RETAIN_CONTROL_REQUIRED"))
	}
	guard := func(expected string, fn func(context.Context, *gorm.DB, string) error) error {
		return authority.RetainGuard(run, row.PolicyID, expected, func(c context.Context, tx *gorm.DB, _ *m.BackupPolicy, fence string) error {
			// RetainGuard pins the independent anchor throughout this transaction.
			currentBinding, e := s.retentionReviewBinding(c)
			if e != nil {
				return e
			}
			if currentBinding != binding {
				return controlFailure(409, "BACKUP_RETAIN_RECOVERY_CHANGED")
			}
			if e := s.retainRepo.LockSources(c, tx, id, row.JobID); e != nil {
				return controlFailure(409, "BACKUP_RETAIN_REVIEW_CHANGED")
			}
			current, e := repo.NewBackupArtifactRepository(tx).GetCleanupReview(c, id)
			if e != nil || !sameCleanupReview(row, current) {
				return controlFailure(409, "BACKUP_RETAIN_REVIEW_CHANGED")
			}
			active, e := s.retainRepo.HasActiveJobs(c, tx, row.PolicyID)
			if e != nil {
				return controlFailure(503, "BACKUP_RETAIN_ACTIVITY_UNAVAILABLE")
			}
			if active {
				return controlFailure(409, "BACKUP_RETAIN_POLICY_BUSY")
			}
			return fn(c, tx, fence)
		})
	}
	// Fence is captured once; a resumed request cannot cross a policy/execution/recovery version change.
	e = guard(old.Fence, func(c context.Context, tx *gorm.DB, fence string) error {
		if old.Fence == "" {
			v := tx.WithContext(c).Model(&m.BackupCleanupRetention{}).Where("request_id = ? AND state = ? AND fence = ?", key, "checking", "").Update("fence", fence)
			if v.Error != nil {
				return v.Error
			}
			if v.RowsAffected != 1 {
				return controlFailure(409, "BACKUP_RETAIN_REVIEW_CHANGED")
			}
			old.Fence = fence
		}
		return nil
	})
	if e != nil {
		return failure(e)
	}
	root, e := resolveBackupArtifactBaseDir()
	if e != nil {
		return failure(controlFailure(503, "BACKUP_RETAIN_UNAVAILABLE"))
	}
	out, e := verifyCleanupContentFinal(run, root, row, func(v CleanupVerification) error { return send(CleanupRetainEvent{Name: "progress", Progress: &v}) }, func(stable func() bool) error {
		return guard(old.Fence, func(c context.Context, tx *gorm.DB, _ string) error {
			receipt, e := s.retainRepo.LockRequest(c, tx, key)
			if e != nil || receipt.State != "checking" || check(receipt) != nil {
				return controlFailure(409, "BACKUP_RETAIN_REVIEW_CHANGED")
			}
			if !stable() {
				return controlFailure(409, "BACKUP_RETAIN_FILE_CHANGED")
			}
			if e = s.retainRepo.Protect(c, tx, row.JobID); e != nil {
				return e
			}
			if e = s.retainRepo.Finish(c, tx, key, "retained", ""); e != nil {
				return e
			}
			commitAttempted = true
			return nil
		})
	})
	if e != nil {
		return failure(e)
	}
	if out.State != "verified" || !out.ContentVerified {
		return failure(controlFailure(422, "BACKUP_RETAIN_CONTENT_"+strings.ToUpper(out.State)))
	}
	s.cleanupSvc.notifyChanged()
	readCtx, readCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer readCancel()
	receipt, e := s.persistRetainResult(readCtx, in.RequestID)
	if e != nil {
		return CleanupRetainReceipt{}, controlFailure(503, "BACKUP_RETAIN_RESULT_UNKNOWN")
	}
	return receipt, nil
}
