package capability_registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/redis/go-redis/v9"
)

const (
	defaultCapabilityCachePrefix = "capability_registry:cache"
	defaultPolicyCachePrefix     = "toolstore:policy"
	defaultBroadcastChannel      = "capability_registry:broadcast"

	defaultCapabilityCacheTTL = 3 * time.Minute
	defaultPolicyCacheTTL     = 5 * time.Minute
)

// CacheManagerOptions 自定义缓存前缀、TTL 与广播通道。
type CacheManagerOptions struct {
	Redis            RedisClient
	CapabilityPrefix string
	CapabilityTTL    time.Duration
	PolicyPrefix     string
	PolicyTTL        time.Duration
	BroadcastChannel string
}

// CacheManager 负责 CapabilityRegistry 相关缓存与广播。
type CacheManager struct {
	redis   redis.UniversalClient
	capTTL  time.Duration
	polTTL  time.Duration
	capPref string
	polPref string
	channel string
}

// CacheBroadcastMessage 定义缓存刷新广播消息。
type CacheBroadcastMessage struct {
	Event            string    `json:"event"`
	CapabilityID     string    `json:"capability_id,omitempty"`
	PluginID         string    `json:"plugin_id,omitempty"`
	CapabilitiesHash string    `json:"capabilities_hash,omitempty"`
	ProtocolHash     string    `json:"protocol_hash,omitempty"`
	PolicyHash       string    `json:"policy_hash,omitempty"`
	TenantUUID       string    `json:"tenant_uuid,omitempty"`
	Timestamp        time.Time `json:"timestamp"`
}

// NewCacheManager 构造缓存管理器。若 Redis 为空则返回 nil（表示禁用）。
func NewCacheManager(opts CacheManagerOptions) *CacheManager {
	if opts.Redis == nil {
		return nil
	}
	capTTL := opts.CapabilityTTL
	if capTTL <= 0 {
		capTTL = defaultCapabilityCacheTTL
	}
	polTTL := opts.PolicyTTL
	if polTTL <= 0 {
		polTTL = defaultPolicyCacheTTL
	}
	prefix := strings.TrimSpace(opts.CapabilityPrefix)
	if prefix == "" {
		prefix = defaultCapabilityCachePrefix
	}
	policyPrefix := strings.TrimSpace(opts.PolicyPrefix)
	if policyPrefix == "" {
		policyPrefix = defaultPolicyCachePrefix
	}
	channel := strings.TrimSpace(opts.BroadcastChannel)
	if channel == "" {
		channel = defaultBroadcastChannel
	}
	return &CacheManager{
		redis:   opts.Redis,
		capTTL:  capTTL,
		polTTL:  polTTL,
		capPref: prefix,
		polPref: policyPrefix,
		channel: channel,
	}
}

// CapabilityCacheKey 返回能力缓存键。
func (m *CacheManager) CapabilityCacheKey(capabilityID string) string {
	trimmed := strings.TrimSpace(capabilityID)
	if trimmed == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", m.capPref, trimmed)
}

// PolicyCacheKey 返回 ToolStore 策略缓存键。
func (m *CacheManager) PolicyCacheKey(hash string) string {
	trimmed := strings.TrimSpace(hash)
	if trimmed == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", m.polPref, trimmed)
}

// CacheCapabilityRecord 写入能力缓存。
func (m *CacheManager) CacheCapabilityRecord(ctx context.Context, record *models.CapabilityRecord) error {
	if m == nil || m.redis == nil || record == nil {
		return nil
	}
	key := m.CapabilityCacheKey(record.CapabilityID)
	if key == "" {
		return fmt.Errorf("capability id missing")
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return m.redis.Set(ctx, key, payload, m.capTTL).Err()
}

// CachePolicySnapshot 缓存 ToolStore Selector 策略。
func (m *CacheManager) CachePolicySnapshot(ctx context.Context, hash string, snapshot interface{}) error {
	if m == nil || m.redis == nil {
		return nil
	}
	key := m.PolicyCacheKey(hash)
	if key == "" {
		return fmt.Errorf("policy hash missing")
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return m.redis.Set(ctx, key, payload, m.polTTL).Err()
}

// Broadcast 发出缓存刷新广播，供 ToolStore/Agent Hub 监听。
func (m *CacheManager) Broadcast(ctx context.Context, msg CacheBroadcastMessage) error {
	if m == nil || m.redis == nil {
		return nil
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return m.redis.Publish(ctx, m.channel, payload).Err()
}
