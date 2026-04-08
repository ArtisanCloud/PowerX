package backup_ops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	inst "github.com/ArtisanCloud/PowerX/internal/service/backup_ops/instrumentation"
	obsops "github.com/ArtisanCloud/PowerX/internal/service/observability_ops"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"gorm.io/gorm"
)

var (
	ErrInvalidBackupPolicy = errors.New("invalid backup policy")
)

type PolicyService struct {
	repo    *repoops.BackupPolicyRepository
	auditor obsops.AuditWriter
	metrics *inst.Recorder
}

type ListPolicyOptions struct {
	EnabledOnly bool
	Page        int
	PageSize    int
}

type UpsertPolicyRequest struct {
	Name          string
	BackupType    string
	Schedule      string
	RetentionDays int
	Enabled       bool
	StorageTarget string
	Operator      string
	TraceID       string
}

func NewPolicyService(db *gorm.DB) *PolicyService {
	return &PolicyService{
		repo:    repoops.NewBackupPolicyRepository(db),
		auditor: obsops.NewUnifiedAuditWriter(db),
		metrics: inst.NewRecorder("powerx.service.backup_policy_ops"),
	}
}

func (s *PolicyService) ListPolicies(ctx context.Context, opt ListPolicyOptions) ([]modelops.BackupPolicy, int64, error) {
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

	var enabled *bool
	if opt.EnabledOnly {
		v := true
		enabled = &v
	}
	return s.repo.List(ctx, enabled, size, offset)
}

func (s *PolicyService) UpsertPolicy(ctx context.Context, req UpsertPolicyRequest) (*modelops.BackupPolicy, error) {
	startedAt := time.Now()
	var retErr error
	defer func() { s.metrics.Observe(ctx, "backup_upsert_policy", startedAt, retErr) }()

	name := strings.TrimSpace(req.Name)
	schedule := strings.TrimSpace(req.Schedule)
	storage := strings.TrimSpace(req.StorageTarget)
	backupType := strings.TrimSpace(strings.ToLower(req.BackupType))
	if name == "" || schedule == "" || storage == "" || backupType == "" || req.RetentionDays <= 0 {
		retErr = ErrInvalidBackupPolicy
		return nil, retErr
	}

	existing, err := s.repo.GetFirst(ctx, map[string]interface{}{"name": name})
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		retErr = err
		return nil, retErr
	}

	operator := normalizeOperator(req.Operator)
	if existing != nil {
		existing.BackupType = modelops.BackupType(backupType)
		existing.Schedule = schedule
		existing.RetentionDays = int32(req.RetentionDays)
		existing.Enabled = req.Enabled
		existing.StorageTarget = storage
		existing.UpdatedBy = operator
		existing.Normalize()
		updated, err := s.repo.Update(ctx, existing)
		if err != nil {
			retErr = err
			return nil, retErr
		}
		s.audit(ctx, obsops.AuditRecord{ResourceType: "backup_policy", ResourceID: fmt.Sprintf("%d", updated.ID), Operation: "update", Outcome: "success", Severity: "info", Detail: map[string]any{"name": updated.Name, "backup_type": updated.BackupType}})
		return updated, nil
	}

	row := &modelops.BackupPolicy{
		Name:          name,
		BackupType:    modelops.BackupType(backupType),
		Schedule:      schedule,
		RetentionDays: int32(req.RetentionDays),
		Enabled:       req.Enabled,
		StorageTarget: storage,
		CreatedBy:     operator,
		UpdatedBy:     operator,
	}
	row.Normalize()
	saved, err := s.repo.Create(ctx, row)
	if err != nil {
		retErr = err
		return nil, retErr
	}
	s.audit(ctx, obsops.AuditRecord{ResourceType: "backup_policy", ResourceID: fmt.Sprintf("%d", saved.ID), Operation: "create", Outcome: "success", Severity: "info", Detail: map[string]any{"name": saved.Name, "backup_type": saved.BackupType}})
	return saved, nil
}

func (s *PolicyService) audit(ctx context.Context, rec obsops.AuditRecord) {
	if s.auditor == nil {
		return
	}
	_ = s.auditor.Write(ctx, rec)
}
