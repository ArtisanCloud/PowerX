package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/instrumentation"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// Config 描述服务运行所需的基本配置。
type Config struct {
	RateLimitPrefix  string
	DefaultRateLimit RateLimitPolicy
	EventTopics      EventTopics
}

// ServiceOptions 初始化选项。
type ServiceOptions struct {
	DB              *gorm.DB
	RouteRepo       *repo.IntegrationRouteRepository
	VersionRepo     *repo.IntegrationRouteVersionRepository
	EventRepo       *repo.IntegrationEventPublicationRepository
	EventBus        event_bus.EventBus
	Instrumentation *instrumentation.Instrumentation
	Auditor         audit.Auditor
	Config          Config
	Clock           func() time.Time
}

// Service 管理集成入口生命周期。
type Service struct {
	db       *gorm.DB
	routes   *repo.IntegrationRouteRepository
	versions *repo.IntegrationRouteVersionRepository
	events   *repo.IntegrationEventPublicationRepository
	bus      event_bus.EventBus

	instrumentation *instrumentation.Instrumentation
	auditor         audit.Auditor
	config          Config
	now             func() time.Time
}

// NewService 构造服务实例。
func NewService(opts ServiceOptions) *Service {
	if opts.DB == nil && opts.RouteRepo == nil {
		panic("integration gateway manager service requires DB or repositories")
	}

	db := opts.DB
	routeRepo := opts.RouteRepo
	if routeRepo == nil {
		routeRepo = repo.NewIntegrationRouteRepository(db)
	}
	versionRepo := opts.VersionRepo
	if versionRepo == nil {
		versionRepo = repo.NewIntegrationRouteVersionRepository(db)
	}
	eventRepo := opts.EventRepo
	if eventRepo == nil {
		eventRepo = repo.NewIntegrationEventPublicationRepository(db)
	}

	inst := opts.Instrumentation
	if inst == nil {
		inst = instrumentation.NewInstrumentation(nil)
	}

	auditor := opts.Auditor
	if auditor == nil {
		auditor = audit.Noop{}
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	cfg := opts.Config
	if cfg.DefaultRateLimit.WindowSeconds <= 0 {
		cfg.DefaultRateLimit.WindowSeconds = 60
	}
	if cfg.DefaultRateLimit.Scope == "" {
		cfg.DefaultRateLimit.Scope = "per_route_per_tenant"
	}

	return &Service{
		db:              db,
		routes:          routeRepo,
		versions:        versionRepo,
		events:          eventRepo,
		bus:             opts.EventBus,
		instrumentation: inst,
		auditor:         auditor,
		config:          cfg,
		now:             clock,
	}
}

// CreateRouteInput 描述创建集成路由的输入。
type CreateRouteInput struct {
	TenantID     string
	Actor        string
	RouteSlug    string
	CapabilityID string
	ToolGrantIDs []string
	Channels     []string
	RateLimit    *RateLimitPolicy
	EventTopics  *EventTopics
	Description  string
}

// UpdateRouteInput 描述更新路由的输入。
type UpdateRouteInput struct {
	RouteID      uuid.UUID
	TenantID     string
	Actor        string
	Version      uint32
	CapabilityID string
	ToolGrantIDs []string
	Channels     []string
	RateLimit    *RateLimitPolicy
	EventTopics  *EventTopics
	Description  *string
	Status       *string
}

// ChangeLifecycleInput 描述生命周期变更的输入。
type ChangeLifecycleInput struct {
	RouteID  uuid.UUID
	TenantID string
	Actor    string
	Action   string
	Reason   string
	Version  uint32
}

// ListRoutesInput 控制列表查询。
type ListRoutesInput struct {
	TenantID       string
	CapabilityID   string
	LifecycleState string
	Page           int
	PageSize       int
}

// CreateRoute 创建新的集成入口。
func (s *Service) CreateRoute(ctx context.Context, in CreateRouteInput) (Route, error) {
	ctx, traceID := instrumentation.EnsureTraceContext(ctx)
	start := s.now()

	if err := validateSlug(in.RouteSlug); err != nil {
		return Route{}, err
	}
	slug := normalizeSlug(in.RouteSlug)
	channels := normalizeChannels(in.Channels)
	toolGrants := normalizeToolGrants(in.ToolGrantIDs)

	policy := s.resolveRateLimit(in.RateLimit)
	if err := validateRateLimit(policy); err != nil {
		return Route{}, err
	}
	topics := s.resolveEventTopics(in.EventTopics)

	actor := strings.TrimSpace(in.Actor)
	if actor == "" {
		actor = "system"
	}

	now := s.now().UTC()
	routeModel := &models.IntegrationRoute{
		TenantID:        in.TenantID,
		RouteSlug:       slug,
		CapabilityID:    in.CapabilityID,
		ToolGrantIDs:    mustJSON(toolGrants),
		Channels:        mustJSON(channels),
		RateLimit:       mustJSON(policy),
		EventTopics:     mustJSON(topics),
		LifecycleState:  LifecycleActive,
		Status:          StatusEnabled,
		CurrentVersion:  1,
		Description:     in.Description,
		CreatedBy:       actor,
		UpdatedBy:       actor,
		LastActivityAt:  &now,
		LastPublishedAt: &now,
	}

	if _, err := s.routes.GetBySlug(ctx, in.TenantID, slug); err == nil {
		return Route{}, ErrSlugConflict
	} else if err != nil && !errors.Is(err, repo.ErrRouteNotFound) {
		return Route{}, err
	}

	if _, err := s.routes.Create(ctx, routeModel); err != nil {
		return Route{}, err
	}

	versionModel := &models.IntegrationRouteVersion{
		RouteUUID:  routeModel.UUID,
		Version:    routeModel.CurrentVersion,
		Snapshot:   mustJSON(routeFromModel(routeModel)),
		ChangeType: "created",
		ChangedBy:  actor,
		TraceID:    traceID,
		ChangedAt:  now,
	}

	if _, err := s.versions.Create(ctx, versionModel); err != nil {
		return Route{}, err
	}

	if err := s.enqueueEvent(ctx, routeModel, topics.Created, "created", actor, traceID); err != nil {
		return Route{}, err
	}

	out := routeFromModel(routeModel)
	s.auditor.LogAPI(ctx, "integration_gateway.admin.create", http.StatusCreated, s.now().Sub(start))
	return out, nil
}

// UpdateRoute 更新集成入口配置。
func (s *Service) UpdateRoute(ctx context.Context, in UpdateRouteInput) (Route, error) {
	ctx, traceID := instrumentation.EnsureTraceContext(ctx)
	start := s.now()

	if in.Version == 0 {
		return Route{}, ErrVersionConflict
	}

	actor := strings.TrimSpace(in.Actor)
	if actor == "" {
		actor = "system"
	}

	current, err := s.routes.GetByUUID(ctx, in.RouteID)
	if err != nil {
		if errors.Is(err, repo.ErrRouteNotFound) {
			return Route{}, ErrRouteNotFound
		}
		return Route{}, err
	}
	if strings.TrimSpace(in.TenantID) != "" && current.TenantID != strings.TrimSpace(in.TenantID) {
		return Route{}, errors.New("tenant mismatch")
	}
	if current.TenantID != in.TenantID {
		return Route{}, errors.New("tenant mismatch")
	}
	if current.CurrentVersion != in.Version {
		return Route{}, ErrVersionConflict
	}

	var channels []string
	if len(in.Channels) > 0 {
		channels = normalizeChannels(in.Channels)
	}
	var toolGrants []string
	if len(in.ToolGrantIDs) > 0 {
		toolGrants = normalizeToolGrants(in.ToolGrantIDs)
	}

	var policy RateLimitPolicy
	policyUpdated := false
	if in.RateLimit != nil {
		policy = *in.RateLimit
		policy.Scope = normalizeScope(policy.Scope)
		if err := validateRateLimit(policy); err != nil {
			return Route{}, err
		}
		policyUpdated = true
	}

	var topics EventTopics
	topicsUpdated := false
	if in.EventTopics != nil {
		topics = s.resolveEventTopics(in.EventTopics)
		topicsUpdated = true
	}

	nextVersion := current.CurrentVersion + 1
	updatedRoute := models.IntegrationRoute{}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var mutateErr error
		updated, err := s.routes.UpdateWithVersion(ctx, current.UUID, current.CurrentVersion, func(r *models.IntegrationRoute) {
			r.CurrentVersion = nextVersion
			r.UpdatedBy = actor
			now := s.now().UTC()
			r.LastActivityAt = &now
			if in.CapabilityID != "" {
				r.CapabilityID = in.CapabilityID
			}
			if len(toolGrants) > 0 {
				r.ToolGrantIDs = mustJSON(toolGrants)
			}
			if len(channels) > 0 {
				r.Channels = mustJSON(channels)
			}
			if policyUpdated {
				r.RateLimit = mustJSON(policy)
			}
			if topicsUpdated {
				r.EventTopics = mustJSON(topics)
			}
			if in.Description != nil {
				r.Description = strings.TrimSpace(*in.Description)
			}
			if in.Status != nil && (*in.Status == StatusEnabled || *in.Status == StatusDisabled) {
				r.Status = *in.Status
			}
		})
		if err != nil {
			mutateErr = err
			return err
		}
		updatedRoute = *updated

		version := &models.IntegrationRouteVersion{
			RouteUUID:  updatedRoute.UUID,
			Version:    updatedRoute.CurrentVersion,
			Snapshot:   mustJSON(routeFromModel(&updatedRoute)),
			ChangeType: "updated",
			ChangedBy:  actor,
			TraceID:    traceID,
			ChangedAt:  s.now().UTC(),
		}
		if _, err := s.versions.Create(ctx, version); err != nil {
			return err
		}
		return mutateErr
	})
	if err != nil {
		if errors.Is(err, repo.ErrVersionConflict) {
			return Route{}, ErrVersionConflict
		}
		if errors.Is(err, repo.ErrRouteNotFound) {
			return Route{}, ErrRouteNotFound
		}
		return Route{}, err
	}

	resolvedTopics := s.resolveEventTopics(nil)
	if topicsUpdated {
		resolvedTopics = topics
	} else {
		currentTopics := routeEventTopics(current.EventTopics, s.config.EventTopics)
		resolvedTopics = currentTopics
	}

	if err := s.enqueueEvent(ctx, &updatedRoute, resolvedTopics.Updated, "updated", actor, traceID); err != nil {
		return Route{}, err
	}
	out := routeFromModel(&updatedRoute)
	s.auditor.LogAPI(ctx, "integration_gateway.admin.update", http.StatusOK, s.now().Sub(start))
	return out, nil
}

