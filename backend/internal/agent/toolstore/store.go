package toolstore

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/eventbus"
	capregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// PolicySnapshotSource 抽象策略生成器接口，便于注入与测试。
type PolicySnapshotSource interface {
	GenerateSnapshot(ctx context.Context, in capregistry.GeneratePolicyInput) (capregistry.SelectorPolicySnapshot, error)
}

// StoreOptions 描述 ToolStore 构建所需依赖。
type StoreOptions struct {
	Generator PolicySnapshotSource
	EventBus  event_bus.EventBus
	CacheTTL  time.Duration
	Logger    *pxlog.Logger
	Clock     func() time.Time
}

// Store 维护 SelectorPolicySnapshot 的内存缓存，并在事件触发时刷新。
type Store struct {
	generator PolicySnapshotSource
	cacheTTL  time.Duration
	now       func() time.Time
	logger    *pxlog.Logger

	mu    sync.RWMutex
	cache map[string]cachedSnapshot

	unsubscribes []func()
}

type cachedSnapshot struct {
	value     SelectorPolicySnapshot
	expiresAt time.Time
}

var (
	storeMu     sync.RWMutex
	globalStore *Store

	errGeneratorMissing = errors.New("toolstore: policy snapshot generator is required")
)

// NewStore 构建 ToolStore 并订阅能力同步事件。
func NewStore(opts StoreOptions) *Store {
	if opts.Generator == nil {
		panic(errGeneratorMissing)
	}
	ttl := opts.CacheTTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	store := &Store{
		generator:    opts.Generator,
		cacheTTL:     ttl,
		now:          clock,
		logger:       logger,
		cache:        make(map[string]cachedSnapshot),
		unsubscribes: make([]func(), 0),
	}
	store.subscribe(opts.EventBus)
	return store
}

// Close 释放订阅资源。
func (s *Store) Close() {
	for _, unsub := range s.unsubscribes {
		if unsub != nil {
			unsub()
		}
	}
	s.unsubscribes = nil
}

// SetGlobalStore 注册全局 ToolStore 实例。
func SetGlobalStore(store *Store) {
	storeMu.Lock()
	defer storeMu.Unlock()
	globalStore = store
}

// GetGlobalStore 返回全局 ToolStore 实例。
func GetGlobalStore() *Store {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return globalStore
}

// GetSnapshot 返回指定租户的 SelectorPolicySnapshot。
func (s *Store) GetSnapshot(ctx context.Context, tenantUUID string, toolGrants []string) (SelectorPolicySnapshot, error) {
	if s == nil {
		return SelectorPolicySnapshot{}, errors.New("toolstore: store is nil")
	}
	tenant := strings.TrimSpace(tenantUUID)
	if tenant == "" {
		return SelectorPolicySnapshot{}, errors.New("tenant uuid is required")
	}
	normalized, key := normalizeGrants(tenant, toolGrants)
	if snapshot, ok := s.loadFromCache(key); ok {
		return snapshot, nil
	}
	return s.refreshSnapshot(ctx, tenant, normalized, key)
}

func (s *Store) loadFromCache(key string) (SelectorPolicySnapshot, bool) {
	s.mu.RLock()
	entry, ok := s.cache[key]
	s.mu.RUnlock()
	if !ok {
		return SelectorPolicySnapshot{}, false
	}
	if s.now().After(entry.expiresAt) {
		return SelectorPolicySnapshot{}, false
	}
	return cloneSnapshot(entry.value), true
}

func (s *Store) refreshSnapshot(ctx context.Context, tenant string, grants []string, key string) (SelectorPolicySnapshot, error) {
	src, err := s.generator.GenerateSnapshot(ctx, capregistry.GeneratePolicyInput{
		TenantUUID:   tenant,
		ToolGrantIDs: grants,
	})
	if err != nil {
		return SelectorPolicySnapshot{}, err
	}
	snapshot := convertSnapshot(src)
	entry := cachedSnapshot{
		value:     snapshot,
		expiresAt: s.now().Add(s.cacheTTL),
	}
	s.mu.Lock()
	s.cache[key] = entry
	s.mu.Unlock()
	return snapshot, nil
}

func (s *Store) subscribe(bus event_bus.EventBus) {
	if bus == nil {
		return
	}
	handler := func(evt event_bus.Event) error {
		s.invalidateAll(evt)
		return nil
	}
	for _, topic := range []string{
		eventbus.TopicCapabilityCatalogSyncStarted,
		eventbus.TopicCapabilityCatalogSyncSucceeded,
		eventbus.TopicCapabilityCatalogSyncFailed,
	} {
		unsub := bus.Subscribe(topic, handler)
		s.unsubscribes = append(s.unsubscribes, unsub)
	}
}

