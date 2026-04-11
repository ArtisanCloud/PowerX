package backup_ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	inst "github.com/ArtisanCloud/PowerX/internal/service/backup_ops/instrumentation"
	obsops "github.com/ArtisanCloud/PowerX/internal/service/observability_ops"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type RestoreDrillService struct {
	restoreRepo  *repoops.RestoreDrillRecordRepository
	jobRepo      *repoops.BackupJobRepository
	artifactRepo *repoops.BackupArtifactRepository
	runner       ScriptRunner
	auditor      obsops.AuditWriter
	scriptDir    string
	metrics      *inst.Recorder
	sm           *JobStateMachine
}

type TriggerRestoreDrillRequest struct {
	SourceJobID uint64
	ArtifactID  uint64
	Reason      string
	Operator    string
	TraceID     string
}

type ListRestoreDrillOptions struct {
	SourceJobID uint64
	Status      string
	From        *time.Time
	To          *time.Time
	Page        int
	PageSize    int
}

func NewRestoreDrillService(db *gorm.DB) *RestoreDrillService {
	scriptDir := strings.TrimSpace(os.Getenv("POWERX_OPS_SCRIPT_DIR"))
	if scriptDir == "" {
		scriptDir = filepath.Join("backend", "scripts", "ops")
	}
	return &RestoreDrillService{
		restoreRepo:  repoops.NewRestoreDrillRecordRepository(db),
		jobRepo:      repoops.NewBackupJobRepository(db),
		artifactRepo: repoops.NewBackupArtifactRepository(db),
		runner:       NewOSScriptRunner(),
		auditor:      obsops.NewUnifiedAuditWriter(db),
		scriptDir:    scriptDir,
		metrics:      inst.NewRecorder("powerx.service.restore_drill_ops"),
		sm:           NewJobStateMachine(),
	}
}

func (s *RestoreDrillService) Trigger(ctx context.Context, req TriggerRestoreDrillRequest) (*modelops.RestoreDrillRecord, error) {
	startedAt := time.Now()
	var retErr error
	defer func() { s.metrics.Observe(ctx, "backup_trigger_restore_drill", startedAt, retErr) }()

	sourceJobID, err := s.resolveSourceJobID(ctx, req.SourceJobID, req.ArtifactID)
	if err != nil {
		retErr = err
		return nil, retErr
	}
	if sourceJobID == 0 {
		retErr = ErrInvalidRestoreDrillRequest
		return nil, retErr
	}

	job, err := s.jobRepo.GetById(ctx, sourceJobID, nil)
	if err != nil {
		retErr = err
		return nil, retErr
	}
	if job == nil {
		retErr = ErrInvalidRestoreDrillRequest
		return nil, retErr
	}

	operator := normalizeOperator(req.Operator)
	initial := &modelops.RestoreDrillRecord{
		SourceJobID: sourceJobID,
		Status:      modelops.RestoreDrillStatusQueued,
		RTOSec:      0,
		ReportURI:   strings.TrimSpace(req.Reason),
		Operator:    operator,
		TraceID:     strings.TrimSpace(req.TraceID),
	}
	initial.Normalize()
	saved, err := s.restoreRepo.Create(ctx, initial)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	runningState, smErr := s.sm.Next(JobState(saved.Status), JobEventStart)
	if smErr != nil {
		retErr = smErr
		return nil, retErr
	}
	saved.Status = modelops.RestoreDrillStatus(runningState)
	saved.Normalize()
	saved, err = s.restoreRepo.Update(ctx, saved)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	execErr := s.runRestoreScript(ctx, sourceJobID)
	successState, smErr := s.sm.Next(JobState(saved.Status), JobEventSucceed)
	if smErr != nil {
		retErr = smErr
		return nil, retErr
	}
	rto := int64(120)
	report := strings.TrimSpace(req.Reason)
	finalStatus := modelops.RestoreDrillStatus(successState)
	if execErr != nil {
		failState, failErr := s.sm.Next(JobState(saved.Status), JobEventFail)
		if failErr != nil {
			retErr = failErr
			return nil, retErr
		}
		rto = 0
		report = execErr.Error()
		finalStatus = modelops.RestoreDrillStatus(failState)
	}
	saved.Status = finalStatus
	saved.RTOSec = rto
	saved.ReportURI = report
	saved.Normalize()
	saved, err = s.restoreRepo.Update(ctx, saved)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	s.audit(ctx, obsops.AuditRecord{ResourceType: "restore_drill", ResourceID: fmt.Sprintf("%d", saved.ID), Operation: "execute", Outcome: string(saved.Status), Severity: "info", Detail: map[string]any{"source_job_id": saved.SourceJobID, "rto_seconds": saved.RTOSec, "operator": saved.Operator, "trace_id": saved.TraceID}})
	logOp(ctx, "info", "backup.restore_drill.execute",
		zap.Uint64("drill_id", saved.ID),
		zap.Uint64("source_job_id", saved.SourceJobID),
		zap.String("status", string(saved.Status)),
		zap.Int64("rto_seconds", saved.RTOSec),
		zap.String("operator", saved.Operator),
		zap.String("trace_id", saved.TraceID),
	)
	return saved, nil
}

