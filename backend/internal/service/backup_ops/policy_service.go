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

const (
	defaultIntervalHours    = 6
	defaultRetentionCount   = 14
	defaultTimezone         = "Asia/Shanghai"
	defaultDrillIntervalDay = 7
)

type PolicyService struct {
	repo    *repoops.BackupPolicyRepository
	auditor obsops.AuditWriter
	metrics *inst.Recorder
}

type ListPolicyOptions struct {
	EnabledOnly bool
	Status      string
	Keyword     string
	Timezone    string
	Page        int
	PageSize    int
}

type CreatePolicyRequest struct {
	Name             string
	IntervalHours    int
	RetentionCount   int
	Timezone         string
	DrillEnabled     *bool
	DrillIntervalDay int
	TargetRef        string
	Operator         string
	TraceID          string
}

type UpdatePolicyRequest struct {
	PolicyID         uint64
	Name             *string
	IntervalHours    *int
	RetentionCount   *int
	Timezone         *string
	DrillEnabled     *bool
	DrillIntervalDay *int
	TargetRef        *string
	Operator         string
	TraceID          string
}

type SetPolicyEnabledRequest struct {
	PolicyID uint64
	Enabled  bool
	Operator string
	TraceID  string
}

// UpsertPolicyRequest 保留给现有 gRPC 兼容路径。
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
	return s.repo.ListWithFilters(ctx, opt.Status, opt.Keyword, opt.Timezone, enabled, size, offset)
}

func (s *PolicyService) CreatePolicy(ctx context.Context, req CreatePolicyRequest) (*modelops.BackupPolicy, error) {
	startedAt := time.Now()
	var retErr error
	defer func() { s.metrics.Observe(ctx, "backup_create_policy", startedAt, retErr) }()

	name := strings.TrimSpace(req.Name)
	if name == "" {
		retErr = ErrInvalidBackupPolicy
		return nil, retErr
	}

	interval, retention, timezone, drillEnabled, drillInterval, targetRef, err := normalizePolicyValues(
		req.IntervalHours,
		req.RetentionCount,
		req.Timezone,
		req.DrillEnabled,
		req.DrillIntervalDay,
		req.TargetRef,
	)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	operator := normalizeOperator(req.Operator)
	row := &modelops.BackupPolicy{
		Name:              name,
		BackupType:        modelops.BackupTypeLogical,
		Schedule:          fmt.Sprintf("%dh", interval),
		RetentionDays:     int32(retention),
		IntervalHours:     int32(interval),
		RetentionCount:    int32(retention),
		Timezone:          timezone,
		DrillEnabled:      drillEnabled,
		DrillIntervalDays: int32(drillInterval),
		TargetRef:         targetRef,
		Enabled:           false,
		StorageTarget:     targetRef,
		CreatedBy:         operator,
		UpdatedBy:         operator,
	}
	row.Normalize()
	saved, err := s.repo.Create(ctx, row)
	if err != nil {
		retErr = err
		return nil, retErr
	}
	s.audit(ctx, obsops.AuditRecord{ResourceType: "backup_policy", ResourceID: fmt.Sprintf("%d", saved.ID), Operation: "create", Outcome: "success", Severity: "info", Detail: map[string]any{"name": saved.Name, "interval_hours": saved.IntervalHours, "retention_count": saved.RetentionCount, "timezone": saved.Timezone, "trace_id": strings.TrimSpace(req.TraceID)}})
	return saved, nil
}

func (s *PolicyService) UpdatePolicy(ctx context.Context, req UpdatePolicyRequest) (*modelops.BackupPolicy, error) {
	startedAt := time.Now()
	var retErr error
	defer func() { s.metrics.Observe(ctx, "backup_update_policy", startedAt, retErr) }()

	if req.PolicyID == 0 {
		retErr = ErrInvalidBackupPolicy
		return nil, retErr
	}

	row, err := s.repo.GetById(ctx, req.PolicyID, nil)
	if err != nil {
		retErr = err
		return nil, retErr
	}
	if row == nil {
		retErr = ErrBackupPolicyNotFound
		return nil, retErr
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			retErr = ErrInvalidBackupPolicy
			return nil, retErr
		}
		row.Name = name
	}
	if req.IntervalHours != nil {
		if *req.IntervalHours <= 0 {
			retErr = ErrInvalidBackupPolicy
			return nil, retErr
		}
		row.IntervalHours = int32(*req.IntervalHours)
		row.Schedule = fmt.Sprintf("%dh", *req.IntervalHours)
	}
	if req.RetentionCount != nil {
		if *req.RetentionCount <= 0 {
			retErr = ErrInvalidBackupPolicy
			return nil, retErr
		}
		row.RetentionCount = int32(*req.RetentionCount)
		row.RetentionDays = int32(*req.RetentionCount)
	}
	if req.Timezone != nil {
		tz := strings.TrimSpace(*req.Timezone)
		if tz == "" {
			tz = defaultTimezone
		}
		if _, err := time.LoadLocation(tz); err != nil {
			retErr = ErrInvalidBackupPolicy
			return nil, retErr
		}
		row.Timezone = tz
	}
	if req.DrillEnabled != nil {
		row.DrillEnabled = *req.DrillEnabled
	}
	if req.DrillIntervalDay != nil {
		if *req.DrillIntervalDay <= 0 {
			retErr = ErrInvalidBackupPolicy
			return nil, retErr
		}
		row.DrillIntervalDays = int32(*req.DrillIntervalDay)
	}
	if req.TargetRef != nil {
		target := strings.TrimSpace(*req.TargetRef)
		if target == "" {
			retErr = ErrInvalidBackupPolicy
			return nil, retErr
		}
		row.TargetRef = target
		row.StorageTarget = target
	}

	row.UpdatedBy = normalizeOperator(req.Operator)
	row.Normalize()
	updated, err := s.repo.Update(ctx, row)
	if err != nil {
		retErr = err
		return nil, retErr
	}
	s.audit(ctx, obsops.AuditRecord{ResourceType: "backup_policy", ResourceID: fmt.Sprintf("%d", updated.ID), Operation: "update", Outcome: "success", Severity: "info", Detail: map[string]any{"name": updated.Name, "interval_hours": updated.IntervalHours, "retention_count": updated.RetentionCount, "timezone": updated.Timezone, "trace_id": strings.TrimSpace(req.TraceID)}})
	return updated, nil
}