func (s *Store) invalidateAll(evt event_bus.Event) {
	s.mu.Lock()
	s.cache = make(map[string]cachedSnapshot)
	s.mu.Unlock()
	if s.logger == nil {
		return
	}
	capabilityID := ""
	if payload, ok := evt.Payload.(capregistry.CatalogSyncEvent); ok {
		capabilityID = payload.CapabilityID
	}
	s.logger.InfoF(evt.Ctx, "[toolstore] capability cache invalidated topic=%s capability=%s", evt.Name, capabilityID)
}

func convertSnapshot(src capregistry.SelectorPolicySnapshot) SelectorPolicySnapshot {
	out := SelectorPolicySnapshot{
		TenantID:           src.TenantID,
		CapabilitiesHash:   src.CapabilitiesHash,
		IntentMappings:     make(IntentMapping),
		PreferMatrix:       make(map[string]ProtocolPreference),
		RateLimitOverrides: make(map[string]RateLimitOverride),
		GeneratedAt:        src.GeneratedAt,
		Metadata:           copyStringMap(src.Metadata),
	}
	for intent, scopes := range src.IntentMappings {
		out.IntentMappings[intent] = make(map[string]string, len(scopes))
		for scope, capability := range scopes {
			out.IntentMappings[intent][scope] = capability
		}
	}
	for capID, pref := range src.PreferMatrix {
		out.PreferMatrix[capID] = ProtocolPreference{
			Prefer:               pref.Prefer,
			Fallback:             append([]string(nil), pref.Fallback...),
			RollbackCapabilityID: pref.RollbackCapabilityID,
		}
	}
	for key, override := range src.RateLimitOverrides {
		out.RateLimitOverrides[key] = RateLimitOverride{
			Limit:         override.Limit,
			Burst:         override.Burst,
			WindowSeconds: override.WindowSeconds,
			Scope:         override.Scope,
		}
	}
	return out
}

func cloneSnapshot(src SelectorPolicySnapshot) SelectorPolicySnapshot {
	return convertSnapshot(capregistry.SelectorPolicySnapshot{
		TenantID:           src.TenantID,
		CapabilitiesHash:   src.CapabilitiesHash,
		IntentMappings:     toGenericIntentMap(src.IntentMappings),
		PreferMatrix:       toGenericPreferMatrix(src.PreferMatrix),
		RateLimitOverrides: toGenericOverrides(src.RateLimitOverrides),
		GeneratedAt:        src.GeneratedAt,
		Metadata:           copyStringMap(src.Metadata),
	})
}

func toGenericIntentMap(src IntentMapping) map[string]map[string]string {
	out := make(map[string]map[string]string, len(src))
	for intent, scopes := range src {
		out[intent] = make(map[string]string, len(scopes))
		for scope, capability := range scopes {
			out[intent][scope] = capability
		}
	}
	return out
}

func toGenericPreferMatrix(src map[string]ProtocolPreference) map[string]capregistry.ProtocolPreference {
	out := make(map[string]capregistry.ProtocolPreference, len(src))
	for capID, pref := range src {
		out[capID] = capregistry.ProtocolPreference{
			Prefer:               pref.Prefer,
			Fallback:             append([]string(nil), pref.Fallback...),
			RollbackCapabilityID: pref.RollbackCapabilityID,
		}
	}
	return out
}

func toGenericOverrides(src map[string]RateLimitOverride) map[string]capregistry.RateLimitOverride {
	out := make(map[string]capregistry.RateLimitOverride, len(src))
	for key, override := range src {
		out[key] = capregistry.RateLimitOverride{
			Limit:         override.Limit,
			Burst:         override.Burst,
			WindowSeconds: override.WindowSeconds,
			Scope:         override.Scope,
		}
	}
	return out
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func normalizeGrants(tenant string, grants []string) ([]string, string) {
	if len(grants) == 0 {
		return nil, tenantKey(tenant, "")
	}
	unique := make(map[string]string, len(grants))
	for _, grant := range grants {
		trimmed := strings.TrimSpace(grant)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := unique[key]; !ok {
			unique[key] = trimmed
		}
	}
	if len(unique) == 0 {
		return nil, tenantKey(tenant, "")
	}
	clean := make([]string, 0, len(unique))
	for _, value := range unique {
		clean = append(clean, value)
	}
	sort.Strings(clean)
	return clean, tenantKey(tenant, strings.Join(clean, ","))
}

func tenantKey(tenant string, grants string) string {
	if grants == "" {
		grants = "-"
	}
	return fmt.Sprintf("%s|%s", tenant, grants)
}
