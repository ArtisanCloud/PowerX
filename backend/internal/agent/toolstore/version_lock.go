package toolstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/redis/go-redis/v9"
)

const defaultVersionLockPrefix = "toolstore:version_lock"

// ErrVersionUpgradeRequired 表示当前租户绑定的 capabilities_hash 尚未确认升级。
var ErrVersionUpgradeRequired = errors.New("capability version requires manual upgrade")

// VersionLockStoreOptions 描述版本锁依赖注入。
type VersionLockStoreOptions struct {
	Redis    redis.UniversalClient
	Prefix   string
	EventBus event_bus.EventBus
	Clock    func() time.Time
}

// VersionLockStore 管理租户对 Capability Hash 的确认状态。
type VersionLockStore struct {
	redis    redis.UniversalClient
	prefix   string
	eventBus event_bus.EventBus
	now      func() time.Time

	mu        sync.RWMutex
	bindings  map[string]string
	degraded  map[string]string
	degradedM sync.Mutex
}

// NewVersionLockStore 构建版本锁执行器。
func NewVersionLockStore(opts VersionLockStoreOptions) *VersionLockStore {
	prefix := strings.TrimSpace(opts.Prefix)
	if prefix == "" {
		prefix = defaultVersionLockPrefix
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	var redisClient redis.UniversalClient
	if !isNilUniversalClient(opts.Redis) {
		redisClient = opts.Redis
	}

	return &VersionLockStore{
		redis:    redisClient,
		prefix:   prefix,
		eventBus: opts.EventBus,
		now:      clock,
		bindings: make(map[string]string),
		degraded: make(map[string]string),
	}
}

// Bind 记录租户当前确认的 capabilities_hash。
func (s *VersionLockStore) Bind(ctx context.Context, tenantUUID, capabilityID, hash string) error {
	if err := s.validateArgs(tenantUUID, capabilityID, hash); err != nil {
		return err
	}
	return s.setBinding(ctx, tenantUUID, capabilityID, hash)
}

// Enforce 检查调用的 hash 是否获得授权，否则返回 ErrVersionUpgradeRequired。
func (s *VersionLockStore) Enforce(ctx context.Context, tenantUUID, capabilityID, hash string) error {
	if err := s.validateArgs(tenantUUID, capabilityID, hash); err != nil {
		return err
	}
	if s == nil {
		return errors.New("version lock store is nil")
	}
	current, ok := s.getBinding(ctx, tenantUUID, capabilityID)
	if !ok {
		return s.Bind(ctx, tenantUUID, capabilityID, hash)
	}
	if strings.EqualFold(current, hash) {
		return nil
	}
	s.publishDegraded(ctx, tenantUUID, capabilityID, current, hash)
	return fmt.Errorf("%w: expected %s, got %s", ErrVersionUpgradeRequired, current, hash)
}

// ConfirmUpgrade 手动确认升级到新的 hash。
func (s *VersionLockStore) ConfirmUpgrade(ctx context.Context, tenantUUID, capabilityID, hash string) error {
	if err := s.validateArgs(tenantUUID, capabilityID, hash); err != nil {
		return err
	}
	return s.setBinding(ctx, tenantUUID, capabilityID, hash)
}

// CurrentHash 返回当前绑定的 hash。
func (s *VersionLockStore) CurrentHash(ctx context.Context, tenantUUID, capabilityID string) (string, bool) {
	if s == nil {
		return "", false
	}
	return s.getBinding(ctx, tenantUUID, capabilityID)
}

// IsUpgradeError 判断错误是否由于版本锁约束导致。
func (s *VersionLockStore) IsUpgradeError(err error) bool {
	return errors.Is(err, ErrVersionUpgradeRequired)
}

func (s *VersionLockStore) validateArgs(tenantUUID, capabilityID, hash string) error {
	if s == nil {
		return errors.New("version lock store is nil")
	}
	tenantUUID = strings.TrimSpace(tenantUUID)
	capabilityID = strings.TrimSpace(capabilityID)
	hash = strings.TrimSpace(hash)
	switch {
	case tenantUUID == "":
		return errors.New("tenant uuid is required")
	case capabilityID == "":
		return errors.New("capability id is required")
	case hash == "":
		return errors.New("capabilities hash is required")
	}
	return nil
}

func (s *VersionLockStore) keyFor(tenantUUID, capabilityID string) string {
	return fmt.Sprintf("%s:%s:%s", s.prefix, strings.TrimSpace(tenantUUID), strings.TrimSpace(capabilityID))
}

func (s *VersionLockStore) setBinding(ctx context.Context, tenantUUID, capabilityID, hash string) error {
	key := s.keyFor(tenantUUID, capabilityID)
	if s.redis != nil {
		if err := s.redis.Set(ctx, key, hash, 0).Err(); err != nil {
			return err
		}
	} else {
		s.mu.Lock()
		s.bindings[key] = hash
		s.mu.Unlock()
	}

	s.degradedM.Lock()
	delete(s.degraded, key)
	s.degradedM.Unlock()
	return nil
}

func (s *VersionLockStore) getBinding(ctx context.Context, tenantUUID, capabilityID string) (string, bool) {
	key := s.keyFor(tenantUUID, capabilityID)
	if s.redis != nil {
		val, err := s.redis.Get(ctx, key).Result()
		if err == redis.Nil {
			return "", false
		}
		if err != nil {
			return "", false
		}
		return val, true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.bindings[key]
	return val, ok
}

func (s *VersionLockStore) publishDegraded(ctx context.Context, tenantUUID, capabilityID, expected, incoming string) {
	if s.eventBus == nil {
		return
	}
	key := s.keyFor(tenantUUID, capabilityID)
	if !s.shouldPublishDegraded(key, incoming) {
		return
	}
	payload := map[string]interface{}{
		"tenant_uuid":        tenantUUID,
		"capability_id":      capabilityID,
		"expected_hash":      expected,
		"incoming_hash":      incoming,
		"detected_at":        s.now().UTC().Format(time.RFC3339),
		"requires_approval":  true,
		"policy_event_topic": eventbus.TopicCapabilityPolicyDegraded,
	}
	s.eventBus.Publish(eventbus.TopicCapabilityPolicyDegraded, payload, ctx)
}

func (s *VersionLockStore) shouldPublishDegraded(key, incoming string) bool {
	s.degradedM.Lock()
	defer s.degradedM.Unlock()
	if prev, ok := s.degraded[key]; ok && prev == incoming {
		return false
	}
	s.degraded[key] = incoming
	return true
}
