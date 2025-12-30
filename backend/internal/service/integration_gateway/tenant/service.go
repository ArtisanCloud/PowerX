package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	authorization "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/instrumentation"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ServiceOptions 控制租户服务初始化。
type ServiceOptions struct {
	DB              *gorm.DB
	RouteRepo       *repo.IntegrationRouteRepository
	InvocationRepo  *repo.IntegrationInvocationLogRepository
	EventRepo       *repo.IntegrationEventPublicationRepository
	Router          capabilityRouter
	RateLimiter     authorization.RateLimiter
	EventBus        event_bus.EventBus
	Instrumentation *instrumentation.Instrumentation
	Auditor         audit.Auditor
	ToolGrants      ToolGrantChecker
	Config          Config
	Clock           func() time.Time
}

// Service 处理租户调用统一 API 的核心逻辑。
type Service struct {
	db          *gorm.DB
	routes      *repo.IntegrationRouteRepository
	invocations *repo.IntegrationInvocationLogRepository
	events      *repo.IntegrationEventPublicationRepository

	router       capabilityRouter
	rateLimiter  authorization.RateLimiter
	bus          event_bus.EventBus
	telemetry    *telemetry
	auditor      audit.Auditor
	config       Config
	now          func() time.Time
	toolGrants   ToolGrantChecker
	instrumenter *instrumentation.Instrumentation
}

// NewService 构造租户服务实例。
func NewService(opts ServiceOptions) *Service {
	if opts.DB == nil && opts.RouteRepo == nil {
		panic("integration gateway tenant service requires DB or repositories")
	}

	db := opts.DB
	routeRepo := opts.RouteRepo
	if routeRepo == nil {
		routeRepo = repo.NewIntegrationRouteRepository(db)
	}
	invokeRepo := opts.InvocationRepo
	if invokeRepo == nil {
		invokeRepo = repo.NewIntegrationInvocationLogRepository(db)
	}
	eventRepo := opts.EventRepo
	if eventRepo == nil {
		eventRepo = repo.NewIntegrationEventPublicationRepository(db)
	}

	inst := opts.Instrumentation
	if inst == nil {
		inst = instrumentation.NewInstrumentation(nil)
	}

	aud := opts.Auditor
	if aud == nil {
		aud = audit.Noop{}
	}

	cfg := opts.Config
	if cfg.DefaultRateLimit.WindowSeconds <= 0 {
		cfg.DefaultRateLimit.WindowSeconds = 60
	}
	if cfg.DefaultRateLimit.Limit == 0 {
		cfg.DefaultRateLimit.Limit = 120
	}
	if cfg.DefaultRateLimit.Burst == 0 {
		cfg.DefaultRateLimit.Burst = cfg.DefaultRateLimit.Limit
	}
	if cfg.DefaultRateLimit.Scope == "" {
		cfg.DefaultRateLimit.Scope = "per_route_per_tenant"
	}

	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	return &Service{
		db:           db,
		routes:       routeRepo,
		invocations:  invokeRepo,
		events:       eventRepo,
		router:       opts.Router,
		rateLimiter:  opts.RateLimiter,
		bus:          opts.EventBus,
		telemetry:    newTelemetry(inst),
		auditor:      aud,
		config:       cfg,
		now:          clock,
		toolGrants:   opts.ToolGrants,
		instrumenter: inst,
	}
}

// ListRoutes 返回租户可访问的路由集合。
func (s *Service) ListRoutes(ctx context.Context, tenantUUID, capabilityID, channel string) ([]manager.Route, error) {
	tenantUUID, err := normalizeTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	channel = normalizeChannel(channel)

	records, _, err := s.routes.ListByTenant(ctx, tenantUUID, 0, 0)
	if err != nil {
		return nil, err
	}

	result := make([]manager.Route, 0, len(records))
	for i := range records {
		route := routeFromModel(&records[i], s.config)
		if route.LifecycleState != manager.LifecycleActive {
			continue
		}
		if route.Status != manager.StatusEnabled {
			continue
		}
		if capabilityID != "" && !strings.EqualFold(route.CapabilityID, capabilityID) {
			continue
		}
		if channel != "" && !containsChannel(route.Channels, channel) {
			continue
		}
		result = append(result, route)
	}
	return result, nil
}