func (s *PolicyService) SetPolicyEnabled(ctx context.Context, req SetPolicyEnabledRequest) error {
	startedAt := time.Now()
	var retErr error
	defer func() { s.metrics.Observe(ctx, "backup_set_policy_enabled", startedAt, retErr) }()

	if req.PolicyID == 0 {
		retErr = ErrInvalidBackupPolicy
		return retErr
	}
	row, err := s.repo.GetById(ctx, req.PolicyID, nil)
	if err != nil {
		retErr = err
		return retErr
	}
	if row == nil {
		retErr = ErrBackupPolicyNotFound
		return retErr
	}
	if err := s.repo.SetEnabled(ctx, req.PolicyID, req.Enabled, normalizeOperator(req.Operator)); err != nil {
		retErr = err
		return retErr
	}
	operation := "disable"
	if req.Enabled {
		operation = "enable"
	}
	s.audit(ctx, obsops.AuditRecord{ResourceType: "backup_policy", ResourceID: fmt.Sprintf("%d", req.PolicyID), Operation: operation, Outcome: "success", Severity: "info", Detail: map[string]any{"enabled": req.Enabled, "trace_id": strings.TrimSpace(req.TraceID)}})
	return nil
}

// UpsertPolicy 保留给 gRPC 兼容层，内部转换到 Create/Update。
func (s *PolicyService) UpsertPolicy(ctx context.Context, req UpsertPolicyRequest) (*modelops.BackupPolicy, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrInvalidBackupPolicy
	}
	existing, err := s.repo.GetFirst(ctx, map[string]interface{}{"name": name})
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if existing == nil {
		created, err := s.CreatePolicy(ctx, CreatePolicyRequest{
			Name:             req.Name,
			IntervalHours:    parseScheduleHours(req.Schedule),
			RetentionCount:   req.RetentionDays,
			Timezone:         defaultTimezone,
			DrillEnabled:     boolPtr(true),
			DrillIntervalDay: defaultDrillIntervalDay,
			TargetRef:        req.StorageTarget,
			Operator:         req.Operator,
			TraceID:          req.TraceID,
		})
		if err != nil {
			return nil, err
		}
		_ = s.SetPolicyEnabled(ctx, SetPolicyEnabledRequest{PolicyID: created.ID, Enabled: req.Enabled, Operator: req.Operator, TraceID: req.TraceID})
		return s.repo.GetById(ctx, created.ID, nil)
	}
	interval := parseScheduleHours(req.Schedule)
	retention := req.RetentionDays
	target := req.StorageTarget
	updated, err := s.UpdatePolicy(ctx, UpdatePolicyRequest{
		PolicyID:         existing.ID,
		IntervalHours:    &interval,
		RetentionCount:   &retention,
		TargetRef:        &target,
		Operator:         req.Operator,
		TraceID:          req.TraceID,
		DrillEnabled:     boolPtr(existing.DrillEnabled),
		DrillIntervalDay: intPtr(int(existing.DrillIntervalDays)),
	})
	if err != nil {
		return nil, err
	}
	if err := s.SetPolicyEnabled(ctx, SetPolicyEnabledRequest{PolicyID: updated.ID, Enabled: req.Enabled, Operator: req.Operator, TraceID: req.TraceID}); err != nil {
		return nil, err
	}
	return s.repo.GetById(ctx, updated.ID, nil)
}

func normalizePolicyValues(intervalHours, retentionCount int, timezone string, drillEnabled *bool, drillIntervalDay int, targetRef string) (int, int, string, bool, int, string, error) {
	interval := intervalHours
	if interval <= 0 {
		interval = defaultIntervalHours
	}
	retention := retentionCount
	if retention <= 0 {
		retention = defaultRetentionCount
	}
	tz := strings.TrimSpace(timezone)
	if tz == "" {
		tz = defaultTimezone
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return 0, 0, "", false, 0, "", ErrInvalidBackupPolicy
	}
	drill := true
	if drillEnabled != nil {
		drill = *drillEnabled
	}
	drillInterval := drillIntervalDay
	if drillInterval <= 0 {
		drillInterval = defaultDrillIntervalDay
	}
	target := strings.TrimSpace(targetRef)
	if target == "" {
		target = "powerx_bak"
	}
	return interval, retention, tz, drill, drillInterval, target, nil
}

func parseScheduleHours(schedule string) int {
	raw := strings.TrimSpace(strings.ToLower(schedule))
	if raw == "" {
		return defaultIntervalHours
	}
	if strings.HasSuffix(raw, "h") {
		num := strings.TrimSpace(strings.TrimSuffix(raw, "h"))
		if num != "" {
			var v int
			_, _ = fmt.Sscanf(num, "%d", &v)
			if v > 0 {
				return v
			}
		}
	}
	return defaultIntervalHours
}

func boolPtr(v bool) *bool {
	return &v
}

func intPtr(v int) *int {
	return &v
}

func (s *PolicyService) audit(ctx context.Context, rec obsops.AuditRecord) {
	if s.auditor == nil {
		return
	}
	_ = s.auditor.Write(ctx, rec)
}
