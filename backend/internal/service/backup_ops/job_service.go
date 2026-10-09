package backup_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	inst "github.com/ArtisanCloud/PowerX/internal/service/backup_ops/instrumentation"
	obsops "github.com/ArtisanCloud/PowerX/internal/service/observability_ops"
	opsscripts "github.com/ArtisanCloud/PowerX/internal/service/ops_scripts"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type JobService struct {
	db                  *gorm.DB
	policyRepo          *repoops.BackupPolicyRepository
	jobRepo             *repoops.BackupJobRepository
	artifactRepo        *repoops.BackupArtifactRepository
	runner              ScriptRunner
	auditor             obsops.AuditWriter
	alertSvc            *AlertService
	cleanupSvc          *ArtifactCleanupService
	restoreSvc          *RestoreDrillService
	scriptDir           string
	artifactBaseDir     string
	artifactBaseDirErr  error
	backupScriptTimeout time.Duration
	metrics             *inst.Recorder
	lockMu              sync.Mutex
	policyLock          map[uint64]struct{}
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
	scriptDir := backupScriptDir("backup-db.sh")
	artifactBaseDir, artifactBaseDirErr := resolveBackupArtifactBaseDir()
	logOp(context.Background(), "info", "backup.job_service.config",
		zap.String("script_dir", scriptDir),
		zap.String("backup_script", filepath.Join(scriptDir, "backup-db.sh")),
		zap.String("artifact_base_dir", artifactBaseDir),
		zap.Error(artifactBaseDirErr),
		zap.String("env_POWERX_OPS_SCRIPT_DIR", strings.TrimSpace(os.Getenv("POWERX_OPS_SCRIPT_DIR"))),
	)
	return &JobService{
		db:                  db,
		policyRepo:          repoops.NewBackupPolicyRepository(db),
		jobRepo:             repoops.NewBackupJobRepository(db),
		artifactRepo:        repoops.NewBackupArtifactRepository(db),
		runner:              NewOSScriptRunner(),
		auditor:             obsops.NewUnifiedAuditWriter(db),
		alertSvc:            NewAlertService(db),
		cleanupSvc:          NewArtifactCleanupService(db),
		restoreSvc:          NewRestoreDrillService(db),
		scriptDir:           scriptDir,
		artifactBaseDir:     artifactBaseDir,
		artifactBaseDirErr:  artifactBaseDirErr,
		backupScriptTimeout: resolveBackupScriptTimeout(),
		metrics:             inst.NewRecorder("powerx.service.backup_job_ops"),
		policyLock:          make(map[uint64]struct{}),
	}
}