// GetRoute 返回路由详情。
func (s *Service) GetRoute(ctx context.Context, tenantUUID, slug string) (manager.Route, error) {
	tenantUUID, err := normalizeTenantUUID(tenantUUID)
	if err != nil {
		return manager.Route{}, err
	}
	slug = normalizeSlug(slug)
	if slug == "" {
		return manager.Route{}, errors.New("route_slug required")
	}

	record, err := s.routes.GetBySlug(ctx, tenantUUID, slug)
	if err != nil {
		return manager.Route{}, ErrRouteNotAccessible{Slug: slug, TenantUUID: tenantUUID}
	}

	route := routeFromModel(record, s.config)
	if route.LifecycleState != manager.LifecycleActive || route.Status != manager.StatusEnabled {
		return manager.Route{}, ErrRouteNotAccessible{Slug: slug, TenantUUID: tenantUUID}
	}
	return route, nil
}

// Invoke 触发租户能力调用。
func (s *Service) Invoke(ctx context.Context, in InvokeInput) (result InvokeResult, err error) {
	tenantUUID, err := normalizeTenantUUID(in.TenantUUID)
	if err != nil {
		return result, err
	}
	slug := normalizeSlug(in.RouteSlug)
	if slug == "" {
		return result, errors.New("route_slug required")
	}
	channel := normalizeChannel(in.Channel)
	if channel == "" {
		channel = "http"
	}

	if strings.TrimSpace(in.TraceID) != "" {
		ctx = context.WithValue(ctx, "trace_id", strings.TrimSpace(in.TraceID))
	}
	ctx = instrumentation.WithTenant(ctx, tenantUUID)
	ctx, traceID := instrumentation.EnsureTraceContext(ctx)
	start := s.now()
	logger := s.telemetry.Instrumentation().Logger(ctx)

	record, err := s.routes.GetBySlug(ctx, tenantUUID, slug)
	if err != nil {
		return result, ErrRouteNotAccessible{Slug: slug, TenantUUID: tenantUUID}
	}

	route := routeFromModel(record, s.config)
	if route.LifecycleState != manager.LifecycleActive || route.Status != manager.StatusEnabled {
		return result, ErrRouteNotAccessible{Slug: slug, TenantUUID: tenantUUID}
	}
	if !containsChannel(route.Channels, channel) {
		return result, ErrChannelDisabled{RouteID: route.RouteID, Channel: channel}
	}

	// Tool Grant 校验
	if s.toolGrants != nil && len(route.ToolGrantIDs) > 0 {
		if err = s.toolGrants.Validate(ctx, tenantUUID, route.ToolGrantIDs); err != nil {
			s.telemetry.ObserveInvocation(ctx, InvokeStatusDenied, 0)
			_ = s.recordInvocation(ctx, route, traceID, time.Since(start), "denied", in.Payload, nil, "grant_denied", err.Error(), false, "")
			s.auditor.LogAPI(ctx, "integration_gateway.tenant.invoke", http.StatusForbidden, time.Since(start))
			return result, ErrToolGrantDenied{RouteID: route.RouteID, TenantUUID: tenantUUID, Grants: route.ToolGrantIDs, Reason: err.Error()}
		}
	}

	rateLimit := route.RateLimit
	limitResult, scope, err := s.checkRateLimit(ctx, route, rateLimit)
	if err != nil {
		return result, err
	}
	if !limitResult.Allowed {
		s.telemetry.ObserveRateLimit(ctx, scope)
		rlErr := RateLimitError{
			RouteID:    route.RouteID,
			Scope:      scope,
			RetryAfter: limitResult.ResetAfter,
			Remaining:  limitResult.Remaining,
		}
		resp := InvokeResult{
			Status:  InvokeStatusRateLimited,
			TraceID: traceID,
			RateLimit: &RateLimitResult{
				Scope:      scope,
				RetryAfter: limitResult.ResetAfter,
				Remaining:  limitResult.Remaining,
			},
		}
		resp.Duration = time.Since(start)
		published, pubErr := s.publishEvent(ctx, route, traceID, resp, channel)
		if pubErr != nil {
			logger.WarnF(ctx, "[integration_gateway.tenant] publish event failed: %v", pubErr)
		}
		_ = s.recordInvocation(ctx, route, traceID, time.Since(start), "rate_limited", in.Payload, nil, "rate_limited", rlErr.Error(), published && pubErr == nil, "")
		s.auditor.LogAPI(ctx, "integration_gateway.tenant.invoke", http.StatusTooManyRequests, time.Since(start))
		return resp, rlErr
	}

	payloadBytes, err := json.Marshal(in.Payload)
	if err != nil {
		return result, err
	}

	attributes := map[string]string{
		"tenant.uuid":   tenantUUID,
		"route.id":      route.RouteID.String(),
		"capability.id": route.CapabilityID,
		"channel":       channel,
	}
	spanCtx, span := s.telemetry.Instrumentation().Tracer().StartSpan(ctx, "integration_gateway.tenant.invoke", instrumentation.SpanAttributes(ctx, attributes))
	defer span.End(err)

	if s.router == nil {
		err = errors.New("integration gateway: router service unavailable")
		s.telemetry.ObserveInvocation(spanCtx, InvokeStatusFailed, time.Since(start))
		_ = s.recordInvocation(ctx, route, traceID, time.Since(start), "failed", in.Payload, nil, "router_unavailable", err.Error(), false, "")
		s.auditor.LogAPI(ctx, "integration_gateway.tenant.invoke", http.StatusFailedDependency, time.Since(start))
		return result, err
	}

	routerResult, routerErr := s.router.Invoke(spanCtx, router.InvokeRequest{
		CapabilityID: route.CapabilityID,
		TenantUUID:   route.TenantUUID,
		Payload:      payloadBytes,
	})

	duration := time.Since(start)
	result = InvokeResult{
		Status:             InvokeStatusOK,
		RoutedCapabilityID: route.CapabilityID,
		RoutedAdapter:      routerResult.AdapterID,
		TraceID:            traceID,
		DispatchedAt:       s.now(),
		Duration:           duration,
	}

	var responsePayload map[string]any
	if len(routerResult.Payload) > 0 {
		if err := json.Unmarshal(routerResult.Payload, &responsePayload); err != nil {
			responsePayload = map[string]any{"raw": string(routerResult.Payload)}
		}
	}
	result.Result = responsePayload

	status := "success"
	errorCode := ""
	errorMessage := ""
	if routerErr != nil || routerResult.Error != nil {
		status = "failed"
		result.Status = InvokeStatusFailed
		if routerErr != nil {
			errorMessage = routerErr.Error()
		} else if routerResult.Error != nil {
			errorMessage = routerResult.Error.Error()
		}
		errorCode = "router_failed"
		result.ErrorCode = errorCode
		result.ErrorMessage = errorMessage
		logger.WarnF(spanCtx, "[integration_gateway.tenant] router invoke failed: %v", errorMessage)
	} else {
		logger.InfoF(spanCtx, "[integration_gateway.tenant] router invoke success adapter=%s", routerResult.AdapterID)
	}

	published, pubErr := s.publishEvent(spanCtx, route, traceID, result, channel)
	if pubErr != nil {
		logger.WarnF(spanCtx, "[integration_gateway.tenant] publish event failed: %v", pubErr)
	}

	if recErr := s.recordInvocation(spanCtx, route, traceID, duration, status, in.Payload, result.Result, errorCode, errorMessage, published && pubErr == nil, result.RoutedAdapter); recErr != nil {
		logger.WarnF(spanCtx, "[integration_gateway.tenant] record invocation failed: %v", recErr)
	}

	if result.Status == InvokeStatusFailed {
		s.telemetry.ObserveInvocation(spanCtx, InvokeStatusFailed, duration)
		s.auditor.LogAPI(spanCtx, "integration_gateway.tenant.invoke", http.StatusFailedDependency, duration)
		if routerErr != nil {
			return result, routerErr
		}
		return result, routerResult.Error
	}

	s.telemetry.ObserveInvocation(spanCtx, InvokeStatusOK, duration)
	s.auditor.LogAPI(spanCtx, "integration_gateway.tenant.invoke", http.StatusOK, duration)
	return result, nil
}

