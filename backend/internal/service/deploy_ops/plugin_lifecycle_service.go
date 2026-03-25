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
	ErrInvalidPluginLifecycleRequest = errors.New("invalid plugin lifecycle request")
	ErrUnsupportedPluginAction       = errors.New("unsupported plugin lifecycle action")
)

type PluginLifecycleService struct {
	repo    *repoops.PluginLifecycleAuditRepository
	auditor obsops.AuditWriter
	metrics *inst.Recorder
}

type PluginLifecycleListOptions struct {
	PluginID string
	Page     int
	PageSize int
}

type PluginLifecycleActionRequest struct {
	PluginID    string
	FromVersion string
	ToVersion   string
	Action      string
	Reason      string
	Operator    string
	TraceID     string
}

func NewPluginLifecycleService(db *gorm.DB) *PluginLifecycleService {
	return &PluginLifecycleService{
		repo:    repoops.NewPluginLifecycleAuditRepository(db),
		auditor: obsops.NewUnifiedAuditWriter(db),
		metrics: inst.NewRecorder("powerx.service.plugin_lifecycle_ops"),
	}
}

func (s *PluginLifecycleService) ListAudits(ctx context.Context, opt PluginLifecycleListOptions) ([]modelops.PluginLifecycleAudit, int64, error) {
	pluginID := strings.TrimSpace(strings.ToLower(opt.PluginID))
	if pluginID == "" {
		return nil, 0, ErrInvalidPluginLifecycleRequest
	}

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
	return s.repo.ListByPluginID(ctx, pluginID, size, offset)
}

func (s *PluginLifecycleService) TriggerAction(ctx context.Context, req PluginLifecycleActionRequest) (*modelops.PluginLifecycleAudit, error) {
	startedAt := time.Now()
	var retErr error
	defer func() { s.metrics.Observe(ctx, "plugin_trigger_action", startedAt, retErr) }()

	pluginID := strings.TrimSpace(strings.ToLower(req.PluginID))
	action := strings.TrimSpace(strings.ToLower(req.Action))
	if pluginID == "" || action == "" {
		retErr = ErrInvalidPluginLifecycleRequest
		return nil, retErr
	}

	auditAction, err := normalizePluginAction(action)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	row := &modelops.PluginLifecycleAudit{
		PluginID:    pluginID,
		FromVersion: strings.TrimSpace(req.FromVersion),
		ToVersion:   strings.TrimSpace(req.ToVersion),
		Action:      auditAction,
		Result:      modelops.PluginLifecycleResultSuccess,
		GateResult:  "approved",
		GateReason:  strings.TrimSpace(req.Reason),
		Operator:    normalizeOperator(req.Operator),
		TraceID:     strings.TrimSpace(req.TraceID),
		Detail:      strings.TrimSpace(req.Reason),
	}

	if err := validateActionPayload(row); err != nil {
		retErr = err
		return nil, retErr
	}

	row.Normalize()
	saved, err := s.repo.Create(ctx, row)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	_ = s.audit(ctx, obsops.AuditRecord{
		ResourceType: "plugin",
		ResourceID:   fmt.Sprintf("%d", saved.ID),
		Operation:    string(saved.Action),
		Outcome:      string(saved.Result),
		Severity:     "info",
		Detail: map[string]any{
			"plugin_id":    saved.PluginID,
			"from_version": saved.FromVersion,
			"to_version":   saved.ToVersion,
			"action":       saved.Action,
			"result":       saved.Result,
			"gate_result":  saved.GateResult,
			"gate_reason":  saved.GateReason,
		},
	})
	return saved, nil
}

func (s *PluginLifecycleService) audit(ctx context.Context, rec obsops.AuditRecord) error {
	if s.auditor == nil {
		return nil
	}
	return s.auditor.Write(ctx, rec)
}

func normalizePluginAction(action string) (modelops.PluginLifecycleAction, error) {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case string(modelops.PluginLifecycleActionInstall):
		return modelops.PluginLifecycleActionInstall, nil
	case string(modelops.PluginLifecycleActionSwitch):
		return modelops.PluginLifecycleActionSwitch, nil
	case string(modelops.PluginLifecycleActionRollback):
		return modelops.PluginLifecycleActionRollback, nil
	case string(modelops.PluginLifecycleActionUninstall):
		return modelops.PluginLifecycleActionUninstall, nil
	default:
		return "", ErrUnsupportedPluginAction
	}
}

func validateActionPayload(row *modelops.PluginLifecycleAudit) error {
	if row == nil || row.PluginID == "" {
		return ErrInvalidPluginLifecycleRequest
	}

	switch row.Action {
	case modelops.PluginLifecycleActionSwitch:
		if row.ToVersion == "" {
			return ErrInvalidPluginLifecycleRequest
		}
	case modelops.PluginLifecycleActionRollback:
		if row.ToVersion == "" {
			return ErrInvalidPluginLifecycleRequest
		}
	}

	return nil
}
