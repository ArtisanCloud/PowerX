package agent_lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	imnotify "github.com/ArtisanCloud/PowerX/internal/notifications/im"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	agentinstr "github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle/instrumentation"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// Notifier 定义告警通知发送器。
type Notifier interface {
	Send(ctx context.Context, msg imnotify.Message) error
}

// Config 控制服务运行参数。
type Config struct {
	DefaultCapacityInstances   int
	EventTopics                EventTopics
	HealthDegradedThreshold    int32
	HealthUnavailableThreshold int32
	SubscriptionCacheTTL       time.Duration
	AlertCooldown              time.Duration
	ShareReviewInterval        time.Duration
}

// EventTopics 定义事件主题前缀。
type EventTopics struct {
	LifecyclePrefix string
	HealthPrefix    string
}

const subscriptionMetadataKey = "agent_subscription_config"

type cachedSubscription struct {
	config    SubscriptionConfig
	expiresAt time.Time
}

// ServiceOptions 构建 Service 所需依赖。
type ServiceOptions struct {
	ProfileRepo       *agentrepo.AgentProfileLifecycleRepository
	LifecycleRepo     *agentrepo.AgentLifecycleEventRepository
	HealthRepo        *agentrepo.AgentHealthSnapshotRepository
	ShareRepo         *agentrepo.AgentShareRepository
	TenantFormRepo    *agentrepo.AgentTenantFormRepository
	EventBus          event_bus.EventBus
	Instrumentation   *agentinstr.Instrumentation
	Notifier          Notifier
	Config            Config
	Clock             func() time.Time
	ManifestValidator ManifestValidator
	SandboxRunner     SandboxRunner
	PolicyEngine      PolicyConflictEngine
	ApprovalFlow      ApprovalFlow
	ShareValidator    ShareValidator
	QuotaProvisioner  QuotaProvisioner
}