// resolveBackupArtifactBaseDir 将备份产物放在稳定的数据目录，独立于发布版本和工作目录。
func resolveBackupArtifactBaseDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("POWERX_OPS_BACKUP_ARTIFACT_DIR")); dir != "" {
		if !filepath.IsAbs(dir) {
			return "", fmt.Errorf("POWERX_OPS_BACKUP_ARTIFACT_DIR must be an absolute path")
		}
		return filepath.Clean(dir), nil
	}
	if root := strings.TrimSpace(os.Getenv("POWERX_LINKS_ROOT")); root != "" {
		if !filepath.IsAbs(root) {
			return "", fmt.Errorf("POWERX_LINKS_ROOT must be an absolute path")
		}
		return filepath.Join(root, "storage", "backups"), nil
	}
	// 本地运行也不把数据库备份放入仓库或临时构建目录。
	homeDir, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(homeDir) {
		return "", fmt.Errorf("backup data directory unavailable: configure POWERX_OPS_BACKUP_ARTIFACT_DIR")
	}
	return filepath.Join(homeDir, ".powerx", "storage", "backups"), nil
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
	unlockDB, lockErr := lockBackupPolicy(ctx, s.db, req.PolicyID)
	if lockErr != nil {
		return nil, lockErr
	}
	defer unlockDB()
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
	if req.TriggerType == modelops.BackupTriggerTypeScheduled {
		due, e := s.policyDue(ctx, policy, time.Now().UTC())
		if e != nil {
			return nil, e
		}
		if !due {
			return nil, nil
		}
	}
	if err := s.jobRepo.FailInterrupted(ctx, req.PolicyID, time.Now().UTC().Add(-s.backupScriptTimeout-10*time.Minute)); err != nil {
		return nil, err
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

	artifactPath, prepErr := s.prepareArtifactPath(ctx, saved, policy)
	execErr := prepErr
	if execErr == nil {
		execErr = s.runBackupScript(ctx, saved.PolicyID, artifactPath)
	}
	if execErr == nil {
		if artErr := s.persistBackupArtifact(ctx, saved, artifactPath); artErr != nil {
			execErr = fmt.Errorf("persist backup artifact failed: %w", artErr)
		}
	}
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
		if cleanupRet, cleanupErr := s.cleanupSvc.cleanupPolicy(ctx, policy); cleanupErr != nil {
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
	if updated.Status == modelops.BackupJobStatusSuccess && policy.DrillEnabled && s.restoreSvc != nil {
		interval := int(policy.DrillIntervalDays)
		if interval <= 0 {
			interval = 7
		}
		shouldTrigger, shouldErr := s.restoreSvc.ShouldTriggerByPolicy(ctx, policy.ID, interval, time.Now().UTC())
		if shouldErr == nil && shouldTrigger {
			_, _ = s.restoreSvc.trigger(ctx, TriggerRestoreDrillRequest{
				SourceJobID: updated.ID,
				Reason:      "scheduled_by_policy",
				Operator:    "system.scheduler",
				TraceID:     updated.TraceID,
			}, true)
		}
	}

	s.audit(ctx, obsops.AuditRecord{ResourceType: "backup_job", ResourceID: fmt.Sprintf("%d", updated.ID), Operation: "execute", Outcome: string(updated.Status), Severity: "info", Detail: map[string]any{"policy_id": updated.PolicyID, "status": updated.Status, "operator": updated.Operator, "trigger_type": updated.TriggerType, "trace_id": updated.TraceID}})
	logOp(ctx, "info", "backup.job.execute",
		zap.Uint64("job_id", updated.ID),
		zap.Uint64("policy_id", updated.PolicyID),
		zap.String("status", string(updated.Status)),
		zap.String("trigger_type", string(updated.TriggerType)),
		zap.String("trace_id", strings.TrimSpace(updated.TraceID)),
		zap.Bool("script_failed", execErr != nil),
	)
	publishBackupJobStatus(ctx, map[string]any{
		"job_id":       toStringUint(updated.ID),
		"policy_id":    toStringUint(updated.PolicyID),
		"status":       string(updated.Status),
		"trigger_type": string(updated.TriggerType),
		"trace_id":     strings.TrimSpace(updated.TraceID),
		"operator":     strings.TrimSpace(updated.Operator),
	})
	return updated, nil
}

func (s *JobService) TriggerCleanup(ctx context.Context, operator, traceID string) error {
	startedAt := time.Now()
	var err error
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
	logOp(ctx, "info", "backup.cleanup.trigger",
		zap.String("outcome", outcome),
		zap.String("operator", normalizeOperator(operator)),
		zap.String("trace_id", strings.TrimSpace(traceID)),
	)
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

func (s *JobService) runBackupScript(ctx context.Context, policyID uint64, outputPath string) error {
	if s.runner == nil {
		return fmt.Errorf("backup script runner unavailable")
	}
	path := filepath.Join(s.scriptDir, "backup-db.sh")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("backup script unavailable: %w", err)
	}
	args := []string{strconv.FormatUint(policyID, 10), outputPath}
	spec := ScriptSpec{
		Command: path,
		Args:    args,
		Timeout: s.backupScriptTimeout,
	}
	env, err := postgresEnvironment(sourceDSN(s.db))
	if err != nil {
		return err
	}
	spec.Env = env
	res, err := s.runner.Run(ctx, spec)
	if err != nil && res != nil {
		return fmt.Errorf("backup failed: %w; %s", err, strings.TrimSpace(res.Stderr))
	}
	return err
}

func (s *JobService) prepareArtifactPath(ctx context.Context, job *modelops.BackupJob, policy *modelops.BackupPolicy) (string, error) {
	if job == nil || policy == nil || job.ID == 0 || policy.ID == 0 {
		return "", ErrInvalidBackupRequest
	}
	if s.artifactBaseDirErr != nil {
		return "", fmt.Errorf("backup artifact directory unavailable: %w", s.artifactBaseDirErr)
	}
	if !filepath.IsAbs(s.artifactBaseDir) {
		return "", fmt.Errorf("backup artifact directory must be an absolute path")
	}
	now := time.Now().UTC()
	tenantUUID := sanitizePathSegment(reqctx.GetTenantUUID(ctx))
	if tenantUUID == "" {
		tenantUUID = "tenant_unknown"
	}
	dir := filepath.Join(
		s.artifactBaseDir,
		tenantUUID,
		fmt.Sprintf("policy_%d", policy.ID),
		now.Format("2006"),
		now.Format("01"),
		now.Format("02"),
	)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("job_%d_%s.dump", job.ID, now.Format("20060102T150405Z"))
	return filepath.Join(dir, filename), nil
}

func (s *JobService) persistBackupArtifact(ctx context.Context, job *modelops.BackupJob, artifactPath string) error {
	if job == nil || job.ID == 0 || strings.TrimSpace(artifactPath) == "" {
		return ErrInvalidBackupRequest
	}

	stat, err := os.Stat(artifactPath)
	if err != nil {
		return fmt.Errorf("backup artifact missing: %w", err)
	}
	if stat.IsDir() {
		return fmt.Errorf("backup artifact is directory: %s", artifactPath)
	}
	if stat.Size() <= 0 {
		return fmt.Errorf("backup artifact is empty: %s", artifactPath)
	}
	file, err := os.Open(artifactPath)
	if err != nil {
		return err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return err
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))
	absPath, err := filepath.Abs(artifactPath)
	if err != nil {
		absPath = artifactPath
	}
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}).String()
	row := &modelops.BackupArtifact{
		JobID:       job.ID,
		StorageURI:  uri,
		SizeBytes:   stat.Size(),
		Checksum:    checksum,
		ContentType: "application/postgresql-custom",
	}
	if err := writeArtifactSidecars(absPath, sourceDSN(s.db), job, row); err != nil {
		return fmt.Errorf("write backup manifest: %w", err)
	}
	row.Normalize()
	_, err = s.artifactRepo.Create(ctx, row)
	return err
}