func (s *Service) recordInvocation(
	ctx context.Context,
	route manager.Route,
	traceID string,
	duration time.Duration,
	status string,
	request map[string]any,
	response map[string]any,
	errorCode string,
	errorMessage string,
	eventPublished bool,
	routedAdapter string,
) error {
	reqJSON, _ := json.Marshal(request)
	respJSON, _ := json.Marshal(response)

	record := &models.IntegrationInvocationLog{
		RouteUUID:          route.RouteID,
		TenantUUID:         route.TenantUUID,
		TraceID:            traceID,
		Status:             status,
		DurationMS:         int(duration.Milliseconds()),
		RequestPayload:     datatypes.JSON(reqJSON),
		ResponsePayload:    datatypes.JSON(respJSON),
		RoutedCapabilityID: route.CapabilityID,
		RoutedAdapter:      routedAdapter,
		EventPublished:     eventPublished,
		ErrorCode:          errorCode,
		ErrorMessage:       errorMessage,
	}
	_, err := s.invocations.Create(ctx, record)
	return err
}

func (s *Service) publishEvent(ctx context.Context, route manager.Route, traceID string, result InvokeResult, channel string) (bool, error) {
	var topic string
	switch result.Status {
	case InvokeStatusOK, InvokeStatusAccepted:
		topic = s.config.EventTopics.InvocationSucceeded
	default:
		topic = s.config.EventTopics.InvocationFailed
	}
	if strings.TrimSpace(topic) == "" {
		return false, nil
	}

	payload := map[string]any{
		"route_id":             route.RouteID.String(),
		"route_slug":           route.RouteSlug,
		"tenant_uuid":          route.TenantUUID,
		"capability_id":        route.CapabilityID,
		"status":               result.Status,
		"trace_id":             traceID,
		"channel":              channel,
		"routed_capability_id": route.CapabilityID,
		"routed_adapter":       result.RoutedAdapter,
		"error_code":           result.ErrorCode,
		"error_message":        result.ErrorMessage,
		"duration_ms":          result.Duration.Milliseconds(),
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}

	event := &models.IntegrationEventPublication{
		RouteUUID:  route.RouteID,
		TenantUUID: route.TenantUUID,
		Topic:      topic,
		Payload:    datatypes.JSON(bytes),
		Status:     "pending",
		TraceID:    traceID,
	}
	if _, err := s.events.Create(ctx, event); err != nil {
		return false, err
	}
	if s.bus != nil {
		s.bus.Publish(topic, payload, ctx)
		s.auditor.LogBusPublish(ctx, topic, 1)
		return true, nil
	}
	return false, nil
}