// ChangeLifecycle 更新路由生命周期状态。
func (s *Service) ChangeLifecycle(ctx context.Context, in ChangeLifecycleInput) (Route, error) {
	ctx, traceID := instrumentation.EnsureTraceContext(ctx)
	start := s.now()

	if in.RouteID == uuid.Nil {
		return Route{}, errors.New("route_id required")
	}
	action := strings.ToLower(strings.TrimSpace(in.Action))
	if action == "" {
		return Route{}, errors.New("action required")
	}

	current, err := s.routes.GetByUUID(ctx, in.RouteID)
	if err != nil {
		if errors.Is(err, repo.ErrRouteNotFound) {
			return Route{}, ErrRouteNotFound
		}
		return Route{}, err
	}
	if in.Version > 0 && in.Version != current.CurrentVersion {
		return Route{}, ErrVersionConflict
	}

	targetState, targetStatus, changeType, err := transitionLifecycle(current.LifecycleState, action)
	if err != nil {
		return Route{}, err
	}

	actor := strings.TrimSpace(in.Actor)
	if actor == "" {
		actor = "system"
	}

	var updatedRoute models.IntegrationRoute
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated, err := s.routes.UpdateWithVersion(ctx, current.UUID, current.CurrentVersion, func(r *models.IntegrationRoute) {
			r.LifecycleState = targetState
			r.Status = targetStatus
			r.CurrentVersion = r.CurrentVersion + 1
			r.UpdatedBy = actor
			now := s.now().UTC()
			r.LastActivityAt = &now
		})
		if err != nil {
			return err
		}
		updatedRoute = *updated
		version := &models.IntegrationRouteVersion{
			RouteUUID:     updatedRoute.UUID,
			Version:       updatedRoute.CurrentVersion,
			Snapshot:      mustJSON(routeFromModel(&updatedRoute)),
			ChangeType:    changeType,
			ChangeSummary: strings.TrimSpace(in.Reason),
			ChangedBy:     actor,
			TraceID:       traceID,
			ChangedAt:     s.now().UTC(),
		}
		_, err = s.versions.Create(ctx, version)
		return err
	})
	if err != nil {
		if errors.Is(err, repo.ErrVersionConflict) {
			return Route{}, ErrVersionConflict
		}
		if errors.Is(err, repo.ErrRouteNotFound) {
			return Route{}, ErrRouteNotFound
		}
		return Route{}, err
	}

	if err := s.enqueueEvent(ctx, &updatedRoute, s.config.EventTopics.Updated, changeType, actor, traceID); err != nil {
		return Route{}, err
	}

	out := routeFromModel(&updatedRoute)
	statusCode := http.StatusOK
	switch action {
	case "retire":
		statusCode = http.StatusOK
	case "suspend":
		statusCode = http.StatusOK
	case "resume":
		statusCode = http.StatusOK
	}
	s.auditor.LogAPI(ctx, fmt.Sprintf("integration_gateway.admin.lifecycle.%s", action), statusCode, s.now().Sub(start))
	return out, nil
}

