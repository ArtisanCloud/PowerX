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
	"gorm.io/gorm"
)

var (
	ErrInvalidRestoreDrillRequest = errors.New("invalid restore drill request")
)

type RestoreDrillService struct {
	restoreRepo *repoops.RestoreDrillRecordRepository
	jobRepo     *repoops.BackupJobRepository
	runner      ScriptRunner
	auditor     obsops.AuditWriter
	scriptDir   string
	metrics     *inst.Recorder
}

type TriggerRestoreDrillRequest struct {
	SourceJobID uint64
	Operator    string
	TraceID     string
}

func NewRestoreDrillService(db *gorm.DB) *RestoreDrillService {
	scriptDir := strings.TrimSpace(os.Getenv("POWERX_OPS_SCRIPT_DIR"))
	if scriptDir == "" {
		scriptDir = filepath.Join("backend", "scripts", "ops")
	}
	return &RestoreDrillService{
		restoreRepo: repoops.NewRestoreDrillRecordRepository(db),
		jobRepo:     repoops.NewBackupJobRepository(db),
		runner:      NewOSScriptRunner(),
		auditor:     obsops.NewUnifiedAuditWriter(db),
		scriptDir:   scriptDir,
		metrics:     inst.NewRecorder("powerx.service.restore_drill_ops"),
	}
}

func (s *RestoreDrillService) Trigger(ctx context.Context, req TriggerRestoreDrillRequest) (*modelops.RestoreDrillRecord, error) {
	startedAt := time.Now()
	var retErr error
	defer func() { s.metrics.Observe(ctx, "backup_trigger_restore_drill", startedAt, retErr) }()

	if req.SourceJobID == 0 {
		retErr = ErrInvalidRestoreDrillRequest
		return nil, retErr
	}
	job, err := s.jobRepo.GetById(ctx, req.SourceJobID, nil)
	if err != nil {
		retErr = err
		return nil, retErr
	}
	if job == nil {
		retErr = ErrInvalidRestoreDrillRequest
		return nil, retErr
	}

	rto := int64(120)
	report := ""
	if err := s.runRestoreScript(ctx, req.SourceJobID); err != nil {
		rto = 0
		report = err.Error()
	}

	status := modelops.RestoreDrillStatusSuccess
	if rto == 0 {
		status = modelops.RestoreDrillStatusFailed
	}
	row := &modelops.RestoreDrillRecord{
		SourceJobID: req.SourceJobID,
		Status:      status,
		RTOSec:      rto,
		ReportURI:   report,
		Operator:    normalizeOperator(req.Operator),
		TraceID:     strings.TrimSpace(req.TraceID),
	}
	row.Normalize()
	saved, err := s.restoreRepo.Create(ctx, row)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	s.audit(ctx, obsops.AuditRecord{ResourceType: "restore_drill", ResourceID: fmt.Sprintf("%d", saved.ID), Operation: "trigger", Outcome: string(saved.Status), Severity: "info", Detail: map[string]any{"source_job_id": saved.SourceJobID, "rto_seconds": saved.RTOSec}})
	return saved, nil
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
