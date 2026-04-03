package deploy_ops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	inst "github.com/ArtisanCloud/PowerX/internal/service/deploy_ops/instrumentation"
	obsops "github.com/ArtisanCloud/PowerX/internal/service/observability_ops"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"gorm.io/gorm"
)

var (
	ErrInvalidDeployRequest = errors.New("invalid deploy request")
	ErrReleaseInProgress    = errors.New("a running release already exists in this environment")
	ErrInvalidDeployMode    = errors.New("invalid deploy mode")
)

const (
	DeployModeDocker  = "docker"
	DeployModeSystemd = "systemd"
)

type ReleaseRequest struct {
	Environment     string
	BackendVersion  string
	WebAdminVersion string
	Mode            string
	Operator        string
	TraceID         string
	ApprovalTickets int
}

type RollbackRequest struct {
	Environment     string
	TargetVersion   string
	Mode            string
	Operator        string
	TraceID         string
	ApprovalTickets int
}

type ListReleaseOptions struct {
	Environment string
	Page        int
	PageSize    int
}

type HealthSummary struct {
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

type Service struct {
	releases *repoops.DeployReleaseRecordRepository
	approval *ApprovalPolicyService
	auditor  obsops.AuditWriter
	metrics  *inst.Recorder
}

func NewService(db *gorm.DB) *Service {
	return &Service{
		releases: repoops.NewDeployReleaseRecordRepository(db),
		approval: NewApprovalPolicyService(db),
		auditor:  obsops.NewUnifiedAuditWriter(db),
		metrics:  inst.NewRecorder("powerx.service.deploy_ops"),
	}
}

func (s *Service) ListReleases(ctx context.Context, opt ListReleaseOptions) ([]modelops.DeployReleaseRecord, int64, error) {
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
	return s.releases.ListByEnvironment(ctx, normalizeEnv(opt.Environment), size, offset)
}

func (s *Service) TriggerRelease(ctx context.Context, req ReleaseRequest) (*modelops.DeployReleaseRecord, error) {
	startedAt := time.Now()
	var retErr error
	defer func() { s.metrics.Observe(ctx, "trigger_release", startedAt, retErr) }()

	env := normalizeEnv(req.Environment)
	mode := normalizeMode(req.Mode)
	if env == "" || strings.TrimSpace(req.BackendVersion) == "" || strings.TrimSpace(req.WebAdminVersion) == "" {
		retErr = ErrInvalidDeployRequest
		return nil, retErr
	}
	if mode == "" {
		retErr = ErrInvalidDeployMode
		return nil, retErr
	}

	if err := s.approval.EnsureAllowed(ctx, env, req.ApprovalTickets); err != nil {
		retErr = err
		return nil, retErr
	}

	running, err := s.releases.FindRunningByEnvironment(ctx, env)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		retErr = err
		return nil, retErr
	}
	if running != nil {
		retErr = ErrReleaseInProgress
		return nil, retErr
	}

	now := time.Now().UTC()
	row := &modelops.DeployReleaseRecord{
		Environment:     env,
		BackendVersion:  strings.TrimSpace(req.BackendVersion),
		WebAdminVersion: strings.TrimSpace(req.WebAdminVersion),
		Action:          modelops.DeployActionRelease,
		Status:          modelops.DeployStatusRunning,
		Operator:        normalizeOperator(req.Operator),
		TraceID:         strings.TrimSpace(req.TraceID),
		StartedAt:       &now,
	}
	row.Normalize()
	saved, err := s.releases.Create(ctx, row)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	ended := time.Now().UTC()
	saved.Status = modelops.DeployStatusSuccess
	saved.EndedAt = &ended
	if _, err = s.releases.Update(ctx, saved); err != nil {
		retErr = err
		return nil, retErr
	}

	_ = s.audit(ctx, obsops.AuditRecord{
		ResourceType: "deploy",
		ResourceID:   fmt.Sprintf("%d", saved.ID),
		Operation:    "release",
		Outcome:      "success",
		Severity:     "info",
		Detail: map[string]any{
			"environment":       saved.Environment,
			"backend_version":   saved.BackendVersion,
			"web_admin_version": saved.WebAdminVersion,
			"mode":              mode,
		},
	})
	return saved, nil
}

func (s *Service) TriggerRollback(ctx context.Context, req RollbackRequest) (*modelops.DeployReleaseRecord, error) {
	startedAt := time.Now()
	var retErr error
	defer func() { s.metrics.Observe(ctx, "trigger_rollback", startedAt, retErr) }()

	env := normalizeEnv(req.Environment)
	target := strings.TrimSpace(req.TargetVersion)
	mode := normalizeMode(req.Mode)
	if env == "" || target == "" {
		retErr = ErrInvalidDeployRequest
		return nil, retErr
	}
	if mode == "" {
		retErr = ErrInvalidDeployMode
		return nil, retErr
	}

	if err := s.approval.EnsureAllowed(ctx, env, req.ApprovalTickets); err != nil {
		retErr = err
		return nil, retErr
	}

	now := time.Now().UTC()
	row := &modelops.DeployReleaseRecord{
		Environment:     env,
		BackendVersion:  target,
		WebAdminVersion: target,
		Action:          modelops.DeployActionRollback,
		Status:          modelops.DeployStatusRunning,
		Operator:        normalizeOperator(req.Operator),
		TraceID:         strings.TrimSpace(req.TraceID),
		StartedAt:       &now,
	}
	row.Normalize()
	saved, err := s.releases.Create(ctx, row)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	ended := time.Now().UTC()
	saved.Status = modelops.DeployStatusSuccess
	saved.EndedAt = &ended
	if _, err = s.releases.Update(ctx, saved); err != nil {
		retErr = err
		return nil, retErr
	}

	_ = s.audit(ctx, obsops.AuditRecord{
		ResourceType: "deploy",
		ResourceID:   fmt.Sprintf("%d", saved.ID),
		Operation:    "rollback",
		Outcome:      "success",
		Severity:     "info",
		Detail: map[string]any{
			"environment":    saved.Environment,
			"target_version": target,
			"mode":           mode,
		},
	})
	return saved, nil
}

func (s *Service) GetHealth(ctx context.Context) (*HealthSummary, error) {
	rows, _, err := s.releases.ListByEnvironment(ctx, "", 100, 0)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &HealthSummary{Status: "healthy", Summary: "no deploy jobs yet"}, nil
	}

	running := 0
	failed := 0
	for _, row := range rows {
		switch row.Status {
		case modelops.DeployStatusRunning:
			running++
		case modelops.DeployStatusFailed:
			failed++
		}
	}

	switch {
	case running > 0:
		return &HealthSummary{Status: "degraded", Summary: "deploy job running"}, nil
	case failed > 0:
		return &HealthSummary{Status: "warning", Summary: "recent failures detected"}, nil
	default:
		return &HealthSummary{Status: "healthy", Summary: "deploy pipeline healthy"}, nil
	}
}

func (s *Service) audit(ctx context.Context, rec obsops.AuditRecord) error {
	if s.auditor == nil {
		return nil
	}
	return s.auditor.Write(ctx, rec)
}

func normalizeEnv(v string) string {
	return strings.TrimSpace(strings.ToLower(v))
}

func normalizeOperator(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "system"
	}
	return v
}

func normalizeMode(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return DeployModeDocker
	}
	if v != DeployModeDocker && v != DeployModeSystemd {
		return ""
	}
	return v
}
