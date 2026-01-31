package router

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	registryRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
)

var (
	// ErrNoAdapter 表示无可用适配器。
	ErrNoAdapter = errors.New("capability router: no adapter available")
)

// Service 路由领域服务（占位实现，后续填充具体逻辑）。
type Service struct {
	registryRepo registry.Repository
	healthRepo   HealthRepository
	eventBus     eventPublisher
	instrument   *domain.Instrumentation
	metrics      MetricsRecorder
	now          func() time.Time
	stateMu      sync.RWMutex
	adapterState map[string]map[string]healthState
	stickyMu     sync.RWMutex
	stickyMap    map[string]string
}

type eventPublisher interface {
	Publish(eventType string, payload interface{}, ctx context.Context)
}

// NewService 创建 Router 服务实例（占位实现）。
func NewService(opts ServiceOptions) *Service {
	registryRepository := opts.RegistryRepository
	if registryRepository == nil {
		if opts.DB == nil {
			panic("capability router: DB required when RegistryRepository is nil")
		}
		registryRepository = registryRepo.NewCapabilityRegistryRepository(opts.DB)
	}
	healthRepository := opts.HealthRepository
	if healthRepository == nil {
		healthRepository = NewMemoryRepository()
	}
	inst := opts.Instrumentation
	if inst == nil {
		inst = domain.NewInstrumentation(nil)
	}
	metrics := opts.Metrics
	if metrics == nil {
		metrics = NewRouterMetrics(inst)
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Service{
		registryRepo: registryRepository,
		healthRepo:   healthRepository,
		eventBus:     opts.EventBus,
		instrument:   inst,
		metrics:      metrics,
		now:          clock,
		adapterState: make(map[string]map[string]healthState),
		stickyMap:    make(map[string]string),
	}
}

// Invoke 根据策略选择适配器。
func (s *Service) Invoke(ctx context.Context, in InvokeRequest) (_ InvokeResult, err error) {
	if s.registryRepo == nil {
		return InvokeResult{}, errors.New("capability router: registry repository missing")
	}
	if in.CapabilityID == "" || in.TenantUUID == "" {
		return InvokeResult{}, errors.New("capability router: capability/tenant required")
	}

	ctx, _ = domain.EnsureTraceContext(ctx)
	attributes := domain.SpanAttributes(ctx, map[string]string{
		"capability.id": in.CapabilityID,
		"tenant.uuid":   in.TenantUUID,
		"router.mode":   "invoke",
	})
	spanCtx, span := s.instrument.Tracer().StartSpan(ctx, "capability.router.invoke", attributes)
	defer span.End(err)

	reg, err := s.registryRepo.GetLatest(spanCtx, nil, in.CapabilityID, in.TenantUUID)
	if err != nil {
		s.metrics.ObserveInvocation(spanCtx, "invoke", in.CapabilityID, in.TenantUUID, "", "", 0, false, err)
		return InvokeResult{}, err
	}
	result, err := s.routeWithRegistration(spanCtx, reg, in, true)
	if err != nil {
		s.metrics.ObserveInvocation(spanCtx, "invoke", in.CapabilityID, in.TenantUUID, "", "", 0, false, err)
	}
	return result, err
}

// Simulate 用于 Sandbox 模式下复用 Router 策略逻辑，但不会持久化状态。
func (s *Service) Simulate(ctx context.Context, reg registry.Registration, in InvokeRequest) (_ InvokeResult, err error) {
	if in.CapabilityID == "" {
		in.CapabilityID = reg.CapabilityID
	}
	if in.TenantUUID == "" {
		in.TenantUUID = tenantUUIDValue(reg)
	}
	ctx, _ = domain.EnsureTraceContext(ctx)
	attributes := domain.SpanAttributes(ctx, map[string]string{
		"capability.id": in.CapabilityID,
		"tenant.uuid":   in.TenantUUID,
		"router.mode":   "sandbox",
	})
	spanCtx, span := s.instrument.Tracer().StartSpan(ctx, "capability.router.simulate", attributes)
	defer span.End(err)
	result, err := s.routeWithRegistration(spanCtx, reg, in, false)
	if err != nil {
		s.metrics.ObserveInvocation(spanCtx, "sandbox", in.CapabilityID, in.TenantUUID, "", "", 0, false, err)
	}
	return result, err
}

func (s *Service) routeWithRegistration(ctx context.Context, reg registry.Registration, in InvokeRequest, mutate bool) (InvokeResult, error) {
	start := s.now()
	selection, err := s.selectAdapter(ctx, reg, in)
	if err != nil {
		if errors.Is(err, ErrNoAdapter) {
			selection.latency = s.now().Sub(start)
			result := InvokeResult{
				FallbackUsed: selection.fallbackUsed,
				Payload:      selection.payload,
				Latency:      selection.latency,
			}
			if mutate {
				reason := fallbackReasonFromSelection(selection)
				if errSync := s.syncFallbackState(ctx, reg, reason); errSync != nil {
					s.instrument.Logger(ctx).WarnF(ctx, "router fallback sync failed: %v", errSync)
				}
			}
			observeErr := error(nil)
			if !selection.fallbackUsed {
				observeErr = ErrNoAdapter
			}
			s.observeInvocation(ctx, reg, selection, result, mutate, observeErr)
			if mutate && selection.fallbackUsed {
				s.publishEvent(ctx, in, result)
			}
			return result, nil
		}
		return InvokeResult{}, err
	}

	if mutate && in.StickyKey != "" {
		key := stickyKeyFor(in.TenantUUID, in.CapabilityID, in.StickyKey)
		s.stickyMu.Lock()
		s.stickyMap[key] = selection.adapterID
		s.stickyMu.Unlock()
	}

	selection.latency = s.now().Sub(start)
	result := InvokeResult{
		AdapterID:    selection.adapterID,
		Endpoint:     selection.endpoint,
		Transport:    selection.transport,
		FallbackUsed: selection.fallbackUsed,
		Payload:      selection.payload,
		Latency:      selection.latency,
		Labels:       selection.labels,
	}
	s.observeInvocation(ctx, reg, selection, result, mutate, nil)
	if mutate {
		s.publishEvent(ctx, in, result)
	}
	return result, nil
}

// ReportHealth 记录健康状态。
func (s *Service) ReportHealth(ctx context.Context, in ReportHealthInput) error {
	if in.CapabilityID == "" || in.TenantUUID == "" || in.AdapterID == "" {
		return errors.New("capability router: invalid health input")
	}

	key := routingKey(in.TenantUUID, in.CapabilityID)
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	adapterMap := s.adapterState[key]
	if adapterMap == nil {
		adapterMap = make(map[string]healthState)
		s.adapterState[key] = adapterMap
	}
	status := strings.ToLower(in.Status)
	if status == "" {
		status = "healthy"
	}
	if status == "healthy" {
		delete(adapterMap, in.AdapterID)
	} else {
		adapterMap[in.AdapterID] = healthState{
			status:   status,
			reason:   in.Reason,
			failures: in.Failures,
			updated:  s.now(),
		}
	}

	if status == "unhealthy" {
		s.clearStickyAdapter(routingKey(in.TenantUUID, in.CapabilityID), in.AdapterID)
	}

	var repoErr error
	if s.healthRepo != nil {
		record := HealthProbeRecord{
			CapabilityID: in.CapabilityID,
			TenantUUID:   in.TenantUUID,
			AdapterID:    in.AdapterID,
			Status:       status,
			Reason:       in.Reason,
			Failures:     in.Failures,
			NextRetryAt:  s.now().Add(60 * time.Second),
			UpdatedAt:    s.now(),
		}
		repoErr = s.healthRepo.SaveProbeResult(ctx, nil, record)
		if repoErr != nil {
			s.metrics.ObserveHealthReport(ctx, in.CapabilityID, in.TenantUUID, in.AdapterID, status, repoErr)
			return repoErr
		}
	}
	s.metrics.ObserveHealthReport(ctx, in.CapabilityID, in.TenantUUID, in.AdapterID, status, repoErr)
	return nil
}

// --- 内部辅助 ----------------------------------------------------------------

type healthState struct {
	status   string
	reason   string
	failures uint32
	updated  time.Time
}

type adapterSelection struct {
	adapterID    string
	endpoint     string
	transport    string
	fallbackUsed bool
	payload      []byte
	latency      time.Duration
	labels       map[string]string
}

func (s *Service) selectAdapter(ctx context.Context, reg registry.Registration, in InvokeRequest) (adapterSelection, error) {
	key := routingKey(in.TenantUUID, in.CapabilityID)
	adapter := adapterSelection{}
	adapters := orderedAdapters(reg)

	preferredTransport := normalizeTransport(in.PreferredProtocol)
	if preferredTransport != "" {
		var prioritized []registry.AdapterEndpoint
		var rest []registry.AdapterEndpoint
		for _, ep := range adapters {
			if normalizeTransport(ep.TransportType) == preferredTransport {
				prioritized = append(prioritized, ep)
			} else {
				rest = append(rest, ep)
			}
		}
		if len(prioritized) > 0 {
			adapters = append(prioritized, rest...)
		}
	}

	if in.StickyKey != "" {
		stickyID := s.currentSticky(in, key)
		if stickyID != "" {
			if ep, ok := findAdapter(reg.Adapters, stickyID); ok && s.isAdapterHealthy(key, stickyID) {
				adapter.adapterID = ep.AdapterID
				adapter.endpoint = preferredEndpoint(ep)
				adapter.transport = ep.TransportType
				adapter.labels = ep.Labels
				return adapter, nil
			}
		}
	}

	for _, ep := range adapters {
		if !s.isAdapterHealthy(key, ep.AdapterID) {
			continue
		}
		adapter.adapterID = ep.AdapterID
		adapter.endpoint = preferredEndpoint(ep)
		adapter.transport = ep.TransportType
		adapter.labels = ep.Labels
		return adapter, nil
	}

	if reg.FallbackPlan != nil && reg.FallbackPlan.StaticResponse != nil {
		payload, _ := json.Marshal(reg.FallbackPlan.StaticResponse.Payload)
		adapter.fallbackUsed = true
		adapter.payload = payload
		return adapter, ErrNoAdapter
	}

	return adapterSelection{}, ErrNoAdapter
}

func (s *Service) currentSticky(in InvokeRequest, key string) string {
	stickyID := ""
	if in.StickyKey == "" {
		return ""
	}
	mapKey := stickyKeyFor(in.TenantUUID, in.CapabilityID, in.StickyKey)
	s.stickyMu.RLock()
	stickyID = s.stickyMap[mapKey]
	s.stickyMu.RUnlock()
	return stickyID
}

func (s *Service) isAdapterHealthy(routeKey, adapterID string) bool {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	if adapters, ok := s.adapterState[routeKey]; ok {
		if state, ok := adapters[adapterID]; ok {
			return state.status != "unhealthy"
		}
	}
	return true
}

func (s *Service) publishEvent(ctx context.Context, in InvokeRequest, result InvokeResult) {
	if s.eventBus == nil {
		return
	}
	payload := map[string]any{
		"capability_id": in.CapabilityID,
		"tenant_uuid":   in.TenantUUID,
		"adapter_id":    result.AdapterID,
		"fallback_used": result.FallbackUsed,
		"latency_ms":    result.Latency.Milliseconds(),
	}
	s.eventBus.Publish(EventRouterInvoked, payload, ctx)
	if result.FallbackUsed {
		s.eventBus.Publish(EventRouterFallback, payload, ctx)
	}
}

func (s *Service) observeInvocation(ctx context.Context, reg registry.Registration, selection adapterSelection, result InvokeResult, mutate bool, observeErr error) {
	if s.metrics == nil {
		return
	}
	mode := "invoke"
	if !mutate {
		mode = "sandbox"
	}
	tenantUUID := tenantUUIDValue(reg)
	s.metrics.ObserveInvocation(ctx, mode, reg.CapabilityID, tenantUUID, result.AdapterID, result.Transport, result.Latency, result.FallbackUsed, observeErr)
	if result.FallbackUsed || (observeErr != nil && errors.Is(observeErr, ErrNoAdapter)) {
		reason := fallbackReasonFromSelection(selection)
		s.metrics.ObserveFallback(ctx, reg.CapabilityID, tenantUUID, reason)
	}
}

func routingKey(tenantUUID, capabilityID string) string {
	return tenantUUID + "::" + capabilityID
}

func stickyKeyFor(tenantUUID, capabilityID, sticky string) string {
	return tenantUUID + "::" + capabilityID + "::" + sticky
}

func tenantUUIDValue(reg registry.Registration) string {
	return strings.TrimSpace(reg.TenantUUID)
}

func preferredEndpoint(ep registry.AdapterEndpoint) string {
	if ep.ServiceRef != "" {
		return ep.ServiceRef
	}
	return ep.Endpoint
}

func normalizeTransport(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func orderedAdapters(reg registry.Registration) []registry.AdapterEndpoint {
	if len(reg.Adapters) == 0 {
		return nil
	}
	result := make([]registry.AdapterEndpoint, 0, len(reg.Adapters))
	used := make(map[string]bool, len(reg.Adapters))
	for _, id := range reg.RoutingPolicy.FallbackSequence {
		if ep, ok := findAdapter(reg.Adapters, id); ok {
			result = append(result, ep)
			used[id] = true
		}
	}
	remaining := make([]registry.AdapterEndpoint, 0, len(reg.Adapters)-len(result))
	for _, ep := range reg.Adapters {
		if used[ep.AdapterID] {
			continue
		}
		remaining = append(remaining, ep)
	}
	sort.SliceStable(remaining, func(i, j int) bool {
		if remaining[i].Weight == remaining[j].Weight {
			return remaining[i].AdapterID < remaining[j].AdapterID
		}
		return remaining[i].Weight > remaining[j].Weight
	})
	return append(result, remaining...)
}

func findAdapter(adapters []registry.AdapterEndpoint, id string) (registry.AdapterEndpoint, bool) {
	for _, ep := range adapters {
		if ep.AdapterID == id {
			return ep, true
		}
	}
	return registry.AdapterEndpoint{}, false
}
