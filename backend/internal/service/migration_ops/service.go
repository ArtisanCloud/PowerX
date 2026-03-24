package migration_ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	obsops "github.com/ArtisanCloud/PowerX/internal/service/observability_ops"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"gorm.io/gorm"
)

var (
	ErrInvalidMigrationRequest = errors.New("invalid migration request")
	ErrMigrationNotFound       = errors.New("migration record not found")
	ErrMigrationNotReady       = errors.New("migration acceptance not ready")
)

type Service struct {
	repo      *repoops.MigrationRunbookRecordRepository
	auditor   obsops.AuditWriter
	scriptDir string
}

type TriggerRequest struct {
	SourceEnv string
	TargetEnv string
	DryRun    bool
	Operator  string
	TraceID   string
}

type AcceptanceRequest struct {
	MigrationID              uint64
	DBMigrationCompleted     bool
	InstanceMigrationPassed  bool
	AcceptanceConclusionNote string
}

type SwitchRequest struct {
	MigrationID uint64
	Rollback    bool
	Operator    string
	TraceID     string
}

func NewService(db *gorm.DB) *Service {
	scriptDir := strings.TrimSpace(os.Getenv("POWERX_OPS_SCRIPT_DIR"))
	if scriptDir == "" {
		scriptDir = filepath.Join("backend", "scripts", "ops")
	}
	return &Service{
		repo:      repoops.NewMigrationRunbookRecordRepository(db),
		auditor:   obsops.NewUnifiedAuditWriter(db),
		scriptDir: scriptDir,
	}
}

func (s *Service) TriggerMigration(ctx context.Context, req TriggerRequest) (*modelops.MigrationRunbookRecord, error) {
	source := strings.TrimSpace(req.SourceEnv)
	target := strings.TrimSpace(req.TargetEnv)
	if source == "" || target == "" || strings.EqualFold(source, target) {
		return nil, ErrInvalidMigrationRequest
	}

	now := time.Now().UTC()
	row := &modelops.MigrationRunbookRecord{
		SourceEnv:                source,
		TargetEnv:                target,
		Status:                   modelops.MigrationStatusRunning,
		DBMigrationStatus:        modelops.MigrationStepPending,
		InstanceAcceptanceStatus: modelops.MigrationStepPending,
		TrafficSwitchStatus:      modelops.MigrationStepPending,
		TrafficRollbackStatus:    modelops.MigrationStepPending,
		DryRun:                   req.DryRun,
		Summary:                  "migration started",
		Operator:                 normalizeOperator(req.Operator),
		TraceID:                  strings.TrimSpace(req.TraceID),
		StartedAt:                &now,
	}
	row.Normalize()

	saved, err := s.repo.Create(ctx, row)
	if err != nil {
		return nil, err
	}

	if err := s.run(ctx, "export-instance.sh", strconv.FormatUint(saved.ID, 10), source, target); err != nil {
		return s.failMigration(ctx, saved, fmt.Errorf("export failed: %w", err))
	}
	if err := s.run(ctx, "import-instance.sh", strconv.FormatUint(saved.ID, 10), source, target); err != nil {
		return s.failMigration(ctx, saved, fmt.Errorf("import failed: %w", err))
	}
	if err := s.run(ctx, "verify-migration.sh", strconv.FormatUint(saved.ID, 10), source, target); err != nil {
		return s.failMigration(ctx, saved, fmt.Errorf("verify failed: %w", err))
	}

	saved.DBMigrationStatus = modelops.MigrationStepSuccess
	if req.DryRun {
		ended := time.Now().UTC()
		saved.InstanceAcceptanceStatus = modelops.MigrationStepSuccess
		saved.Status = modelops.MigrationStatusSuccess
		saved.Summary = "dry-run migration verified"
		saved.EndedAt = &ended
	} else {
		saved.Status = modelops.MigrationStatusRunning
		saved.Summary = "database migration completed; waiting instance acceptance"
	}
	saved.Normalize()
	updated, err := s.repo.Update(ctx, saved)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "migration_runbook", updated.ID, "trigger", string(updated.Status), map[string]any{
		"source_env": source,
		"target_env": target,
		"dry_run":    req.DryRun,
	})
	return updated, nil
}

func (s *Service) GetMigration(ctx context.Context, migrationID uint64) (*modelops.MigrationRunbookRecord, error) {
	if migrationID == 0 {
		return nil, ErrInvalidMigrationRequest
	}
	row, err := s.repo.GetById(ctx, migrationID, nil)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrMigrationNotFound
	}
	return row, nil
}

