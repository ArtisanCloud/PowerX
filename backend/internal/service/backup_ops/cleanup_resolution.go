package backup_ops

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const cleanupResolutionOperation = "retain_cleanup"

type CleanupResolutionPreview struct {
	Item           CleanupReviewItem `json:"item"`
	ReviewVersion  string            `json:"review_version"`
	HistoryDigest  string            `json:"history_digest"`
	UnknownRecords int               `json:"unknown_records"`
	CanResolve     bool              `json:"can_resolve"`
	Problem        string            `json:"problem,omitempty"`
	ObservedAt     time.Time         `json:"observed_at"`
}
type CleanupResolutionInput struct {
	RequestID     string `json:"request_id"`
	ExpectedToken string `json:"expected_token"`
	ReviewVersion string `json:"review_version"`
	HistoryDigest string `json:"history_digest"`
	Reason        string `json:"reason"`
	Confirm       bool   `json:"confirm"`
}
type CleanupResolutionReceipt struct {
	Operation                string     `json:"operation"`
	ExpectedDiagnosis        string     `json:"expected_diagnosis,omitempty"`
	DiagnosticDigest         string     `json:"diagnostic_digest,omitempty"`
	ValidBackupProof         bool       `json:"valid_backup_proof"`
	RequestID                string     `json:"request_id"`
	ArtifactID               string     `json:"artifact_id"`
	Token                    string     `json:"token"`
	ReviewVersion            string     `json:"review_version"`
	HistoryDigest            string     `json:"history_digest"`
	Reason                   string     `json:"reason"`
	ActorID                  string     `json:"actor_id"`
	State                    string     `json:"state"`
	Problem                  string     `json:"problem,omitempty"`
	CreatedAt                time.Time  `json:"created_at"`
	CompletedAt              *time.Time `json:"completed_at"`
	CanResume                bool       `json:"can_resume"`
	RecoveryBlocked          bool       `json:"recovery_blocked"`
	IndependentComplete      bool       `json:"independent_complete"`
	SQLCommitted             bool       `json:"sql_committed"`
	OriginalHistoryPreserved bool       `json:"original_history_preserved"`
}
type CleanupResolutionEvent struct {
	Name     string
	Receipt  *CleanupResolutionReceipt
	Progress *CleanupVerification
}