// GetRoute 获取路由信息。
func (s *Service) GetRoute(ctx context.Context, routeID uuid.UUID) (Route, error) {
	model, err := s.routes.GetByUUID(ctx, routeID)
	if err != nil {
		if errors.Is(err, repo.ErrRouteNotFound) {
			return Route{}, ErrRouteNotFound
		}
		return Route{}, err
	}
	return routeFromModel(model), nil
}

// ListRoutes 返回租户下路由列表。
func (s *Service) ListRoutes(ctx context.Context, in ListRoutesInput) ([]Route, int64, error) {
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PageSize <= 0 || in.PageSize > 100 {
		in.PageSize = 20
	}

	routes, total, err := s.routes.ListByTenant(ctx, in.TenantID, (in.Page-1)*in.PageSize, in.PageSize)
	if err != nil {
		return nil, 0, err
	}
	result := make([]Route, 0, len(routes))
	for _, item := range routes {
		if in.CapabilityID != "" && item.CapabilityID != in.CapabilityID {
			continue
		}
		if in.LifecycleState != "" && item.LifecycleState != in.LifecycleState {
			continue
		}
		result = append(result, routeFromModel(&item))
	}
	return result, total, nil
}

// ListVersions 按路由返回历史版本。
func (s *Service) ListVersions(ctx context.Context, routeID uuid.UUID) ([]RouteVersion, error) {
	records, err := s.versions.ListByRoute(ctx, routeID, 20)
	if err != nil {
		return nil, err
	}
	result := make([]RouteVersion, 0, len(records))
	for _, record := range records {
		var snapshot Route
		_ = json.Unmarshal(record.Snapshot, &snapshot)
		result = append(result, RouteVersion{
			Version:       record.Version,
			ChangeType:    record.ChangeType,
			ChangeSummary: record.ChangeSummary,
			ChangedBy:     record.ChangedBy,
			TraceID:       record.TraceID,
			ChangedAt:     record.ChangedAt,
			Snapshot:      snapshot,
		})
	}
	return result, nil
}

