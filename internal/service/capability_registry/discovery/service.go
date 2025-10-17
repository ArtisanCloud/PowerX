package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
)

// Service 提供能力快照的缓存、同步与跨区域复制能力。
type Service struct {
	registryRepo registry.Repository
	cache        CacheStore
	repo         Repository
	replicator   Replicator
	instrument   *domain.Instrumentation
	metrics      MetricsRecorder
	defaultTTL   time.Duration
	now          func() time.Time
}

// NewService 创建 Discovery 服务实例。
func NewService(opts ServiceOptions) *Service {
	if opts.RegistryRepository == nil {
		panic("discovery.Service requires RegistryRepository")
	}
	if opts.CacheStore == nil {
		panic("discovery.Service requires CacheStore")
	}
	inst := opts.Instrumentation
	if inst == nil {
		inst = domain.NewInstrumentation(nil)
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	ttl := opts.DefaultTTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	metrics := opts.Metrics
	if metrics == nil {
		metrics = noopMetricsRecorder{}
	}
	return &Service{
		registryRepo: opts.RegistryRepository,
		cache:        opts.CacheStore,
		repo:         opts.Repository,
		replicator:   opts.Replicator,
		instrument:   inst,
		metrics:      metrics,
		defaultTTL:   ttl,
		now:          clock,
	}
}

// Now 返回当前时钟时间（便于 handler 对齐 TTL）。
func (s *Service) Now() time.Time {
	return s.now()
}

// Sync 根据请求从 Registry 同步快照并写入缓存。
func (s *Service) Sync(ctx context.Context, req SyncRequest) ([]Snapshot, error) {
	if req.TenantID == "" {
		return nil, ErrInvalidRequest
	}
	clientID := normalizeClientID(req.ClientID)
	registrations, err := s.resolveRegistrations(ctx, req)
	if err != nil {
		return nil, err
	}
	now := s.now()
	snapshots := make([]Snapshot, 0, len(registrations))
	for _, reg := range registrations {
		snapshot := s.buildSnapshot(reg, clientID, now, SnapshotSourceRegistry)
		if err := s.storeSnapshot(ctx, snapshot); err != nil {
			s.metrics.ObserveSync(ctx, reg.TenantID, reg.CapabilityID, snapshot.Source, snapshot.ExpiresAt.Sub(snapshot.IssuedAt), err)
			return nil, err
		}
		s.metrics.ObserveSync(ctx, reg.TenantID, reg.CapabilityID, snapshot.Source, snapshot.ExpiresAt.Sub(snapshot.IssuedAt), nil)
		snapshots = append(snapshots, snapshot)
	}
	if s.replicator != nil && len(snapshots) > 0 {
		if err := s.replicator.Replicate(ctx, snapshots); err != nil {
			return nil, err
		}
	}
	return snapshots, nil
}

// GetSnapshot 返回缓存中的快照；若过期则尝试刷新，失败时返回陈旧数据。
func (s *Service) GetSnapshot(ctx context.Context, tenantID, capabilityID, clientID string) (Snapshot, error) {
	if tenantID == "" || capabilityID == "" {
		return Snapshot{}, ErrInvalidRequest
	}
	key := CacheKey{
		TenantID:     tenantID,
		CapabilityID: capabilityID,
		ClientID:     normalizeClientID(clientID),
	}
	snapshot, ok, err := s.cache.Get(ctx, key)
	if err != nil {
		return Snapshot{}, err
	}
	now := s.now()
	if ok {
		if snapshot.Source != SnapshotSourceReplica {
			snapshot.Source = SnapshotSourceCache
		}
		snapshot.ClientID = key.ClientID
		snapshot.Stale = false
		if now.After(snapshot.ExpiresAt) {
			refreshed, refreshErr := s.refreshSnapshot(ctx, key)
			if refreshErr != nil {
				snapshot.Stale = true
				s.metrics.ObserveCacheLookup(ctx, tenantID, capabilityID, "stale")
				return snapshot, nil
			}
			s.metrics.ObserveCacheLookup(ctx, tenantID, capabilityID, "refresh")
			return refreshed, nil
		}
		s.metrics.ObserveCacheLookup(ctx, tenantID, capabilityID, "hit")
		return snapshot, nil
	}

	// 缓存未命中，尝试直接刷新
	refreshed, refreshErr := s.refreshSnapshot(ctx, key)
	if refreshErr == nil {
		s.metrics.ObserveCacheLookup(ctx, tenantID, capabilityID, "miss-refresh")
		return refreshed, nil
	}

	// 若持久化层存在历史快照则回退到陈旧数据
	if s.repo != nil {
		history, histErr := s.repo.ListSnapshots(ctx, tenantID, []string{capabilityID}, key.ClientID)
		if histErr == nil && len(history) > 0 {
			stale := history[0]
			stale.Source = SnapshotSourceCache
			stale.Stale = true
			_ = s.cache.Set(ctx, stale, stale.ExpiresAt.Sub(now))
			s.metrics.ObserveCacheLookup(ctx, tenantID, capabilityID, "stale-history")
			return stale, nil
		}
	}

	return Snapshot{}, ErrSnapshotNotFound
}

// ApplyReplica 将来自其他区域的快照写入当前缓存。
func (s *Service) ApplyReplica(ctx context.Context, snapshot Snapshot) error {
	now := s.now()
	if snapshot.IssuedAt.IsZero() {
		snapshot.IssuedAt = now
	}
	if snapshot.ExpiresAt.IsZero() || snapshot.ExpiresAt.Before(now) {
		snapshot.ExpiresAt = now.Add(s.defaultTTL)
	}
	snapshot.ClientID = normalizeClientID(snapshot.ClientID)
	snapshot.Source = SnapshotSourceReplica
	snapshot.Stale = false
	return s.storeSnapshot(ctx, snapshot)
}

func (s *Service) resolveRegistrations(ctx context.Context, req SyncRequest) ([]registry.Registration, error) {
	if len(req.Capabilities) == 0 {
		registrations, _, err := s.registryRepo.ListLatest(ctx, nil, req.TenantID, 0, 0)
		if err != nil {
			return nil, err
		}
		return registrations, nil
	}
	caps := deduplicate(req.Capabilities)
	result := make([]registry.Registration, 0, len(caps))
	for _, capabilityID := range caps {
		reg, err := s.registryRepo.GetLatest(ctx, nil, capabilityID, req.TenantID)
		if err != nil {
			return nil, err
		}
		result = append(result, reg)
	}
	return result, nil
}

func (s *Service) refreshSnapshot(ctx context.Context, key CacheKey) (Snapshot, error) {
	reg, err := s.registryRepo.GetLatest(ctx, nil, key.CapabilityID, key.TenantID)
	if err != nil {
		return Snapshot{}, err
	}
	now := s.now()
	snapshot := s.buildSnapshot(reg, key.ClientID, now, SnapshotSourceRegistry)
	if err := s.storeSnapshot(ctx, snapshot); err != nil {
		return Snapshot{}, err
	}
	s.metrics.ObserveSync(ctx, key.TenantID, key.CapabilityID, snapshot.Source, snapshot.ExpiresAt.Sub(snapshot.IssuedAt), nil)
	return snapshot, nil
}

func (s *Service) buildSnapshot(reg registry.Registration, clientID string, now time.Time, source SnapshotSource) Snapshot {
	if now.IsZero() {
		now = s.now()
	}
	ttl := s.defaultTTL
	expiresAt := now.Add(ttl)

	normalizedAdapters := make([]registry.AdapterEndpoint, len(reg.Adapters))
	copy(normalizedAdapters, reg.Adapters)
	sort.SliceStable(normalizedAdapters, func(i, j int) bool {
		return normalizedAdapters[i].AdapterID < normalizedAdapters[j].AdapterID
	})

	snapshot := Snapshot{
		CapabilityID:   reg.CapabilityID,
		TenantID:       reg.TenantID,
		Version:        reg.Version,
		IssuedAt:       now,
		ExpiresAt:      expiresAt,
		RoutingPolicy:  reg.RoutingPolicy,
		Adapters:       normalizedAdapters,
		FallbackPlan:   reg.FallbackPlan,
		MetadataDigest: computeMetadataDigest(reg, normalizedAdapters),
		PolicyDigest:   computePolicyDigest(reg.RoutingPolicy),
		Source:         source,
		ClientID:       normalizeClientID(clientID),
		Stale:          false,
	}
	return snapshot
}

func (s *Service) storeSnapshot(ctx context.Context, snapshot Snapshot) error {
	ttl := snapshot.ExpiresAt.Sub(s.now())
	if ttl <= 0 {
		ttl = time.Second
	}
	if err := s.cache.Set(ctx, snapshot, ttl); err != nil {
		return err
	}
	if s.repo != nil {
		if err := s.repo.SaveSnapshot(ctx, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func deduplicate(values []string) []string {
	set := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := set[value]; ok {
			continue
		}
		set[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func computeMetadataDigest(reg registry.Registration, adapters []registry.AdapterEndpoint) string {
	payload := map[string]interface{}{
		"capability_id": reg.CapabilityID,
		"tenant_id":     reg.TenantID,
		"version":       reg.Version,
		"adapters":      adapters,
		"routing":       reg.RoutingPolicy,
		"fallback":      reg.FallbackPlan,
	}
	return shaPayload(payload)
}

func computePolicyDigest(policy registry.RoutingPolicy) string {
	payload := map[string]interface{}{
		"strategy":          policy.Strategy,
		"fallback_sequence": policy.FallbackSequence,
		"cooldown_seconds":  policy.CooldownSeconds,
		"tenant_strategies": policy.TenantStrategies,
		"sticky_keys":       policy.StickyKeys,
		"rate_limit":        policy.RateLimit,
	}
	return shaPayload(payload)
}

func shaPayload(payload interface{}) string {
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func normalizeClientID(clientID string) string {
	if clientID == "" {
		return "default"
	}
	return clientID
}
