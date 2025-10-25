package discoverycache

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	discovery "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
)

const defaultPrefix = "capability:discovery"

// Store 基于 cache.ICache（默认 Redis）提供 Discovery 快照缓存读写。
type Store struct {
	cache  cache.ICache
	prefix string
}

// NewStore 创建缓存封装。
func NewStore(c cache.ICache, prefix string) *Store {
	if c == nil {
		panic("discovery cache store requires cache backend")
	}
	if prefix == "" {
		prefix = defaultPrefix
	}
	return &Store{
		cache:  c,
		prefix: prefix,
	}
}

// Get 读取缓存快照。
func (s *Store) Get(ctx context.Context, key discovery.CacheKey) (discovery.Snapshot, bool, error) {
	raw, err := s.cache.Get(ctx, s.redisKey(key))
	if err != nil {
		return discovery.Snapshot{}, false, err
	}
	if len(raw) == 0 {
		return discovery.Snapshot{}, false, nil
	}
	var record snapshotRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return discovery.Snapshot{}, false, err
	}
	return record.toSnapshot(), true, nil
}

// Set 写入缓存快照。
func (s *Store) Set(ctx context.Context, snapshot discovery.Snapshot, ttl time.Duration) error {
	record := fromSnapshot(snapshot)
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		ttl = time.Second
	}
	return s.cache.Set(ctx, s.redisKey(snapshot.CacheKey()), payload, ttl)
}

// Delete 移除缓存。
func (s *Store) Delete(ctx context.Context, key discovery.CacheKey) error {
	return s.cache.Delete(ctx, s.redisKey(key))
}

func (s *Store) redisKey(key discovery.CacheKey) string {
	parts := []string{s.prefix, key.TenantID, key.CapabilityID, key.ClientID}
	return strings.Join(parts, ":")
}

type snapshotRecord struct {
	CapabilityID   string                     `json:"capability_id"`
	TenantID       string                     `json:"tenant_id"`
	Version        uint64                     `json:"version"`
	IssuedAt       time.Time                  `json:"issued_at"`
	ExpiresAt      time.Time                  `json:"expires_at"`
	RoutingPolicy  registry.RoutingPolicy     `json:"routing_policy"`
	Adapters       []registry.AdapterEndpoint `json:"adapters"`
	FallbackPlan   *registry.FallbackPlan     `json:"fallback_plan,omitempty"`
	MetadataDigest string                     `json:"metadata_digest"`
	PolicyDigest   string                     `json:"policy_digest"`
	Source         discovery.SnapshotSource   `json:"source"`
	ClientID       string                     `json:"client_id"`
	Stale          bool                       `json:"stale"`
}

func fromSnapshot(snapshot discovery.Snapshot) snapshotRecord {
	return snapshotRecord{
		CapabilityID:   snapshot.CapabilityID,
		TenantID:       snapshot.TenantID,
		Version:        snapshot.Version,
		IssuedAt:       snapshot.IssuedAt,
		ExpiresAt:      snapshot.ExpiresAt,
		RoutingPolicy:  snapshot.RoutingPolicy,
		Adapters:       snapshot.Adapters,
		FallbackPlan:   snapshot.FallbackPlan,
		MetadataDigest: snapshot.MetadataDigest,
		PolicyDigest:   snapshot.PolicyDigest,
		Source:         snapshot.Source,
		ClientID:       snapshot.ClientID,
		Stale:          snapshot.Stale,
	}
}

func (r snapshotRecord) toSnapshot() discovery.Snapshot {
	return discovery.Snapshot{
		CapabilityID:   r.CapabilityID,
		TenantID:       r.TenantID,
		Version:        r.Version,
		IssuedAt:       r.IssuedAt,
		ExpiresAt:      r.ExpiresAt,
		RoutingPolicy:  r.RoutingPolicy,
		Adapters:       r.Adapters,
		FallbackPlan:   r.FallbackPlan,
		MetadataDigest: r.MetadataDigest,
		PolicyDigest:   r.PolicyDigest,
		Source:         r.Source,
		ClientID:       r.ClientID,
		Stale:          r.Stale,
	}
}