func resolutionInputDigest(id uint64, in CleanupResolutionInput) string {
	return historyHash(struct {
		Artifact uint64
		Input    CleanupResolutionInput
	}{id, in})
}
func resolutionRecord(r m.BackupCleanupResolution) recovery.RetentionRecord {
	if r.CompletedAt != nil {
		v := r.CompletedAt.UTC().Truncate(time.Microsecond)
		r.CompletedAt = &v
	}
	return recovery.RetentionRecord{Operation: resolutionOperation(r), ExpectedDiagnosis: r.ExpectedDiagnosis, DiagnosticDigest: r.DiagnosticDigest, HistoryDigest: r.HistoryDigest, JobID: r.JobID, PolicyID: r.PolicyID, RequestID: r.RequestID.String(), Scope: r.ScopeFingerprint, InputDigest: r.InputDigest, ReviewVersion: r.ReviewVersion, Generation: r.Generation, ArtifactID: r.ArtifactID, ActorID: r.ActorID, Token: r.CleanupToken, Reason: r.Reason, CreatedAt: r.CreatedAt.UTC().Truncate(time.Microsecond), State: r.State, Problem: r.Problem, CompletedAt: r.CompletedAt}
}
func resolutionOperation(r m.BackupCleanupResolution) string {
	if r.Operation == "" {
		return cleanupResolutionOperation
	}
	return r.Operation
}
func validResolutionRow(r m.BackupCleanupResolution) bool {
	in := CleanupResolutionInput{r.RequestID.String(), r.CleanupToken, r.ReviewVersion, r.HistoryDigest, r.Reason, true}
	digest := resolutionInputDigest(r.ArtifactID, in)
	if resolutionOperation(r) == cleanupQuarantineOperation {
		q := CleanupQuarantineInput{in, r.ExpectedDiagnosis, r.DiagnosticDigest}
		if !validQuarantineInput(r.ArtifactID, q) {
			return false
		}
		digest = quarantineInputDigest(r.ArtifactID, in, quarantineExpectation{q.ExpectedDiagnosis, q.DiagnosticDigest})
	}
	return !r.DeletedAt.Valid && recoveryHash(r.Fence) && validateResolutionInput(r.ArtifactID, in) && r.InputDigest == digest && recovery.ValidRetentionRecord(resolutionRecord(r))
}
func sameResolutionIdentity(a, b recovery.RetentionRecord) bool {
	a.State, b.State = "", ""
	a.Problem, b.Problem = "", ""
	a.CompletedAt, b.CompletedAt = nil, nil
	return reflect.DeepEqual(a, b)
}
func resolutionMatches(r recovery.RetentionRecord, row m.BackupCleanupResolution) bool {
	return validResolutionRow(row) && sameResolutionIdentity(r, resolutionRecord(row))
}
func validateResolutionInput(id uint64, in CleanupResolutionInput) bool {
	return validateRetainInput(id, CleanupRetainInput{in.RequestID, in.ExpectedToken, in.ReviewVersion, in.Reason, in.Confirm}) && len(in.HistoryDigest) == 64 && recoveryHash(in.HistoryDigest)
}
func recoveryHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func (s *JobService) requireResolutionSchema(ctx context.Context) error {
	if e := s.requireRetainSchema(ctx); e != nil {
		return e
	}
	if s.resolutionRepo == nil || !s.resolutionRepo.Ready(ctx) {
		return controlFailure(503, "BACKUP_RESOLUTION_SCHEMA_NOT_READY")
	}
	return nil
}
func resolutionReceipt(r recovery.RetentionRecord) CleanupResolutionReceipt {
	return CleanupResolutionReceipt{Operation: r.Operation, ExpectedDiagnosis: r.ExpectedDiagnosis, DiagnosticDigest: r.DiagnosticDigest, RequestID: r.RequestID, ArtifactID: strconv.FormatUint(r.ArtifactID, 10), Token: r.Token, ReviewVersion: r.ReviewVersion, HistoryDigest: r.HistoryDigest, Reason: r.Reason, ActorID: strconv.FormatUint(r.ActorID, 10), State: r.State, Problem: r.Problem, CreatedAt: r.CreatedAt, CompletedAt: r.CompletedAt, IndependentComplete: r.State != "checking", SQLCommitted: r.State == "retained" || r.State == "quarantined", OriginalHistoryPreserved: true}
}

// resolutionHistory 固定读取精确意图的全部有界历史；排除本次已登记身份，防止自身使快照漂移。
func (s *JobService) resolutionHistory(ctx context.Context, tx *gorm.DB, row repo.CleanupReviewRow, exclude, generation string) (string, int, error) {
	store, e := s.retentionLedger()
	if e != nil {
		return "", 0, e
	}
	inv, e := store.RetentionInventory(ctx)
	if e != nil {
		return "", 0, ledgerError(e)
	}
	legacy := []m.BackupCleanupRetention{}
	if e = tx.WithContext(ctx).Unscoped().Where("artifact_id=? AND cleanup_token=?", row.ID, row.CleanupToken).Order("request_id ASC").Limit(recovery.MaxRetentionRecords + 1).Find(&legacy).Error; e != nil {
		return "", 0, controlFailure(503, "BACKUP_RESOLUTION_HISTORY_UNAVAILABLE")
	}
	resolutions, e := s.resolutionRepo.ForIntent(ctx, tx, row.ID, row.CleanupToken, recovery.MaxRetentionRecords+1)
	if e != nil {
		return "", 0, controlFailure(503, "BACKUP_RESOLUTION_HISTORY_UNAVAILABLE")
	}
	if len(legacy) > recovery.MaxRetentionRecords || len(resolutions) > recovery.MaxRetentionRecords {
		return "", 0, controlFailure(423, "BACKUP_RESOLUTION_HISTORY_LIMIT")
	}
	independent := map[string]recovery.RetentionRecord{}
	unknown := 0
	for key, r := range inv.Records {
		if r.ArtifactID != row.ID || r.Token != row.CleanupToken || key == exclude {
			continue
		}
		if r.Scope != s.retentionScope() {
			return "", 0, controlFailure(409, "BACKUP_RESOLUTION_HISTORY_CONFLICT")
		}
		independent[key] = r
		if r.State == "checking" {
			unknown++
			if (r.Operation == cleanupResolutionOperation || r.Operation == cleanupQuarantineOperation) && r.Generation == generation {
				return "", 0, controlFailure(409, "BACKUP_RESOLUTION_ORIGINAL_REQUEST_REQUIRED")
			}
		}
	}
	for _, r := range legacy {
		coverage, _, _ := historyCoverage(r, inv.Records)
		if r.ScopeFingerprint != s.retentionScope() || coverage != "covered" {
			return "", 0, controlFailure(423, "BACKUP_RESOLUTION_HISTORY_COVERAGE_REQUIRED")
		}
	}
	prior := []m.BackupCleanupResolution{}
	for _, r := range resolutions {
		if r.RequestID.String() == exclude {
			continue
		}
		external, ok := independent[r.RequestID.String()]
		if !ok || !resolutionMatches(external, r) || external.State == "checking" && r.State != "checking" || external.State != "checking" && r.State != "checking" && !reflect.DeepEqual(external, resolutionRecord(r)) {
			return "", 0, controlFailure(409, "BACKUP_RESOLUTION_HISTORY_CONFLICT")
		}
		prior = append(prior, r)
	}
	return historyHash(struct {
		Source      string
		Legacy      []m.BackupCleanupRetention
		Resolutions []m.BackupCleanupResolution
		Independent map[string]recovery.RetentionRecord
	}{s.ExecutionFingerprint(), legacy, prior, independent}), unknown, nil
}
func resolutionVersion(row repo.CleanupReviewRow, binding, history string) string {
	return historyHash([]string{"powerx.cleanup-retain/v1", boundCleanupReviewVersion(row, binding), history})
}

