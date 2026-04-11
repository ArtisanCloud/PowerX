package backup_ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	inst "github.com/ArtisanCloud/PowerX/internal/service/backup_ops/instrumentation"
	obsops "github.com/ArtisanCloud/PowerX/internal/service/observability_ops"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"gorm.io/gorm"
)

type JobService struct {
	policyRepo *repoops.BackupPolicyRepository
	jobRepo    *repoops.BackupJobRepository
	runner     ScriptRunner
	auditor    obsops.AuditWriter
	alertSvc   *AlertService
	cleanupSvc *ArtifactCleanupService
	scriptDir  string
	metrics    *inst.Recorder
	lockMu     sync.Mutex
	policyLock map[uint64]struct{}
	nextRuns   map[uint64]time.Time
}

type TriggerJobRequest struct {
	PolicyID    uint64
	Operator    string
	TraceID     string
	ForceSync   bool
	TriggerType modelops.BackupTriggerType
}

type ListJobOptions struct {
	PolicyID uint64
	Status   string
	From     *time.Time
	To       *time.Time
	Page     int
	PageSize int
}

func NewJobService(db *gorm.DB) *JobService {
	scriptDir := strings.TrimSpace(os.Getenv("POWERX_OPS_SCRIPT_DIR"))
	if scriptDir == "" {
		scriptDir = filepath.Join("backend", "scripts", "ops")
	}
	return &JobService{
		policyRepo: repoops.NewBackupPolicyRepository(db),
		jobRepo:    repoops.NewBackupJobRepository(db),
		runner:     NewOSScriptRunner(),
		auditor:    obsops.NewUnifiedAuditWriter(db),
		alertSvc:   NewAlertService(db),
		cleanupSvc: NewArtifactCleanupService(db),
		scriptDir:  scriptDir,
		metrics:    inst.NewRecorder("powerx.service.backup_job_ops"),
		policyLock: make(map[uint64]struct{}),
		nextRuns:   make(map[uint64]time.Time),
	}
}

func (s *JobService) ListJobs(ctx context.Context, opt ListJobOptions) ([]modelops.BackupJob, int64, error) {
	page := opt.Page
	if page <= 0 {
		page = 1
	}
	size := opt.PageSize
	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	offset := (page - 1) * size
	return s.jobRepo.ListWithFilters(ctx, opt.PolicyID, opt.Status, opt.From, opt.To, size, offset)
}

func (s *JobService) GetJob(ctx context.Context, jobID uint64) (*modelops.BackupJob, error) {
	if jobID == 0 {
		return nil, ErrInvalidBackupRequest
	}
	row, err := s.jobRepo.GetById(ctx, jobID, nil)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrBackupJobNotFound
	}
	return row, nil
}

func (s *JobService) TriggerJob(ctx context.Context, req TriggerJobRequest) (*modelops.BackupJob, error) {
	startedAt := time.Now()
	var retErr error
	defer func() { s.metrics.Observe(ctx, "backup_trigger_job", startedAt, retErr) }()

	if req.PolicyID == 0 {
		retErr = ErrInvalidBackupRequest
		return nil, retErr
	}
	if !s.tryLockPolicy(req.PolicyID) {
		retErr = ErrBackupJobAlreadyRunning
		return nil, retErr
	}
	defer s.unlockPolicy(req.PolicyID)

	policy, err := s.policyRepo.GetById(ctx, req.PolicyID, nil)
	if err != nil {
		retErr = err
		return nil, retErr
	}
	if policy == nil {
		retErr = ErrBackupPolicyNotFound
		return nil, retErr
	}
	running, err := s.jobRepo.ExistsRunningByPolicy(ctx, req.PolicyID)
	if err != nil {
		retErr = err
		return nil, retErr
	}
	if running {
		retErr = ErrBackupJobAlreadyRunning
		return nil, retErr
	}

	now := time.Now().UTC()
	triggerType := req.TriggerType
	if strings.TrimSpace(string(triggerType)) == "" {
		triggerType = modelops.BackupTriggerTypeManual
	}
	job := &modelops.BackupJob{
		PolicyID:    req.PolicyID,
		Status:      modelops.BackupJobStatusRunning,
		TriggerType: triggerType,
		StartedAt:   &now,
		Operator:    normalizeOperator(req.Operator),
		TraceID:     strings.TrimSpace(req.TraceID),
	}
	job.Normalize()
	saved, err := s.jobRepo.Create(ctx, job)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	execErr := s.runBackupScript(ctx, saved.PolicyID)
	ended := time.Now().UTC()
	saved.EndedAt = &ended
	if execErr != nil {
		saved.Status = modelops.BackupJobStatusFailed
		saved.ErrorMessage = execErr.Error()
	} else {
		saved.Status = modelops.BackupJobStatusSuccess
	}
	updated, err := s.jobRepo.Update(ctx, saved)
	if err != nil {
		retErr = err
		return nil, retErr
	}
	if updated.Status == modelops.BackupJobStatusFailed && s.alertSvc != nil {
		_ = s.alertSvc.HandleJobCompletionAlert(ctx, updated)
	}
	if updated.Status == modelops.BackupJobStatusSuccess && s.cleanupSvc != nil {
		if cleanupRet, cleanupErr := s.cleanupSvc.CleanupByPolicy(ctx, policy.ID, int(policy.RetentionCount)); cleanupErr != nil {
			if s.alertSvc != nil {
				_ = s.alertSvc.CreateCleanupFailureAlert(ctx, policy.ID, updated.TraceID, cleanupErr)
			}
		} else {
			s.audit(ctx, obsops.AuditRecord{
				ResourceType: "backup_cleanup",
				ResourceID:   fmt.Sprintf("%d", policy.ID),
				Operation:    "cleanup",
				Outcome:      "success",
				Severity:     "info",
				Detail: map[string]any{
					"policy_id":           policy.ID,
					"deleted_jobs":        cleanupRet.DeletedJobs,
					"deleted_artifacts":   cleanupRet.DeletedArtifacts,
					"triggered_by_job_id": updated.ID,
				},
			})
		}
	}

	s.audit(ctx, obsops.AuditRecord{ResourceType: "backup_job", ResourceID: fmt.Sprintf("%d", updated.ID), Operation: "execute", Outcome: string(updated.Status), Severity: "info", Detail: map[string]any{"policy_id": updated.PolicyID, "status": updated.Status, "operator": updated.Operator, "trigger_type": updated.TriggerType, "trace_id": updated.TraceID}})
	return updated, nil
}