func normalizeSlug(slug string) string {
	return strings.ToLower(strings.TrimSpace(slug))
}

func normalizeChannel(channel string) string {
	channel = strings.ToLower(strings.TrimSpace(channel))
	switch channel {
	case "", "http":
		return "http"
	case "grpc":
		return "http"
	case "mcp":
		return "mcp"
	default:
		return channel
	}
}

func containsChannel(channels []string, target string) bool {
	for _, c := range channels {
		if strings.EqualFold(c, target) {
			return true
		}
	}
	return false
}

func routeFromModel(model *models.IntegrationRoute, cfg Config) manager.Route {
	var toolGrants []string
	_ = json.Unmarshal(model.ToolGrantIDs, &toolGrants)

	var channels []string
	if err := json.Unmarshal(model.Channels, &channels); err != nil || len(channels) == 0 {
		channels = []string{"http"}
	}

	rateLimit := cfg.DefaultRateLimit
	if len(model.RateLimit) > 0 {
		_ = json.Unmarshal(model.RateLimit, &rateLimit)
		if rateLimit.Limit == 0 {
			rateLimit.Limit = cfg.DefaultRateLimit.Limit
		}
		if rateLimit.Burst == 0 {
			rateLimit.Burst = rateLimit.Limit
		}
		if rateLimit.WindowSeconds <= 0 {
			rateLimit.WindowSeconds = cfg.DefaultRateLimit.WindowSeconds
		}
		if strings.TrimSpace(rateLimit.Scope) == "" {
			rateLimit.Scope = cfg.DefaultRateLimit.Scope
		}
	}

	eventTopics := cfg.EventTopics
	if len(model.EventTopics) > 0 {
		var topics manager.EventTopics
		if err := json.Unmarshal(model.EventTopics, &topics); err == nil {
			if topics.InvocationSucceeded != "" {
				eventTopics.InvocationSucceeded = topics.InvocationSucceeded
			}
			if topics.InvocationFailed != "" {
				eventTopics.InvocationFailed = topics.InvocationFailed
			}
		}
	}

	return manager.Route{
		RouteID:         model.UUID,
		TenantUUID:      model.TenantUUID,
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

func normalizeTenantUUID(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", errors.New("tenant_uuid required")
	}
	if _, err := uuid.Parse(trimmed); err != nil {
		return "", fmt.Errorf("invalid tenant uuid: %w", err)
	}
	return trimmed, nil
}
