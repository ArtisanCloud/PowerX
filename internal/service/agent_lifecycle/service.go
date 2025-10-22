package agent_lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	imnotify "github.com/ArtisanCloud/PowerX/internal/notifications/im"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	agentinstr "github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle/instrumentation"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// Config 控制服务运行参数。
type Config struct {
	DefaultCapacityInstances int
	EventTopics              EventTopics
}

// EventTopics 定义事件主题前缀。
type EventTopics struct {
	LifecyclePrefix string
	HealthPrefix    string
}

// ServiceOptions 构建 Service 所需依赖。
type ServiceOptions struct {
	ProfileRepo     *agentrepo.AgentProfileLifecycleRepository
	LifecycleRepo   *agentrepo.AgentLifecycleEventRepository
	HealthRepo      *agentrepo.AgentHealthSnapshotRepository
	EventBus        event_bus.EventBus
	Instrumentation *agentinstr.Instrumentation
	Notifier        *imnotify.Sender
	Config          Config
	Clock           func() time.Time
}

// Service 封装 Agent 生命周期核心逻辑。
type Service struct {
	profiles *agentrepo.AgentProfileLifecycleRepository
	events   *agentrepo.AgentLifecycleEventRepository
	health   *agentrepo.AgentHealthSnapshotRepository
	bus      event_bus.EventBus
	instr    *agentinstr.Instrumentation
	notifier *imnotify.Sender
	config   Config
	clock    func() time.Time
}

