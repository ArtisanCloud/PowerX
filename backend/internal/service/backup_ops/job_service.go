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
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
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
	backupScriptTimeout time.Duration
	metrics             *inst.Recorder
	lockMu              sync.Mutex
	policyLock          map[uint64]struct{}
	nextRuns            map[uint64]nextRunCache
}

type nextRunCache struct {
	At       time.Time
	Schedule string
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
		scriptDir = resolveOpsScriptDir()
	}
	artifactBaseDir := strings.TrimSpace(os.Getenv("POWERX_OPS_BACKUP_ARTIFACT_DIR"))
	if artifactBaseDir == "" {
		appEnv := strings.TrimSpace(strings.ToLower(os.Getenv("APP_ENV")))
		if appEnv == "" {
			appEnv = strings.TrimSpace(strings.ToLower(os.Getenv("POWERX_ENV")))
		}
		if appEnv == "prod" || appEnv == "production" {
			artifactBaseDir = "/var/lib/powerx/ops-backup/artifacts"
		} else {
			artifactBaseDir = resolveDevArtifactBaseDir()
		}
	}
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
		backupScriptTimeout: resolveBackupScriptTimeout(),
		metrics:             inst.NewRecorder("powerx.service.backup_job_ops"),
		policyLock:          make(map[uint64]struct{}),
		nextRuns:            make(map[uint64]nextRunCache),
	}
}

func resolveOpsScriptDir() string {
	candidates := []string{
		filepath.Join("scripts", "ops"),
		filepath.Join("backend", "scripts", "ops"),
	}
	for i := range candidates {
		if pathExists(filepath.Join(candidates[i], "backup-db.sh")) {
			return candidates[i]
		}
	}
	if root := detectProjectRoot(); root != "" {
		return filepath.Join(root, "backend", "scripts", "ops")
	}
	return filepath.Join("backend", "scripts", "ops")
}

func resolveDevArtifactBaseDir() string {
	if root := detectProjectRoot(); root != "" {
		return filepath.Join(root, "backend", "tmp", "ops-backup", "artifacts")
	}
	if pathExists("tmp") {
		return filepath.Join("tmp", "ops-backup", "artifacts")
	}
	return filepath.Join("backend", "tmp", "ops-backup", "artifacts")
}

func detectProjectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := filepath.Clean(wd)
	for {
		if pathExists(filepath.Join(dir, "backend", "etc", "config.yaml")) {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir || next == "." || next == string(filepath.Separator) {
			break
		}
		dir = next
	}
	return ""
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
	if updated.Status == modelops.BackupJobStatusSuccess && policy.DrillEnabled && s.restoreSvc != nil {
		interval := int(policy.DrillIntervalDays)
		if interval <= 0 {
			interval = 7
		}
		shouldTrigger, shouldErr := s.restoreSvc.ShouldTriggerByPolicy(ctx, policy.ID, interval, time.Now().UTC())
		if shouldErr == nil && shouldTrigger {
			_, _ = s.restoreSvc.Trigger(ctx, TriggerRestoreDrillRequest{
				SourceJobID: updated.ID,
				Reason:      "scheduled_by_policy",
				Operator:    "system.scheduler",
				TraceID:     updated.TraceID,
			})
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
		return nil
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
	if dsn := strings.TrimSpace(resolveBackupSourceDSN()); dsn != "" {
		spec.Env = append(spec.Env, "POWERX_OPS_BACKUP_SOURCE_DSN="+dsn)
	}
	_, err := s.runner.Run(ctx, spec)
	return err
}

func (s *JobService) prepareArtifactPath(ctx context.Context, job *modelops.BackupJob, policy *modelops.BackupPolicy) (string, error) {
	if job == nil || policy == nil || job.ID == 0 || policy.ID == 0 {
		return "", ErrInvalidBackupRequest
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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
	uri := "file://" + filepath.ToSlash(absPath)
	row := &modelops.BackupArtifact{
		JobID:       job.ID,
		StorageURI:  uri,
		SizeBytes:   stat.Size(),
		Checksum:    checksum,
		ContentType: "application/postgresql-custom",
	}
	row.Normalize()
	_, err = s.artifactRepo.Create(ctx, row)
	return err
}

func (s *JobService) scanAndTrigger(ctx context.Context) {
	enabled := true
	policies, _, err := s.policyRepo.List(ctx, &enabled, 500, 0)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	schedulerCtx := s.ensureTenantContext(ctx)
	for i := range policies {
		p := policies[i]
		nextAt := s.nextRunAt(p.ID, now, p.Schedule)
		if now.Before(nextAt) {
			continue
		}
		_, _ = s.TriggerJob(schedulerCtx, TriggerJobRequest{
			PolicyID:    p.ID,
			Operator:    "system.scheduler",
			TriggerType: modelops.BackupTriggerTypeScheduled,
		})
		s.setNextRunAt(p.ID, now.Add(parseScheduleDuration(p.Schedule)), p.Schedule)
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

func (s *JobService) nextRunAt(policyID uint64, now time.Time, schedule string) time.Time {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	normalizedSchedule := normalizeScheduleKey(schedule)
	if v, ok := s.nextRuns[policyID]; ok && !v.At.IsZero() && v.Schedule == normalizedSchedule {
		return v.At
	}
	nextAt := now.Add(parseScheduleDuration(schedule))
	s.nextRuns[policyID] = nextRunCache{
		At:       nextAt,
		Schedule: normalizedSchedule,
	}
	return nextAt
}

func (s *JobService) setNextRunAt(policyID uint64, next time.Time, schedule string) {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	s.nextRuns[policyID] = nextRunCache{
		At:       next,
		Schedule: normalizeScheduleKey(schedule),
	}
}

func parseScheduleDuration(schedule string) time.Duration {
	d, _, err := parseScheduleDurationStrict(schedule)
	if err == nil && d > 0 {
		return d
	}
	return 6 * time.Hour
}

func normalizeScheduleKey(schedule string) string {
	return strings.TrimSpace(strings.ToLower(schedule))
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

func resolveBackupSourceDSN() string {
	if dsn := strings.TrimSpace(os.Getenv("POWERX_OPS_BACKUP_SOURCE_DSN")); dsn != "" {
		return dsn
	}
	if dsn := strings.TrimSpace(os.Getenv("POWERX_DB_DSN")); dsn != "" {
		return dsn
	}
	cfg, ok := loadBackupDBConfig()
	if !ok {
		return ""
	}
	if dsn := strings.TrimSpace(cfg.DSN); dsn != "" {
		return dsn
	}
	driver := strings.TrimSpace(strings.ToLower(cfg.Driver))
	if driver != "" && driver != "postgres" && driver != "postgresql" {
		return ""
	}
	host := strings.TrimSpace(cfg.Host)
	dbName := strings.TrimSpace(cfg.Database)
	user := strings.TrimSpace(cfg.UserName)
	if host == "" || dbName == "" || user == "" {
		return ""
	}
	port := cfg.Port
	if port <= 0 {
		port = 5432
	}
	q := url.Values{}
	sslMode := strings.TrimSpace(cfg.SSLMode)
	if sslMode == "" {
		sslMode = "disable"
	}
	q.Set("sslmode", sslMode)
	if tz := strings.TrimSpace(cfg.Timezone); tz != "" {
		q.Set("timezone", tz)
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?%s",
		url.PathEscape(user),
		url.PathEscape(cfg.Password),
		host,
		port,
		url.PathEscape(dbName),
		q.Encode(),
	)
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

type backupDBConfig struct {
	Driver   string `yaml:"driver"`
	DSN      string `yaml:"dsn"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	UserName string `yaml:"username"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	SSLMode  string `yaml:"ssl_mode"`
	Timezone string `yaml:"timezone"`
}

type backupConfigFile struct {
	Database backupDBConfig `yaml:"database"`
}

func loadBackupDBConfig() (backupDBConfig, bool) {
	candidates := make([]string, 0, 4)
	if p := strings.TrimSpace(os.Getenv("POWERX_CONFIG")); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, filepath.Join("backend", "etc", "config.yaml"))
	candidates = append(candidates, filepath.Join("etc", "config.yaml"))
	for _, path := range candidates {
		cfg, ok := loadBackupDBConfigFrom(path)
		if ok {
			return cfg, true
		}
	}
	return backupDBConfig{}, false
}

func loadBackupDBConfigFrom(path string) (backupDBConfig, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return backupDBConfig{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return backupDBConfig{}, false
	}
	var parsed backupConfigFile
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return backupDBConfig{}, false
	}
	return parsed.Database, true
}