func (s *Service) resolveRateLimit(in *RateLimitPolicy) RateLimitPolicy {
	if in == nil {
		return s.config.DefaultRateLimit
	}
	policy := *in
	if policy.Limit == 0 {
		policy.Limit = s.config.DefaultRateLimit.Limit
	}
	if policy.Burst == 0 {
		policy.Burst = policy.Limit
	}
	if policy.WindowSeconds <= 0 {
		policy.WindowSeconds = s.config.DefaultRateLimit.WindowSeconds
	}
	policy.Scope = normalizeScope(policy.Scope)
	return policy
}

func (s *Service) resolveEventTopics(in *EventTopics) EventTopics {
	if in == nil {
		return s.config.EventTopics
	}
	result := s.config.EventTopics
	if in.Created != "" {
		result.Created = in.Created
	}
	if in.Updated != "" {
		result.Updated = in.Updated
	}
	if in.InvocationSucceeded != "" {
		result.InvocationSucceeded = in.InvocationSucceeded
	}
	if in.InvocationFailed != "" {
		result.InvocationFailed = in.InvocationFailed
	}
	return result
}

func (s *Service) enqueueEvent(ctx context.Context, route *models.IntegrationRoute, topic string, change string, actor string, traceID string) error {
	if strings.TrimSpace(topic) == "" {
		return nil
	}
	payload := EventPayload{
		Route:   routeFromModel(route),
		Actor:   actor,
		Change:  change,
		TraceID: traceID,
	}
	eventModel := &models.IntegrationEventPublication{
		RouteUUID: route.UUID,
		TenantID:  route.TenantID,
		Topic:     topic,
		Payload:   mustJSON(payload),
		Status:    "pending",
		TraceID:   traceID,
	}
	if _, err := s.events.Create(ctx, eventModel); err != nil {
		return err
	}
	if s.bus != nil {
		s.bus.Publish(topic, payload, ctx)
		s.auditor.LogBusPublish(ctx, topic, 1)
	}
	return nil
}