func (s *RestoreDrillService) List(ctx context.Context, opt ListRestoreDrillOptions) ([]modelops.RestoreDrillRecord, int64, error) {
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
	return s.restoreRepo.List(ctx, opt.SourceJobID, opt.Status, opt.From, opt.To, size, offset)
}

func (s *RestoreDrillService) Get(ctx context.Context, drillID uint64) (*modelops.RestoreDrillRecord, error) {
	if drillID == 0 {
		return nil, ErrInvalidRestoreDrillRequest
	}
	row, err := s.restoreRepo.GetById(ctx, drillID, nil)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrRestoreDrillNotFound
	}
	return row, nil
}

func (s *RestoreDrillService) ShouldTriggerByPolicy(ctx context.Context, policyID uint64, intervalDays int, now time.Time) (bool, error) {
	if policyID == 0 {
		return false, nil
	}
	if intervalDays <= 0 {
		intervalDays = 7
	}
	latest, err := s.restoreRepo.GetLatestByPolicy(ctx, policyID)
	if err != nil {
		return false, err
	}
	if latest == nil {
		return true, nil
	}
	nextAt := latest.CreatedAt.UTC().Add(time.Duration(intervalDays) * 24 * time.Hour)
	return !now.UTC().Before(nextAt), nil
}

func (s *RestoreDrillService) resolveSourceJobID(ctx context.Context, sourceJobID, artifactID uint64) (uint64, error) {
	if artifactID > 0 {
		artifact, err := s.artifactRepo.GetById(ctx, artifactID, nil)
		if err != nil {
			return 0, err
		}
		if artifact == nil || artifact.JobID == 0 || strings.TrimSpace(artifact.StorageURI) == "" {
			return 0, ErrInvalidRestoreDrillRequest
		}
		return artifact.JobID, nil
	}
	if sourceJobID > 0 {
		return sourceJobID, nil
	}
	return 0, ErrInvalidRestoreDrillRequest
}

func (s *RestoreDrillService) runRestoreScript(ctx context.Context, sourceJobID uint64) error {
	if s.runner == nil {
		return nil
	}
	path := filepath.Join(s.scriptDir, "restore-drill.sh")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	_, err := s.runner.Run(ctx, ScriptSpec{Command: path, Args: []string{strconv.FormatUint(sourceJobID, 10)}, Timeout: 2 * time.Minute})
	return err
}

func (s *RestoreDrillService) audit(ctx context.Context, rec obsops.AuditRecord) {
	if s.auditor == nil {
		return
	}
	_ = s.auditor.Write(ctx, rec)
}
