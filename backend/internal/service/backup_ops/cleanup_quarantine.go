package backup_ops

import (
	"context"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"gorm.io/gorm"
)

const cleanupQuarantineOperation = "quarantine_cleanup"

type quarantineExpectation struct {
	ExpectedDiagnosis string `json:"expected_diagnosis"`
	DiagnosticDigest  string `json:"diagnostic_digest"`
}
type CleanupQuarantineInput struct {
	CleanupResolutionInput
	ExpectedDiagnosis string `json:"expected_diagnosis"`
	DiagnosticDigest  string `json:"diagnostic_digest"`
}
type CleanupQuarantinePreview struct {
	Item             CleanupReviewItem `json:"item"`
	ReviewVersion    string            `json:"review_version"`
	HistoryDigest    string            `json:"history_digest"`
	Diagnosis        string            `json:"diagnosis"`
	DiagnosticDigest string            `json:"diagnostic_digest"`
	UnknownRecords   int               `json:"unknown_records"`
	CanQuarantine    bool              `json:"can_quarantine"`
	Problem          string            `json:"problem,omitempty"`
	ObservedAt       time.Time         `json:"observed_at"`
}

func quarantineInputDigest(id uint64, in CleanupResolutionInput, q quarantineExpectation) string {
	return historyHash(struct {
		Operation string
		Artifact  uint64
		Input     CleanupQuarantineInput
	}{cleanupQuarantineOperation, id, CleanupQuarantineInput{in, q.ExpectedDiagnosis, q.DiagnosticDigest}})
}
func quarantineVersion(row repo.CleanupReviewRow, binding, history string, q quarantineExpectation) string {
	return historyHash([]string{"powerx.cleanup-quarantine/v1", boundCleanupReviewVersion(row, binding), history, q.ExpectedDiagnosis, q.DiagnosticDigest})
}
func validQuarantineInput(id uint64, in CleanupQuarantineInput) bool {
	return validateResolutionInput(id, in.CleanupResolutionInput) && recovery.ValidQuarantineDiagnosis(in.ExpectedDiagnosis) && recoveryHash(in.DiagnosticDigest)
}
func (s *JobService) PreviewCleanupQuarantine(ctx context.Context, id uint64, token string, progress func(CleanupVerification) error) (CleanupQuarantinePreview, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return CleanupQuarantinePreview{}, e
	}
	if id == 0 || !validCleanupToken(token) || progress == nil {
		return CleanupQuarantinePreview{}, controlFailure(400, "BACKUP_QUARANTINE_INPUT_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if e := s.requireResolutionSchema(run); e != nil {
		return CleanupQuarantinePreview{}, e
	}
	row, e := s.verificationRow(run, id)
	if e != nil {
		return CleanupQuarantinePreview{}, reviewReadError(e)
	}
	if row.CleanupToken != token || !cleanupReviewItem(row).Inspectable || row.JobStatus != "success" || row.PolicyID == 0 {
		return CleanupQuarantinePreview{}, controlFailure(409, "BACKUP_QUARANTINE_INTENT_CHANGED")
	}
	pool, e := s.db.DB()
	if e != nil || pool.Stats().MaxOpenConnections == 1 {
		return CleanupQuarantinePreview{}, controlFailure(503, "BACKUP_REVIEW_DATABASE_POOL_INSUFFICIENT")
	}
	acquire, done := context.WithTimeout(run, 3*time.Second)
	unlock, e := lockBackupPolicy(acquire, s.db, row.PolicyID)
	done()
	if e != nil {
		return CleanupQuarantinePreview{}, controlFailure(409, "BACKUP_QUARANTINE_POLICY_BUSY")
	}
	defer unlock()
	binding, e := s.retentionReviewBinding(run)
	if e != nil {
		return CleanupQuarantinePreview{}, e
	}
	a, ok := s.authority.(CleanupRetainAuthority)
	if !ok {
		return CleanupQuarantinePreview{}, controlFailure(503, "BACKUP_RESOLUTION_CONTROL_REQUIRED")
	}
	out := CleanupQuarantinePreview{Item: cleanupReviewItem(row), ObservedAt: time.Now().UTC()}
	fence := ""
	guard := func(expected string) error {
		return a.RetainGuard(run, row.PolicyID, expected, func(c context.Context, tx *gorm.DB, _ *m.BackupPolicy, f string) error {
			current, e := s.retentionReviewBinding(c)
			if e != nil {
				return e
			}
			if current != binding {
				return controlFailure(409, "BACKUP_QUARANTINE_RECOVERY_CHANGED")
			}
			fresh, e := repo.NewBackupArtifactRepository(tx).GetCleanupReview(c, id)
			if e != nil || !sameCleanupReview(row, fresh) {
				return controlFailure(409, "BACKUP_QUARANTINE_INTENT_CHANGED")
			}
			history, n, e := s.resolutionHistory(c, tx, row, "", binding)
			if e != nil {
				return e
			}
			if out.HistoryDigest != "" && out.HistoryDigest != history {
				return controlFailure(409, "BACKUP_QUARANTINE_HISTORY_CHANGED")
			}
			out.HistoryDigest = history
			out.UnknownRecords = n
			active, e := s.retainRepo.HasActiveJobs(c, tx, row.PolicyID)
			if e != nil {
				return e
			}
			if active {
				return controlFailure(409, "BACKUP_QUARANTINE_POLICY_BUSY")
			}
			fence = f
			return nil
		})
	}
	if e = guard(""); e != nil {
		out.Problem = retainProblem(e)
		return out, nil
	}
	root, e := resolveBackupArtifactBaseDir()
	if e != nil {
		return out, controlFailure(503, "BACKUP_QUARANTINE_SOURCE_UNAVAILABLE")
	}
	probe, digest, e := probeQuarantine(run, root, row, progress, nil)
	if e != nil {
		return out, e
	}
	out.Diagnosis = probe.State
	out.DiagnosticDigest = digest
	out.ObservedAt = time.Now().UTC()
	if e = guard(fence); e != nil {
		out.Problem = retainProblem(e)
		return out, nil
	}
	if recovery.ValidQuarantineDiagnosis(out.Diagnosis) && recoveryHash(digest) {
		out.ReviewVersion = quarantineVersion(row, binding, out.HistoryDigest, quarantineExpectation{out.Diagnosis, digest})
		out.CanQuarantine = true
	} else {
		out.Problem = "BACKUP_QUARANTINE_DIAGNOSIS_NOT_ELIGIBLE"
	}
	return out, nil
}
func (s *JobService) ResolveCleanupQuarantine(ctx context.Context, id uint64, in CleanupQuarantineInput, send func(CleanupResolutionEvent) error) (CleanupResolutionReceipt, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return CleanupResolutionReceipt{}, e
	}
	if !validQuarantineInput(id, in) {
		return CleanupResolutionReceipt{}, controlFailure(400, "BACKUP_QUARANTINE_INPUT_INVALID")
	}
	return s.resolveCleanupDisposition(ctx, id, in.CleanupResolutionInput, &quarantineExpectation{in.ExpectedDiagnosis, in.DiagnosticDigest}, send)
}
func (s *JobService) CleanupQuarantineRequest(ctx context.Context, key string) (CleanupResolutionReceipt, error) {
	return s.cleanupDispositionRequest(ctx, key, cleanupQuarantineOperation)
}
func (s *JobService) CleanupQuarantineRequests(ctx context.Context, id uint64, token string) ([]CleanupResolutionReceipt, error) {
	return s.cleanupDispositionRequests(ctx, id, token, cleanupQuarantineOperation)
}