func routeFromModel(model *models.IntegrationRoute) Route {
	var toolGrants []string
	_ = json.Unmarshal(model.ToolGrantIDs, &toolGrants)

	var channels []string
	_ = json.Unmarshal(model.Channels, &channels)

	rateLimit := RateLimitPolicy{}
	_ = json.Unmarshal(model.RateLimit, &rateLimit)

	eventTopics := routeEventTopics(model.EventTopics, EventTopics{})

	return Route{
		RouteID:         model.UUID,
		TenantID:        model.TenantID,
		RouteSlug:       model.RouteSlug,
		CapabilityID:    model.CapabilityID,
		ToolGrantIDs:    toolGrants,
		Channels:        channels,
		RateLimit:       rateLimit,
		EventTopics:     eventTopics,
		LifecycleState:  model.LifecycleState,
		Status:          model.Status,
		CurrentVersion:  model.CurrentVersion,
		Description:     model.Description,
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
		LastActivityAt:  model.LastActivityAt,
		LastPublishedAt: model.LastPublishedAt,
	}
}

func routeEventTopics(raw datatypes.JSON, fallback EventTopics) EventTopics {
	if len(raw) == 0 {
		return fallback
	}
	var topics EventTopics
	if err := json.Unmarshal(raw, &topics); err != nil {
		return fallback
	}
	if topics.Created == "" {
		topics.Created = fallback.Created
	}
	if topics.Updated == "" {
		topics.Updated = fallback.Updated
	}
	if topics.InvocationSucceeded == "" {
		topics.InvocationSucceeded = fallback.InvocationSucceeded
	}
	if topics.InvocationFailed == "" {
		topics.InvocationFailed = fallback.InvocationFailed
	}
	return topics
}

func mustJSON(v interface{}) datatypes.JSON {
	bytes, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return datatypes.JSON(bytes)
}

func transitionLifecycle(current, action string) (string, string, string, error) {
	switch action {
	case "suspend":
		if current != LifecycleActive {
			return "", "", "", fmt.Errorf("cannot suspend route in %s state", current)
		}
		return LifecycleSuspended, StatusDisabled, "suspended", nil
	case "resume":
		if current != LifecycleSuspended {
			return "", "", "", fmt.Errorf("cannot resume route in %s state", current)
		}
		return LifecycleActive, StatusEnabled, "resumed", nil
	case "retire":
		if current == LifecycleRetired {
			return current, StatusDisabled, "retired", nil
		}
		return LifecycleRetired, StatusDisabled, "retired", nil
	default:
		return "", "", "", fmt.Errorf("unknown lifecycle action: %s", action)
	}
}