// NewService 构建 Service。
func NewService(opts ServiceOptions) *Service {
	if opts.ProfileRepo == nil || opts.LifecycleRepo == nil {
		panic("agent lifecycle service requires repositories")
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if opts.Config.DefaultCapacityInstances <= 0 {
		opts.Config.DefaultCapacityInstances = 1
	}
	if opts.Instrumentation == nil {
		opts.Instrumentation = agentinstr.New(agentinstr.Options{})
	}

	return &Service{
		profiles: opts.ProfileRepo,
		events:   opts.LifecycleRepo,
		health:   opts.HealthRepo,
		bus:      opts.EventBus,
		instr:    opts.Instrumentation,
		notifier: opts.Notifier,
		config:   opts.Config,
		clock:    opts.Clock,
	}
}

// Register 创建新代理档案并记录初始生命周期事件。
func (s *Service) Register(ctx context.Context, in RegisterInput) (*LifecycleResult, error) {
	start := s.clock()
	if strings.TrimSpace(in.TenantID) == "" || strings.TrimSpace(in.Alias) == "" {
		return nil, fmt.Errorf("tenant_id and alias are required")
	}

	ctx, traceID := agentinstr.EnsureTraceContext(ctx)
	ctx = agentinstr.WithTenant(ctx, in.TenantID)

	if _, err := s.profiles.GetByTenantAlias(ctx, in.TenantID, in.Alias); err == nil {
		return nil, ErrAliasConflict
	} else if err != nil && !errors.Is(err, agentrepo.ErrAgentProfileNotFound) {
		return nil, err
	}

	if in.DefaultCapacityInstances <= 0 {
		in.DefaultCapacityInstances = int32(s.config.DefaultCapacityInstances)
	}

	toolGrantBytes, err := json.Marshal(in.ToolGrants)
	if err != nil {
		return nil, fmt.Errorf("marshal tool grants: %w", err)
	}
	metadataBytes, err := json.Marshal(in.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	eventTopicPrefix := in.EventTopicPrefix
	if eventTopicPrefix == "" {
		sanitized := strings.ReplaceAll(strings.ToLower(in.Alias), " ", "-")
		eventTopicPrefix = fmt.Sprintf("%s.%s", strings.TrimSuffix(s.config.EventTopics.LifecyclePrefix, "."), sanitized)
	}

	profile := &agentmodel.AgentProfileLifecycle{
		TenantID:                 in.TenantID,
		Alias:                    in.Alias,
		DisplayName:              defaultDisplayName(in.DisplayName, in.Alias),
		Status:                   "pending",
		ToolGrants:               datatypes.JSON(toolGrantBytes),
		TelemetryContractVersion: in.TelemetryContractVersion,
		DefaultCapacityInstances: in.DefaultCapacityInstances,
		MaxCapacityInstances:     in.MaxCapacityInstances,
		CurrentCapacityInstances: 0,
		EventTopicPrefix:         eventTopicPrefix,
		NotificationChannel:      in.NotificationChannel,
		Metadata:                 datatypes.JSON(metadataBytes),
		CreatedBy:                in.RequestedBy,
		UpdatedBy:                in.RequestedBy,
	}

	created, err := s.profiles.Create(ctx, profile)
	if err != nil {
		if errors.Is(err, agentrepo.ErrAgentAliasConflict) {
			return nil, ErrAliasConflict
		}
		return nil, err
	}

	lifecycleEvent, err := s.appendLifecycleEvent(ctx, created, "register", "", created.Status, in.RequestedBy, nil, "", in.TraceID, s.clock().Sub(start))
	if err != nil {
		return nil, err
	}

	s.publishLifecycle(ctx, "registered", map[string]any{
		"agent_id":  created.UUID.String(),
		"tenant_id": created.TenantID,
		"status":    created.Status,
	}, traceID)

	s.instr.AuditLifecycleEvent(ctx, in.TenantID, created.UUID.String(), "REGISTER_AGENT", "SUCCESS", map[string]any{
		"alias": in.Alias,
	})

	return &LifecycleResult{
		Agent: toAgent(created, in.ToolGrants, in.Metadata),
		Event: lifecycleEvent,
	}, nil
}

// Activate 将代理状态切换为运行中。
func (s *Service) Activate(ctx context.Context, in ActivateInput) (*LifecycleResult, error) {
	start := s.clock()
	if in.AgentID == uuid.Nil {
		return nil, fmt.Errorf("agent_id is required")
	}

	ctx, traceID := agentinstr.EnsureTraceContext(ctx)

	profile, err := s.profiles.GetByUUID(ctx, in.AgentID)
	if err != nil {
		if errors.Is(err, agentrepo.ErrAgentProfileNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	tenantID := strings.TrimSpace(in.TenantID)
	if tenantID == "" {
		tenantID = profile.TenantID
	}
	ctx = agentinstr.WithTenant(ctx, tenantID)

	if profile.Status == "active" {
		return &LifecycleResult{Agent: toAgent(profile, parseToolGrants(profile.ToolGrants), parseMetadata(profile.Metadata))}, nil
	}
	if profile.Status != "pending" && profile.Status != "paused" {
		return nil, ErrInvalidStatusTransition
	}

	fromStatus := profile.Status
	profile.Status = "active"
	profile.CurrentCapacityInstances = coalesceInt32(profile.CurrentCapacityInstances, profile.DefaultCapacityInstances)
	profile.UpdatedBy = in.RequestedBy
	if updated, err := s.profiles.Save(ctx, profile); err != nil {
		return nil, err
	} else {
		profile = updated
	}

	lifecycleEvent, err := s.appendLifecycleEvent(ctx, profile, "activate", fromStatus, profile.Status, in.RequestedBy, nil, in.Reason, traceID, s.clock().Sub(start))
	if err != nil {
		return nil, err
	}

	s.publishLifecycle(ctx, "activated", map[string]any{
		"agent_id":  profile.UUID.String(),
		"tenant_id": profile.TenantID,
		"from":      fromStatus,
		"to":        profile.Status,
	}, traceID)

	s.instr.AuditLifecycleEvent(ctx, tenantID, profile.UUID.String(), "ACTIVATE_AGENT", "SUCCESS", map[string]any{
		"from": fromStatus,
		"to":   profile.Status,
	})

	return &LifecycleResult{
		Agent: toAgent(profile, parseToolGrants(profile.ToolGrants), parseMetadata(profile.Metadata)),
		Event: lifecycleEvent,
	}, nil
}

func (s *Service) Pause(ctx context.Context, in PauseInput) (*LifecycleResult, error) {
	start := s.clock()
	if in.AgentID == uuid.Nil {
		return nil, fmt.Errorf("agent_id is required")
	}
	ctx, traceID := agentinstr.EnsureTraceContext(ctx)
	profile, err := s.profiles.GetByUUID(ctx, in.AgentID)
	if err != nil {
		if errors.Is(err, agentrepo.ErrAgentProfileNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	tenantID := strings.TrimSpace(in.TenantID)
	if tenantID == "" {
		tenantID = profile.TenantID
	}
	ctx = agentinstr.WithTenant(ctx, tenantID)

	if profile.Status != "active" {
		return nil, ErrInvalidStatusTransition
	}

	fromStatus := profile.Status
	profile.Status = "paused"
	profile.CurrentCapacityInstances = 0
	profile.UpdatedBy = in.RequestedBy

	if updated, err := s.profiles.Save(ctx, profile); err != nil {
		return nil, err
	} else {
		profile = updated
	}

	lifecycleEvent, err := s.appendLifecycleEvent(ctx, profile, "pause", fromStatus, profile.Status, in.RequestedBy, nil, in.Reason, traceID, s.clock().Sub(start))
	if err != nil {
		return nil, err
	}

	s.publishLifecycle(ctx, "paused", map[string]any{
		"agent_id":  profile.UUID.String(),
		"tenant_id": tenantID,
		"from":      fromStatus,
		"to":        profile.Status,
		"reason":    in.Reason,
	}, traceID)

	s.instr.AuditLifecycleEvent(ctx, tenantID, profile.UUID.String(), "PAUSE_AGENT", "SUCCESS", map[string]any{
		"from":   fromStatus,
		"to":     profile.Status,
		"reason": in.Reason,
	})

	return &LifecycleResult{
		Agent: toAgent(profile, parseToolGrants(profile.ToolGrants), parseMetadata(profile.Metadata)),
		Event: lifecycleEvent,
	}, nil
}

func (s *Service) Resume(ctx context.Context, in ResumeInput) (*LifecycleResult, error) {
	start := s.clock()
	if in.AgentID == uuid.Nil {
		return nil, fmt.Errorf("agent_id is required")
	}
	ctx, traceID := agentinstr.EnsureTraceContext(ctx)
	profile, err := s.profiles.GetByUUID(ctx, in.AgentID)
	if err != nil {
		if errors.Is(err, agentrepo.ErrAgentProfileNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	tenantID := strings.TrimSpace(in.TenantID)
	if tenantID == "" {
		tenantID = profile.TenantID
	}
	ctx = agentinstr.WithTenant(ctx, tenantID)

	if profile.Status != "paused" {
		return nil, ErrInvalidStatusTransition
	}

	fromStatus := profile.Status
	profile.Status = "active"
	profile.CurrentCapacityInstances = coalesceInt32(profile.CurrentCapacityInstances, profile.DefaultCapacityInstances)
	profile.UpdatedBy = in.RequestedBy

	if updated, err := s.profiles.Save(ctx, profile); err != nil {
		return nil, err
	} else {
		profile = updated
	}

	lifecycleEvent, err := s.appendLifecycleEvent(ctx, profile, "resume", fromStatus, profile.Status, in.RequestedBy, nil, in.Reason, traceID, s.clock().Sub(start))
	if err != nil {
		return nil, err
	}

	s.publishLifecycle(ctx, "resumed", map[string]any{
		"agent_id":  profile.UUID.String(),
		"tenant_id": tenantID,
		"from":      fromStatus,
		"to":        profile.Status,
	}, traceID)

	s.instr.AuditLifecycleEvent(ctx, tenantID, profile.UUID.String(), "RESUME_AGENT", "SUCCESS", map[string]any{
		"from": fromStatus,
		"to":   profile.Status,
	})

	return &LifecycleResult{
		Agent: toAgent(profile, parseToolGrants(profile.ToolGrants), parseMetadata(profile.Metadata)),
		Event: lifecycleEvent,
	}, nil
}

func (s *Service) Retire(ctx context.Context, in RetireInput) (*LifecycleResult, error) {
	start := s.clock()
	if in.AgentID == uuid.Nil {
		return nil, fmt.Errorf("agent_id is required")
	}
	ctx, traceID := agentinstr.EnsureTraceContext(ctx)
	profile, err := s.profiles.GetByUUID(ctx, in.AgentID)
	if err != nil {
		if errors.Is(err, agentrepo.ErrAgentProfileNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	tenantID := strings.TrimSpace(in.TenantID)
	if tenantID == "" {
		tenantID = profile.TenantID
	}
	ctx = agentinstr.WithTenant(ctx, tenantID)

	if profile.Status == "retired" {
		return &LifecycleResult{Agent: toAgent(profile, parseToolGrants(profile.ToolGrants), parseMetadata(profile.Metadata))}, nil
	}
	if profile.Status != "active" && profile.Status != "paused" {
		return nil, ErrInvalidStatusTransition
	}

	fromStatus := profile.Status
	profile.Status = "retired"
	profile.CurrentCapacityInstances = 0
	now := s.clock().UTC()
	profile.RetiredAt = &now
	profile.UpdatedBy = in.RequestedBy

	if updated, err := s.profiles.Save(ctx, profile); err != nil {
		return nil, err
	} else {
		profile = updated
	}

	lifecycleEvent, err := s.appendLifecycleEvent(ctx, profile, "retire", fromStatus, profile.Status, in.RequestedBy, nil, in.Reason, traceID, s.clock().Sub(start))
	if err != nil {
		return nil, err
	}

	s.publishLifecycle(ctx, "retired", map[string]any{
		"agent_id":  profile.UUID.String(),
		"tenant_id": tenantID,
		"from":      fromStatus,
		"to":        profile.Status,
		"reason":    in.Reason,
	}, traceID)

	s.instr.AuditLifecycleEvent(ctx, tenantID, profile.UUID.String(), "RETIRE_AGENT", "SUCCESS", map[string]any{
		"from":   fromStatus,
		"to":     profile.Status,
		"reason": in.Reason,
	})

	return &LifecycleResult{
		Agent: toAgent(profile, parseToolGrants(profile.ToolGrants), parseMetadata(profile.Metadata)),
		Event: lifecycleEvent,
	}, nil
}

func (s *Service) Scale(ctx context.Context, in ScaleInput) (*LifecycleResult, error) {
	start := s.clock()
	if in.AgentID == uuid.Nil {
		return nil, fmt.Errorf("agent_id is required")
	}
	if in.Target <= 0 {
		return nil, ErrInvalidCapacity
	}
	ctx, traceID := agentinstr.EnsureTraceContext(ctx)
	profile, err := s.profiles.GetByUUID(ctx, in.AgentID)
	if err != nil {
		if errors.Is(err, agentrepo.ErrAgentProfileNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	tenantID := strings.TrimSpace(in.TenantID)
	if tenantID == "" {
		tenantID = profile.TenantID
	}
	ctx = agentinstr.WithTenant(ctx, tenantID)

	if profile.Status != "active" {
		return nil, ErrInvalidStatusTransition
	}

	if profile.MaxCapacityInstances != nil && in.Target > *profile.MaxCapacityInstances {
		return nil, ErrCapacityExceeded
	}

	fromCapacity := profile.CurrentCapacityInstances
	profile.CurrentCapacityInstances = in.Target
	profile.UpdatedBy = in.RequestedBy

	if updated, err := s.profiles.Save(ctx, profile); err != nil {
		return nil, err
	} else {
		profile = updated
	}

	target := in.Target
	lifecycleEvent, err := s.appendLifecycleEvent(ctx, profile, "scale", profile.Status, profile.Status, in.RequestedBy, &target, in.Reason, traceID, s.clock().Sub(start))
	if err != nil {
		return nil, err
	}

	s.publishLifecycle(ctx, "scaled", map[string]any{
		"agent_id":      profile.UUID.String(),
		"tenant_id":     tenantID,
		"from_capacity": fromCapacity,
		"to_capacity":   in.Target,
		"reason":        in.Reason,
	}, traceID)

	s.instr.AuditLifecycleEvent(ctx, tenantID, profile.UUID.String(), "SCALE_AGENT", "SUCCESS", map[string]any{
		"from_capacity": fromCapacity,
		"to_capacity":   in.Target,
		"reason":        in.Reason,
	})

	return &LifecycleResult{
		Agent: toAgent(profile, parseToolGrants(profile.ToolGrants), parseMetadata(profile.Metadata)),
		Event: lifecycleEvent,
	}, nil
}

// Get 返回代理档案。
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Agent, error) {
	profile, err := s.profiles.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, agentrepo.ErrAgentProfileNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	return toAgent(profile, parseToolGrants(profile.ToolGrants), parseMetadata(profile.Metadata)), nil
}

// ListByTenant 返回租户下的代理集合。
func (s *Service) ListByTenant(ctx context.Context, tenantID string) ([]*Agent, error) {
	items, _, err := s.profiles.ListByTenant(ctx, tenantID, 0, 0)
	if err != nil {
		return nil, err
	}
	result := make([]*Agent, 0, len(items))
	for _, item := range items {
		tg := parseToolGrants(item.ToolGrants)
		meta := parseMetadata(item.Metadata)
		agent := toAgent(&item, tg, meta)
		result = append(result, agent)
	}
	return result, nil
}

func (s *Service) appendLifecycleEvent(ctx context.Context, profile *agentmodel.AgentProfileLifecycle, eventType, fromStatus, toStatus, triggeredBy string, requestedCapacity *int32, reason, traceID string, latency time.Duration) (*agentmodel.AgentLifecycleEventRecord, error) {
	if s.events == nil {
		return nil, nil
	}
	evt := &agentmodel.AgentLifecycleEventRecord{
		AgentUUID:         profile.UUID,
		TenantID:          profile.TenantID,
		EventType:         eventType,
		FromStatus:        fromStatus,
		ToStatus:          toStatus,
		RequestedCapacity: requestedCapacity,
		Reason:            reason,
		TriggeredBy:       triggeredBy,
		TraceID:           traceID,
		OccurredAt:        s.clock().UTC(),
	}
	event, err := s.events.Append(ctx, evt)
	if err != nil {
		return nil, err
	}
	if s.instr != nil {
		s.instr.RecordLifecycleTransition(ctx, eventType, toStatus, latency)
	}
	return event, nil
}

func (s *Service) publishLifecycle(ctx context.Context, action string, payload map[string]any, traceID string) {
	if s.bus == nil {
		return
	}
	topic := strings.TrimSuffix(s.config.EventTopics.LifecyclePrefix, ".")
	if topic != "" {
		topic = topic + "." + action
	} else {
		topic = action
	}
	s.bus.Publish(topic, map[string]any{
		"payload":  payload,
		"trace_id": traceID,
	}, ctx)
}

func toAgent(model *agentmodel.AgentProfileLifecycle, toolGrants []ToolGrant, metadata map[string]string) *Agent {
	if model == nil {
		return nil
	}
	return &Agent{
		ID:                       model.UUID,
		TenantID:                 model.TenantID,
		Alias:                    model.Alias,
		DisplayName:              model.DisplayName,
		Status:                   model.Status,
		ToolGrants:               toolGrants,
		TelemetryContractVersion: model.TelemetryContractVersion,
		DefaultCapacityInstances: model.DefaultCapacityInstances,
		MaxCapacityInstances:     model.MaxCapacityInstances,
		CurrentCapacityInstances: model.CurrentCapacityInstances,
		EventTopicPrefix:         model.EventTopicPrefix,
		NotificationChannel:      model.NotificationChannel,
		Metadata:                 metadata,
		CreatedAt:                model.CreatedAt,
		UpdatedAt:                model.UpdatedAt,
	}
}

func parseToolGrants(data datatypes.JSON) []ToolGrant {
	if len(data) == 0 {
		return nil
	}
	var grants []ToolGrant
	_ = json.Unmarshal(data, &grants)
	return grants
}

func parseMetadata(data datatypes.JSON) map[string]string {
	if len(data) == 0 {
		return nil
	}
	var meta map[string]string
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return meta
}

func defaultDisplayName(name, fallback string) string {
	if strings.TrimSpace(name) == "" {
		return fallback
	}
	return name
}

func coalesceInt32(current, fallback int32) int32 {
	if current > 0 {
		return current
	}
	if fallback > 0 {
		return fallback
	}
	return 0
}