func (s *Service) AcceptMigration(ctx context.Context, req AcceptanceRequest) (*modelops.MigrationRunbookRecord, error) {
	if req.MigrationID == 0 {
		return nil, ErrInvalidMigrationRequest
	}
	row, err := s.repo.GetById(ctx, req.MigrationID, nil)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrMigrationNotFound
	}

	if !req.DBMigrationCompleted {
		row.DBMigrationStatus = modelops.MigrationStepFailed
		row.Status = modelops.MigrationStatusFailed
		row.Summary = "database migration acceptance failed"
		row.ErrorMessage = req.AcceptanceConclusionNote
	} else {
		row.DBMigrationStatus = modelops.MigrationStepSuccess
	}

	if req.InstanceMigrationPassed {
		row.InstanceAcceptanceStatus = modelops.MigrationStepSuccess
		if row.Status != modelops.MigrationStatusFailed {
			row.Status = modelops.MigrationStatusSuccess
			row.Summary = firstNonEmpty(req.AcceptanceConclusionNote, "instance migration acceptance passed")
		}
	} else {
		row.InstanceAcceptanceStatus = modelops.MigrationStepFailed
		row.Status = modelops.MigrationStatusFailed
		row.Summary = firstNonEmpty(req.AcceptanceConclusionNote, "instance migration acceptance failed")
	}

	ended := time.Now().UTC()
	row.EndedAt = &ended
	row.Normalize()
	updated, err := s.repo.Update(ctx, row)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "migration_runbook", updated.ID, "accept", string(updated.Status), map[string]any{
		"db_migration_completed":    req.DBMigrationCompleted,
		"instance_migration_passed": req.InstanceMigrationPassed,
	})
	return updated, nil
}

func (s *Service) TriggerTrafficSwitch(ctx context.Context, req SwitchRequest) (string, *modelops.MigrationRunbookRecord, error) {
	if req.MigrationID == 0 {
		return "", nil, ErrInvalidMigrationRequest
	}
	row, err := s.repo.GetById(ctx, req.MigrationID, nil)
	if err != nil {
		return "", nil, err
	}
	if row == nil {
		return "", nil, ErrMigrationNotFound
	}

	if req.Rollback {
		if err := s.run(ctx, "rollback-traffic.sh", strconv.FormatUint(req.MigrationID, 10), row.SourceEnv, row.TargetEnv); err != nil {
			return "", nil, err
		}
		row.TrafficRollbackStatus = modelops.MigrationStepSuccess
		row.Summary = "traffic rollback completed"
	} else {
		if row.DBMigrationStatus != modelops.MigrationStepSuccess || row.InstanceAcceptanceStatus != modelops.MigrationStepSuccess {
			return "", nil, ErrMigrationNotReady
		}
		if err := s.run(ctx, "switch-traffic.sh", strconv.FormatUint(req.MigrationID, 10), row.SourceEnv, row.TargetEnv); err != nil {
			return "", nil, err
		}
		row.TrafficSwitchStatus = modelops.MigrationStepSuccess
		row.Summary = "traffic switch completed"
	}

	row.Operator = normalizeOperator(req.Operator)
	row.TraceID = strings.TrimSpace(req.TraceID)
	row.Normalize()
	updated, err := s.repo.Update(ctx, row)
	if err != nil {
		return "", nil, err
	}

	operationID := fmt.Sprintf("migration-%d-%d", req.MigrationID, time.Now().UTC().Unix())
	operation := "switch"
	if req.Rollback {
		operation = "rollback"
	}
	s.audit(ctx, "migration_runbook", updated.ID, operation, "success", map[string]any{
		"operation_id": operationID,
		"rollback":     req.Rollback,
	})
	return operationID, updated, nil
}

func (s *Service) run(ctx context.Context, scriptName string, args ...string) error {
	path := filepath.Join(s.scriptDir, scriptName)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(runCtx, path, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Service) failMigration(ctx context.Context, row *modelops.MigrationRunbookRecord, cause error) (*modelops.MigrationRunbookRecord, error) {
	row.Status = modelops.MigrationStatusFailed
	row.DBMigrationStatus = modelops.MigrationStepFailed
	row.ErrorMessage = cause.Error()
	row.Summary = "migration failed before acceptance"
	ended := time.Now().UTC()
	row.EndedAt = &ended
	row.Normalize()
	updated, err := s.repo.Update(ctx, row)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "migration_runbook", updated.ID, "trigger", "failed", map[string]any{
		"error": cause.Error(),
	})
	return updated, nil
}

func (s *Service) audit(ctx context.Context, resourceType string, resourceID uint64, operation, outcome string, detail map[string]any) {
	if s.auditor == nil {
		return
	}
	_ = s.auditor.Write(ctx, obsops.AuditRecord{
		ResourceType: resourceType,
		ResourceID:   strconv.FormatUint(resourceID, 10),
		Operation:    operation,
		Outcome:      outcome,
		Severity:     "info",
		Detail:       detail,
	})
}

func normalizeOperator(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "system"
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