func (s *JobService) TriggerCleanup(ctx context.Context, operator, traceID string) error {
	startedAt := time.Now()
	err := s.runOptionalScript(ctx, "cleanup-backups.sh", nil)
	if err == nil && s.cleanupSvc != nil {
		_, err = s.cleanupSvc.CleanupAllPolicies(ctx)
	}
	s.metrics.Observe(ctx, "backup_trigger_cleanup", startedAt, err)
	outcome := "success"
	if err != nil {
		outcome = "failed"
		if s.alertSvc != nil {
			_ = s.alertSvc.CreateCleanupFailureAlert(ctx, 0, traceID, err)
		}
	}
	s.audit(ctx, obsops.AuditRecord{ResourceType: "backup_cleanup", ResourceID: "cleanup", Operation: "cleanup", Outcome: outcome, Severity: "info", Detail: map[string]any{"operator": normalizeOperator(operator), "trace_id": strings.TrimSpace(traceID)}})
	return err
}

func (s *JobService) RegisterPolicyScheduler(ctx context.Context, tick time.Duration) {
	if tick <= 0 {
		tick = 30 * time.Second
	}
	ticker := time.NewTicker(tick)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.scanAndTrigger(ctx)
			}
		}
	}()
}

func (s *JobService) runBackupScript(ctx context.Context, policyID uint64) error {
	args := []string{strconv.FormatUint(policyID, 10)}
	return s.runOptionalScript(ctx, "backup-db.sh", args)
}

func (s *JobService) scanAndTrigger(ctx context.Context) {
	enabled := true
	policies, _, err := s.policyRepo.List(ctx, &enabled, 500, 0)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for i := range policies {
		p := policies[i]
		nextAt := s.nextRunAt(p.ID, now, p.Schedule)
		if now.Before(nextAt) {
			continue
		}
		_, _ = s.TriggerJob(ctx, TriggerJobRequest{
			PolicyID:    p.ID,
			Operator:    "system.scheduler",
			TriggerType: modelops.BackupTriggerTypeScheduled,
		})
		s.setNextRunAt(p.ID, now.Add(parseScheduleDuration(p.Schedule)))
	}
}

func (s *JobService) nextRunAt(policyID uint64, now time.Time, schedule string) time.Time {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	if v, ok := s.nextRuns[policyID]; ok && !v.IsZero() {
		return v
	}
	v := now.Add(parseScheduleDuration(schedule))
	s.nextRuns[policyID] = v
	return v
}

func (s *JobService) setNextRunAt(policyID uint64, next time.Time) {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	s.nextRuns[policyID] = next
}

func parseScheduleDuration(schedule string) time.Duration {
	schedule = strings.TrimSpace(strings.ToLower(schedule))
	if schedule == "" {
		return 6 * time.Hour
	}
	if d, err := time.ParseDuration(schedule); err == nil && d > 0 {
		return d
	}
	return 6 * time.Hour
}

func (s *JobService) tryLockPolicy(policyID uint64) bool {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	if _, ok := s.policyLock[policyID]; ok {
		return false
	}
	s.policyLock[policyID] = struct{}{}
	return true
}

func (s *JobService) unlockPolicy(policyID uint64) {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	delete(s.policyLock, policyID)
}

func (s *JobService) runOptionalScript(ctx context.Context, scriptName string, args []string) error {
	if s.runner == nil {
		return nil
	}
	path := filepath.Join(s.scriptDir, scriptName)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	_, err := s.runner.Run(ctx, ScriptSpec{Command: path, Args: args, Timeout: 2 * time.Minute})
	return err
}

func (s *JobService) audit(ctx context.Context, rec obsops.AuditRecord) {
	if s.auditor == nil {
		return
	}
	_ = s.auditor.Write(ctx, rec)
}