func (s *JobService) PreviewCleanupResolution(ctx context.Context, id uint64, token string) (CleanupResolutionPreview, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return CleanupResolutionPreview{}, e
	}
	if id == 0 || !validCleanupToken(token) {
		return CleanupResolutionPreview{}, controlFailure(400, "BACKUP_RESOLUTION_INPUT_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if e := s.requireResolutionSchema(run); e != nil {
		return CleanupResolutionPreview{}, e
	}
	row, e := s.verificationRow(run, id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return CleanupResolutionPreview{}, controlFailure(409, "BACKUP_RESOLUTION_INTENT_CHANGED")
	}
	if e != nil {
		return CleanupResolutionPreview{}, reviewReadError(e)
	}
	if row.CleanupToken != token || !cleanupReviewItem(row).Inspectable || row.JobStatus != "success" || row.PolicyID == 0 {
		return CleanupResolutionPreview{}, controlFailure(409, "BACKUP_RESOLUTION_INTENT_CHANGED")
	}
	binding, e := s.retentionReviewBinding(run)
	if e != nil {
		return CleanupResolutionPreview{}, e
	}
	out := CleanupResolutionPreview{Item: cleanupReviewItem(row), ObservedAt: time.Now().UTC()}
	a, ok := s.authority.(CleanupRetainAuthority)
	if !ok {
		return out, controlFailure(503, "BACKUP_RESOLUTION_CONTROL_REQUIRED")
	}
	e = a.RetainGuard(run, row.PolicyID, "", func(c context.Context, tx *gorm.DB, _ *m.BackupPolicy, _ string) error {
		var err error
		out.HistoryDigest, out.UnknownRecords, err = s.resolutionHistory(c, tx, row, "", binding)
		if err != nil {
			return err
		}
		active, err := s.retainRepo.HasActiveJobs(c, tx, row.PolicyID)
		if err != nil {
			return err
		}
		if active {
			return controlFailure(409, "BACKUP_RESOLUTION_POLICY_BUSY")
		}
		return nil
	})
	if e != nil {
		out.Problem = retainProblem(e)
		return out, nil
	}
	out.ReviewVersion = resolutionVersion(row, binding, out.HistoryDigest)
	out.CanResolve = true
	return out, nil
}