// RunDue 从任务持久化时间推导下一次执行；重启不会重新开始整个等待周期。
func (s *JobService) RunDue(ctx context.Context) error {
	enabled := true
	policies, _, err := s.policyRepo.List(ctx, &enabled, 0, 0)
	if err != nil {
		return err
	}
	schedulerCtx := s.ensureTenantContext(ctx)
	var failures []error
	for i := range policies {
		policy := policies[i]
		due, e := s.policyDue(ctx, &policy, time.Now().UTC())
		if e != nil {
			failures = append(failures, e)
			continue
		}
		if !due {
			continue
		}
		job, e := s.TriggerJob(schedulerCtx, TriggerJobRequest{PolicyID: policy.ID, Operator: "system.scheduler", TriggerType: modelops.BackupTriggerTypeScheduled})
		if errors.Is(e, ErrBackupJobAlreadyRunning) {
			continue
		}
		if e != nil {
			failures = append(failures, e)
		} else if job != nil && job.Status == modelops.BackupJobStatusFailed {
			failures = append(failures, fmt.Errorf("backup job %d failed: %s", job.ID, job.ErrorMessage))
		}
	}
	return errors.Join(failures...)
}
func (s *JobService) policyDue(ctx context.Context, policy *modelops.BackupPolicy, now time.Time) (bool, error) {
	latest, err := s.jobRepo.GetLatestByPolicy(ctx, policy.ID)
	if err != nil {
		return false, err
	}
	anchor := policy.CreatedAt
	if latest != nil {
		anchor = latest.CreatedAt
		if latest.StartedAt != nil {
			anchor = *latest.StartedAt
		}
	}
	return !now.Before(anchor.Add(parseScheduleDuration(policy.Schedule))), nil
}
func (s *JobService) scanAndTrigger(ctx context.Context) {
	if err := s.RunDue(ctx); err != nil {
		logOp(ctx, "error", "backup.scheduler.failed", zap.Error(err))
	}
}

func (s *JobService) ensureTenantContext(ctx context.Context) context.Context {
	if strings.TrimSpace(reqctx.GetTenantUUID(ctx)) != "" {
		return ctx
	}
	if fallback := strings.TrimSpace(os.Getenv("POWERX_GATEWAY_BOOTSTRAP_TENANT_UUID")); fallback != "" {
		return reqctx.WithTenantUUID(ctx, fallback)
	}
	if s.db == nil {
		return ctx
	}
	var row struct {
		UUID string `gorm:"column:uuid"`
	}
	err := s.db.WithContext(ctx).
		Table((&tenantmodel.Tenant{}).TableName()).
		Select("uuid").
		Where("status = ?", tenantmodel.TenantStatusActive).
		Order("CASE WHEN key = 'system' THEN 1 ELSE 0 END ASC, id ASC").
		Limit(1).
		Scan(&row).Error
	if err != nil || strings.TrimSpace(row.UUID) == "" {
		return ctx
	}
	return reqctx.WithTenantUUID(ctx, strings.TrimSpace(row.UUID))
}

func parseScheduleDuration(schedule string) time.Duration {
	d, _, err := parseScheduleDurationStrict(schedule)
	if err == nil && d > 0 {
		return d
	}
	return time.Duration(defaultIntervalHours) * time.Hour
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

func (s *JobService) audit(ctx context.Context, rec obsops.AuditRecord) {
	if s.auditor == nil {
		return
	}
	_ = s.auditor.Write(ctx, rec)
}

func resolveBackupScriptTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("POWERX_OPS_BACKUP_SCRIPT_TIMEOUT"))
	if raw == "" {
		return 30 * time.Minute
	}
	dur, err := time.ParseDuration(raw)
	if err != nil || dur <= 0 {
		return 30 * time.Minute
	}
	return dur
}

func sanitizePathSegment(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "._")
}

func backupScriptDir(required string) string {
	if dir := strings.TrimSpace(os.Getenv("POWERX_OPS_SCRIPT_DIR")); dir != "" {
		return opsscripts.ResolveDir(required)
	}
	if root := strings.TrimSpace(os.Getenv("POWERX_LINKS_ROOT")); filepath.IsAbs(root) {
		return filepath.Join(root, "backend", "scripts", "ops")
	}
	return opsscripts.ResolveDir(required)
}