// Service 封装 Agent 生命周期核心逻辑。
type Service struct {
	profiles          *agentrepo.AgentProfileLifecycleRepository
	events            *agentrepo.AgentLifecycleEventRepository
	health            *agentrepo.AgentHealthSnapshotRepository
	shares            *agentrepo.AgentShareRepository
	tenantForms       *agentrepo.AgentTenantFormRepository
	bus               event_bus.EventBus
	instr             *agentinstr.Instrumentation
	notifier          Notifier
	config            Config
	clock             func() time.Time
	subscriptionMu    sync.RWMutex
	subscriptionCache map[uuid.UUID]cachedSubscription
	manifestValidator ManifestValidator
	sandboxRunner     SandboxRunner
	policyEngine      PolicyConflictEngine
	approvalFlow      ApprovalFlow
	shareValidator    ShareValidator
	quotaProvisioner  QuotaProvisioner
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
	if opts.Config.HealthDegradedThreshold <= 0 {
		opts.Config.HealthDegradedThreshold = 80
	}
	if opts.Config.HealthUnavailableThreshold <= 0 {
		opts.Config.HealthUnavailableThreshold = 50
	}
	if opts.Config.SubscriptionCacheTTL <= 0 {
		opts.Config.SubscriptionCacheTTL = 5 * time.Minute
	}
	if opts.Config.AlertCooldown <= 0 {
		opts.Config.AlertCooldown = 2 * time.Minute
	}
	if opts.Config.ShareReviewInterval <= 0 {
		opts.Config.ShareReviewInterval = 30 * 24 * time.Hour
	}
	if opts.Instrumentation == nil {
		opts.Instrumentation = agentinstr.New(agentinstr.Options{
			AlertCooldown: opts.Config.AlertCooldown,
		})
	}
	if opts.ManifestValidator == nil {
		opts.ManifestValidator = defaultManifestValidator{}
	}
	if opts.SandboxRunner == nil {
		opts.SandboxRunner = noopSandboxRunner{}
	}
	if opts.PolicyEngine == nil {
		opts.PolicyEngine = NewDefaultPolicyConflictEngine(PolicyEngineOptions{})
	}
	if opts.ApprovalFlow == nil {
		opts.ApprovalFlow = newInMemoryApprovalFlow()
	}
	if opts.ShareValidator == nil {
		opts.ShareValidator = NewTenantShareValidator(opts.PolicyEngine, opts.SandboxRunner)
	}
	if opts.QuotaProvisioner == nil {
		opts.QuotaProvisioner = NewDefaultQuotaProvisioner()
	}

	return &Service{
		profiles:          opts.ProfileRepo,
		events:            opts.LifecycleRepo,
		health:            opts.HealthRepo,
		shares:            opts.ShareRepo,
		tenantForms:       opts.TenantFormRepo,
		bus:               opts.EventBus,
		instr:             opts.Instrumentation,
		notifier:          opts.Notifier,
		config:            opts.Config,
		clock:             opts.Clock,
		subscriptionCache: make(map[uuid.UUID]cachedSubscription),
		manifestValidator: opts.ManifestValidator,
		sandboxRunner:     opts.SandboxRunner,
		policyEngine:      opts.PolicyEngine,
		approvalFlow:      opts.ApprovalFlow,
		shareValidator:    opts.ShareValidator,
		quotaProvisioner:  opts.QuotaProvisioner,
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

func (s *Service) subscriptionForProfile(profile *agentmodel.AgentProfileLifecycle) SubscriptionConfig {
	if profile == nil {
		return s.defaultSubscriptionConfig()
	}
	if cfg, ok := s.subscriptionFromCache(profile.UUID); ok {
		return cfg
	}
	meta := parseMetadata(profile.Metadata)
	cfg := s.defaultSubscriptionConfig()
	if meta != nil {
		if raw := meta[subscriptionMetadataKey]; raw != "" {
			var decoded SubscriptionConfig
			if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
				if sanitized, err := s.sanitizeSubscription(decoded, false); err == nil {
					cfg = sanitized
				}
			}
		}
	}
	s.storeSubscriptionCache(profile.UUID, cfg)
	return cfg
}

func (s *Service) subscriptionFromCache(agentID uuid.UUID) (SubscriptionConfig, bool) {
	if agentID == uuid.Nil {
		return SubscriptionConfig{}, false
	}
	s.subscriptionMu.RLock()
	entry, ok := s.subscriptionCache[agentID]
	s.subscriptionMu.RUnlock()
	if !ok || entry.expiresAt.Before(s.clock()) {
		return SubscriptionConfig{}, false
	}
	return entry.config, true
}

func (s *Service) storeSubscriptionCache(agentID uuid.UUID, cfg SubscriptionConfig) {
	if agentID == uuid.Nil {
		return
	}
	s.subscriptionMu.Lock()
	s.subscriptionCache[agentID] = cachedSubscription{
		config:    cfg,
		expiresAt: s.clock().Add(s.config.SubscriptionCacheTTL),
	}
	s.subscriptionMu.Unlock()
}

func (s *Service) defaultSubscriptionConfig() SubscriptionConfig {
	return SubscriptionConfig{
		MetricsFilter:  append([]string(nil), defaultSubscriptionMetrics...),
		HealthStatuses: append([]string(nil), defaultSubscriptionStatuses...),
	}
}

func (s *Service) sanitizeSubscription(cfg SubscriptionConfig, strict bool) (SubscriptionConfig, error) {
	statuses, err := normalizeStatusList(cfg.HealthStatuses, strict)
	if err != nil {
		return SubscriptionConfig{}, err
	}
	metrics := normalizeMetricsList(cfg.MetricsFilter)
	cfg.HealthStatuses = statuses
	cfg.MetricsFilter = metrics
	if !cfg.UpdatedAt.IsZero() {
		cfg.UpdatedAt = cfg.UpdatedAt.UTC()
	}
	return cfg, nil
}

func normalizeStatusList(values []string, strict bool) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, val := range values {
		key := normalizeStatus(val)
		if key == "" {
			continue
		}
		if _, ok := allowedSubscriptionStatuses[key]; !ok {
			if strict {
				return nil, ErrInvalidSubscription
			}
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	if len(result) == 0 {
		if strict {
			return nil, ErrInvalidSubscription
		}
		return append([]string(nil), defaultSubscriptionStatuses...), nil
	}
	return result, nil
}

func normalizeMetricsList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, val := range values {
		key := strings.TrimSpace(strings.ToLower(val))
		if key == "" {
			continue
		}
		if _, ok := allowedMetricsKeys[key]; !ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	if len(result) == 0 {
		return append([]string(nil), defaultSubscriptionMetrics...)
	}
	return result
}

func (s *Service) UpdateSubscription(ctx context.Context, input SubscriptionUpdateInput) (*SubscriptionConfig, error) {
	if input.AgentID == uuid.Nil {
		return nil, fmt.Errorf("agent_id is required")
	}
	profile, err := s.profiles.GetByUUID(ctx, input.AgentID)
	if err != nil {
		if errors.Is(err, agentrepo.ErrAgentProfileNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	if strings.TrimSpace(input.TenantID) != "" && !strings.EqualFold(profile.TenantID, input.TenantID) {
		return nil, fmt.Errorf("tenant mismatch")
	}

	cfg, err := s.sanitizeSubscription(input.Config, true)
	if err != nil {
		return nil, err
	}
	cfg.UpdatedAt = s.clock().UTC()

	meta := parseMetadata(profile.Metadata)
	if meta == nil {
		meta = map[string]string{}
	}
	bytes, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	meta[subscriptionMetadataKey] = string(bytes)
	profile.Metadata = encodeMetadata(meta)
	profile.UpdatedBy = input.RequestedBy

	if _, err := s.profiles.Save(ctx, profile); err != nil {
		return nil, err
	}
	s.storeSubscriptionCache(profile.UUID, cfg)
	return &cfg, nil
}

func (s *Service) GetSubscription(ctx context.Context, agentID uuid.UUID) (*SubscriptionConfig, error) {
	if agentID == uuid.Nil {
		return nil, fmt.Errorf("agent_id is required")
	}
	profile, err := s.profiles.GetByUUID(ctx, agentID)
	if err != nil {
		if errors.Is(err, agentrepo.ErrAgentProfileNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	cfg := s.subscriptionForProfile(profile)
	return &cfg, nil
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

func (s *Service) publishHealth(ctx context.Context, status string, payload map[string]any, traceID string) {
	if s.bus == nil {
		return
	}
	topic := strings.TrimSuffix(s.config.EventTopics.HealthPrefix, ".")
	if topic != "" {
		topic = topic + "." + status
	} else {
		topic = "agent.health." + status
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

func encodeMetadata(meta map[string]string) datatypes.JSON {
	if meta == nil {
		meta = map[string]string{}
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(b)
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