func (s *JobService) CleanupResolutionRequest(ctx context.Context, key string) (CleanupResolutionReceipt, error) {
	return s.cleanupDispositionRequest(ctx, key, cleanupResolutionOperation)
}
func (s *JobService) cleanupDispositionRequest(ctx context.Context, key, operation string) (CleanupResolutionReceipt, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return CleanupResolutionReceipt{}, e
	}
	if !validCleanupToken(key) {
		return CleanupResolutionReceipt{}, controlFailure(400, "BACKUP_RESOLUTION_INPUT_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	store, e := s.retentionLedger()
	if e != nil {
		return CleanupResolutionReceipt{}, e
	}
	r, e := store.Retention(run, key)
	if errors.Is(e, recovery.ErrRetentionNotFound) || e == nil && r.Operation != operation {
		return CleanupResolutionReceipt{}, controlFailure(404, "BACKUP_RESOLUTION_REQUEST_NOT_FOUND")
	}
	if e != nil {
		return CleanupResolutionReceipt{}, ledgerError(e)
	}
	if r.Scope != s.retentionScope() {
		return CleanupResolutionReceipt{}, controlFailure(403, "BACKUP_RETAIN_SCOPE_MISMATCH")
	}
	out := resolutionReceipt(r)
	if out.IndependentComplete {
		return out, nil
	}
	row, se := s.resolutionRepo.Request(run, uuid.MustParse(key))
	generation, _ := s.retentionReviewBinding(run)
	return resolveCleanupResolution(ctx, r, row, se, generation), nil
}

func resolveCleanupResolution(ctx context.Context, r recovery.RetentionRecord, row m.BackupCleanupResolution, se error, generation string) CleanupResolutionReceipt {
	out := resolutionReceipt(r)
	if out.IndependentComplete {
		return out
	}
	if se != nil || !resolutionMatches(r, row) {
		out.RecoveryBlocked = true
		out.Problem = "BACKUP_RESOLUTION_SQL_HISTORY_UNAVAILABLE"
		return out
	}
	if row.State != "checking" {
		out.SQLCommitted = row.State == "retained" || row.State == "quarantined"
		out.Problem = "BACKUP_RESOLUTION_CHECKPOINT_REQUIRED"
		out.CanResume = r.ActorID == reqctx.GetClaims(ctx).UserID
		return out
	}
	out.RecoveryBlocked = generation == "" || generation != r.Generation
	if out.RecoveryBlocked {
		out.Problem = "BACKUP_RESOLUTION_RECOVERY_REVIEW_REQUIRED"
	} else {
		out.CanResume = r.ActorID == reqctx.GetClaims(ctx).UserID
	}
	return out
}
func (s *JobService) CleanupResolutionRequests(ctx context.Context, id uint64, token string) ([]CleanupResolutionReceipt, error) {
	return s.cleanupDispositionRequests(ctx, id, token, cleanupResolutionOperation)
}
func (s *JobService) cleanupDispositionRequests(ctx context.Context, id uint64, token, operation string) ([]CleanupResolutionReceipt, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return nil, e
	}
	if id == 0 || !validCleanupToken(token) {
		return nil, controlFailure(400, "BACKUP_RESOLUTION_INPUT_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	store, e := s.retentionLedger()
	if e != nil {
		return nil, e
	}
	keys, e := store.CleanupResolutions(run, s.retentionScope(), id, token)
	if e != nil {
		return nil, ledgerError(e)
	}
	filtered := keys[:0]
	for _, r := range keys {
		if r.Operation == operation {
			filtered = append(filtered, r)
		}
	}
	keys = filtered
	if len(keys) > 20 {
		keys = keys[:20]
	}
	ids := []uuid.UUID{}
	for _, r := range keys {
		if r.State == "checking" {
			ids = append(ids, uuid.MustParse(r.RequestID))
		}
	}
	rows, se := s.resolutionRepo.Requests(run, ids)
	byID := map[string]m.BackupCleanupResolution{}
	for _, r := range rows {
		byID[r.RequestID.String()] = r
	}
	generation := ""
	if len(ids) > 0 {
		generation, _ = s.retentionReviewBinding(run)
	}
	out := []CleanupResolutionReceipt{}
	for _, r := range keys {
		row, ok := byID[r.RequestID]
		err := se
		if err == nil && !ok {
			err = gorm.ErrRecordNotFound
		}
		v := resolveCleanupResolution(ctx, r, row, err, generation)
		out = append(out, v)
	}
	return out, nil
}
func (s *JobService) persistResolution(ctx context.Context, key string) (CleanupResolutionReceipt, error) {
	store, e := s.retentionLedger()
	if e != nil {
		return CleanupResolutionReceipt{}, e
	}
	r, e := store.Retention(ctx, key)
	if e != nil {
		return CleanupResolutionReceipt{}, ledgerError(e)
	}
	row, e := s.resolutionRepo.Request(ctx, uuid.MustParse(key))
	if e != nil || !resolutionMatches(r, row) || row.State == "checking" {
		return CleanupResolutionReceipt{}, controlFailure(503, "BACKUP_RESOLUTION_RESULT_UNKNOWN")
	}
	if e = store.CompleteRetention(ctx, resolutionRecord(row)); e != nil {
		return CleanupResolutionReceipt{}, controlFailure(503, "BACKUP_RESOLUTION_RESULT_UNKNOWN")
	}
	s.cleanupSvc.notifyChanged()
	return resolutionReceipt(resolutionRecord(row)), nil
}

func (s *JobService) ResolveCleanupRetained(ctx context.Context, id uint64, in CleanupResolutionInput, send func(CleanupResolutionEvent) error) (CleanupResolutionReceipt, error) {
	return s.resolveCleanupDisposition(ctx, id, in, nil, send)
}
func (s *JobService) resolveCleanupDisposition(ctx context.Context, id uint64, in CleanupResolutionInput, q *quarantineExpectation, send func(CleanupResolutionEvent) error) (CleanupResolutionReceipt, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return CleanupResolutionReceipt{}, e
	}
	if !validateResolutionInput(id, in) || send == nil {
		return CleanupResolutionReceipt{}, controlFailure(400, "BACKUP_RESOLUTION_INPUT_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	store, e := s.retentionLedger()
	if e != nil {
		return CleanupResolutionReceipt{}, e
	}
	actor := reqctx.GetClaims(ctx).UserID
	operation, targetState := cleanupResolutionOperation, "retained"
	digest := resolutionInputDigest(id, in)
	if q != nil {
		operation, targetState = cleanupQuarantineOperation, "quarantined"
		digest = quarantineInputDigest(id, in, *q)
	}
	key := uuid.MustParse(in.RequestID)
	check := func(r recovery.RetentionRecord) error {
		if r.ActorID != actor {
			return controlFailure(403, "BACKUP_RETAIN_ACTOR_MISMATCH")
		}
		if r.Operation != operation || r.Scope != s.retentionScope() || r.ArtifactID != id || r.InputDigest != digest || r.Token != in.ExpectedToken || r.ReviewVersion != in.ReviewVersion || r.HistoryDigest != in.HistoryDigest || r.Reason != in.Reason {
			return controlFailure(409, "BACKUP_RETAIN_IDEMPOTENCY_CONFLICT")
		}
		return nil
	}
	external, le := store.Retention(run, in.RequestID)
	if le == nil {
		if e = check(external); e != nil {
			return CleanupResolutionReceipt{}, e
		}
		if external.State != "checking" {
			return resolutionReceipt(external), nil
		}
	} else if !errors.Is(le, recovery.ErrRetentionNotFound) {
		return CleanupResolutionReceipt{}, ledgerError(le)
	}
	if e = s.requireResolutionSchema(run); e != nil {
		return CleanupResolutionReceipt{}, e
	}
	pool, poolErr := s.db.DB()
	if poolErr != nil || pool.Stats().MaxOpenConnections == 1 {
		return CleanupResolutionReceipt{}, controlFailure(503, "BACKUP_REVIEW_DATABASE_POOL_INSUFFICIENT")
	}
	acquire, done := context.WithTimeout(run, 3*time.Second)
	takePolicy, release, e := lockCleanupRetention(acquire, s.db, in.RequestID)
	done()
	if e != nil {
		return CleanupResolutionReceipt{}, e
	}
	defer release()
	old, se := s.resolutionRepo.Request(run, key)
	if le == nil {
		if se != nil || !resolutionMatches(external, old) {
			return s.cleanupDispositionRequest(run, in.RequestID, operation)
		}
		if old.State != "checking" {
			return s.persistResolution(run, in.RequestID)
		}
		binding, err := s.retentionReviewBinding(run)
		if err != nil || binding != external.Generation {
			return s.cleanupDispositionRequest(run, in.RequestID, operation)
		}
	} else if se == nil || !errors.Is(se, gorm.ErrRecordNotFound) {
		return CleanupResolutionReceipt{}, controlFailure(503, "BACKUP_RESOLUTION_SQL_HISTORY_UNAVAILABLE")
	}
	row, e := s.verificationRow(run, id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return CleanupResolutionReceipt{}, controlFailure(409, "BACKUP_RESOLUTION_INTENT_CHANGED")
	}
	if e != nil {
		return CleanupResolutionReceipt{}, reviewReadError(e)
	}
	if row.CleanupToken != in.ExpectedToken || !cleanupReviewItem(row).Inspectable || row.JobStatus != "success" || row.PolicyID == 0 {
		return CleanupResolutionReceipt{}, controlFailure(409, "BACKUP_RESOLUTION_INTENT_CHANGED")
	}
	if e = takePolicy(run, row.PolicyID); e != nil {
		return CleanupResolutionReceipt{}, controlFailure(409, "BACKUP_RESOLUTION_POLICY_BUSY")
	}
	binding, e := s.retentionReviewBinding(run)
	if e != nil {
		return CleanupResolutionReceipt{}, e
	}
	authority, ok := s.authority.(CleanupRetainAuthority)
	if !ok {
		return CleanupResolutionReceipt{}, controlFailure(503, "BACKUP_RESOLUTION_CONTROL_REQUIRED")
	}
	guard := func(fence string, fn func(context.Context, *gorm.DB, string) error) error {
		return authority.RetainGuard(run, row.PolicyID, fence, func(c context.Context, tx *gorm.DB, _ *m.BackupPolicy, f string) error {
			current, e := s.retentionReviewBinding(c)
			if e != nil {
				return e
			}
			if current != binding {
				return controlFailure(409, "BACKUP_RESOLUTION_RECOVERY_CHANGED")
			}
			if e = s.retainRepo.LockSources(c, tx, id, row.JobID); e != nil {
				return e
			}
			fresh, e := repo.NewBackupArtifactRepository(tx).GetCleanupReview(c, id)
			if e != nil || !sameCleanupReview(row, fresh) {
				return controlFailure(409, "BACKUP_RESOLUTION_INTENT_CHANGED")
			}
			history, _, e := s.resolutionHistory(c, tx, row, in.RequestID, binding)
			if e != nil {
				return e
			}
			version := resolutionVersion(row, binding, history)
			if q != nil {
				version = quarantineVersion(row, binding, history, *q)
			}
			if history != in.HistoryDigest || version != in.ReviewVersion {
				return controlFailure(409, "BACKUP_RESOLUTION_REVIEW_CHANGED")
			}
			active, e := s.retainRepo.HasActiveJobs(c, tx, row.PolicyID)
			if e != nil {
				return e
			}
			if active {
				return controlFailure(409, "BACKUP_RESOLUTION_POLICY_BUSY")
			}
			return fn(c, tx, f)
		})
	}
	if errors.Is(le, recovery.ErrRetentionNotFound) {
		if e = guard("", func(context.Context, *gorm.DB, string) error { return nil }); e != nil {
			return CleanupResolutionReceipt{}, e
		}
		old = m.BackupCleanupResolution{Operation: operation, RequestID: key, ArtifactID: id, JobID: row.JobID, PolicyID: row.PolicyID, CleanupToken: in.ExpectedToken, ActorID: actor, InputDigest: digest, ReviewVersion: in.ReviewVersion, HistoryDigest: in.HistoryDigest, ScopeFingerprint: s.retentionScope(), Generation: binding, Reason: in.Reason, State: "checking"}
		if q != nil {
			old.ExpectedDiagnosis = q.ExpectedDiagnosis
			old.DiagnosticDigest = q.DiagnosticDigest
		}
		old.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
		reserved, created, e := store.ReserveRetentionAdmission(run, resolutionRecord(old))
		if e != nil {
			return CleanupResolutionReceipt{}, ledgerError(e)
		}
		if e = check(reserved); e != nil {
			return CleanupResolutionReceipt{}, e
		}
		if !created {
			return s.cleanupDispositionRequest(run, in.RequestID, operation)
		}
		e = guard("", func(c context.Context, tx *gorm.DB, f string) error {
			old.Fence = f
			return s.resolutionRepo.Create(c, tx, &old)
		})
		if e != nil {
			return CleanupResolutionReceipt{}, controlFailure(503, "BACKUP_RESOLUTION_RESULT_UNKNOWN")
		}
	}
	admitted := resolutionReceipt(resolutionRecord(old))
	admitted.CanResume = true
	commitAttempted := false
	failure := func(cause error) (CleanupResolutionReceipt, error) {
		if commitAttempted {
			return CleanupResolutionReceipt{}, controlFailure(503, "BACKUP_RESOLUTION_RESULT_UNKNOWN")
		}
		bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		e := store.WithEpoch(bounded, func(epoch recovery.Epoch) error {
			if recovery.RetentionGeneration(s.retentionScope(), epoch) != binding {
				return recovery.ErrUnavailable
			}
			return s.db.WithContext(bounded).Transaction(func(tx *gorm.DB) error {
				r, e := s.resolutionRepo.LockRequest(bounded, tx, key)
				if e != nil || !resolutionMatches(resolutionRecord(old), r) || r.State != "checking" {
					return recovery.ErrUnavailable
				}
				return s.resolutionRepo.Finish(bounded, tx, key, "rejected", retainProblem(cause))
			})
		})
		if e != nil {
			return CleanupResolutionReceipt{}, controlFailure(503, "BACKUP_RESOLUTION_RESULT_UNKNOWN")
		}
		return s.persistResolution(bounded, in.RequestID)
	}
	s.cleanupSvc.notifyChanged()
	if e = send(CleanupResolutionEvent{Name: "accepted", Receipt: &admitted}); e != nil {
		return CleanupResolutionReceipt{}, controlFailure(503, "BACKUP_RESOLUTION_RESULT_UNKNOWN")
	}
	root, e := resolveBackupArtifactBaseDir()
	if e != nil {
		return failure(e)
	}
	commit := func(stable func() bool) error {
		return guard(old.Fence, func(c context.Context, tx *gorm.DB, _ string) error {
			r, e := s.resolutionRepo.LockRequest(c, tx, key)
			if e != nil || !resolutionMatches(resolutionRecord(old), r) || r.State != "checking" {
				return controlFailure(409, "BACKUP_RESOLUTION_INTENT_CHANGED")
			}
			if !stable() {
				return controlFailure(409, "BACKUP_RESOLUTION_FILE_CHANGED")
			}
			if q == nil {
				if e = s.retainRepo.Protect(c, tx, row.JobID); e != nil {
					return e
				}
			}
			if e = s.resolutionRepo.DisposeCurrentIntent(c, tx, id, in.ExpectedToken, row.UpdatedAt, targetState); e != nil {
				return e
			}
			if e = s.resolutionRepo.Finish(c, tx, key, targetState, ""); e != nil {
				return e
			}
			commitAttempted = true
			return nil
		})
	}
	progress := func(v CleanupVerification) error { return send(CleanupResolutionEvent{Name: "progress", Progress: &v}) }
	var out CleanupVerification
	if q == nil {
		out, e = verifyCleanupContentFinal(run, root, row, progress, commit)
	} else {
		var digest string
		out, digest, e = probeQuarantine(run, root, row, progress, func(kind, digest string, stable func() bool) error {
			if kind != q.ExpectedDiagnosis || digest != q.DiagnosticDigest {
				return controlFailure(409, "BACKUP_QUARANTINE_DIAGNOSIS_CHANGED")
			}
			return commit(stable)
		})
		if e == nil && (out.State != q.ExpectedDiagnosis || digest != q.DiagnosticDigest) {
			e = controlFailure(409, "BACKUP_QUARANTINE_DIAGNOSIS_CHANGED")
		}
	}
	if e != nil {
		return failure(e)
	}
	if q == nil && (out.State != "verified" || !out.ContentVerified) {
		return failure(controlFailure(422, "BACKUP_RESOLUTION_CONTENT_"+strings.ToUpper(out.State)))
	}
	readCtx, readCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer readCancel()
	return s.persistResolution(readCtx, in.RequestID)
}
