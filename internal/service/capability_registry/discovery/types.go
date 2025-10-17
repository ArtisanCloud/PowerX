package discovery

import (
	"context"
	"errors"
	"time"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
)

// ErrSnapshotNotFound 表示缓存中不存在指定快照。
var ErrSnapshotNotFound = errors.New("capability discovery: snapshot not found")

// ErrInvalidRequest 表示同步或查询请求参数非法。
var ErrInvalidRequest = errors.New("capability discovery: invalid request")

// SnapshotSource 描述快照来源。
type SnapshotSource string

const (
	// SnapshotSourceRegistry 表示源自 Registry 的最新快照。
	SnapshotSourceRegistry SnapshotSource = "registry"
	// SnapshotSourceCache 表示来自本地缓存（可能为陈旧数据）。
	SnapshotSourceCache SnapshotSource = "cache"
	// SnapshotSourceReplica 表示跨区域复制得到的快照。
	SnapshotSourceReplica SnapshotSource = "replica"
)

// CacheKey 表示缓存键，按照租户/能力/客户端划分。
type CacheKey struct {
	TenantID     string
	CapabilityID string
	ClientID     string
}

// Snapshot 表示下发给客户端的能力快照。
type Snapshot struct {
	CapabilityID   string                     `json:"capability_id"`
	TenantID       string                     `json:"tenant_id"`
	Version        uint64                     `json:"version"`
	IssuedAt       time.Time                  `json:"issued_at"`
	ExpiresAt      time.Time                  `json:"expires_at"`
	RoutingPolicy  registry.RoutingPolicy     `json:"routing_policy"`
	Adapters       []registry.AdapterEndpoint `json:"adapters"`
	FallbackPlan   *registry.FallbackPlan     `json:"fallback_plan,omitempty"`
	MetadataDigest string                     `json:"metadata_digest"`
	Source         SnapshotSource             `json:"source"`
	ClientID       string                     `json:"client_id,omitempty"`
	PolicyDigest   string                     `json:"policy_digest,omitempty"`
	Stale          bool                       `json:"stale"`
}

// CacheKey 返回快照对应的缓存键。
func (s Snapshot) CacheKey() CacheKey {
	return CacheKey{
		TenantID:     s.TenantID,
		CapabilityID: s.CapabilityID,
		ClientID:     normalizeClientID(s.ClientID),
	}
}

// RemainingTTL 计算剩余 TTL。
func (s Snapshot) RemainingTTL(ref time.Time) time.Duration {
	if ref.IsZero() {
		ref = time.Now()
	}
	return s.ExpiresAt.Sub(ref)
}

// SyncRequest 描述批量同步的输入。
type SyncRequest struct {
	TenantID     string
	Capabilities []string
	ClientID     string
	Force        bool
}

// CacheStore 为 Discovery 缓存定义接口，默认基于 Redis，可在测试中注入内存实现。
type CacheStore interface {
	Get(ctx context.Context, key CacheKey) (Snapshot, bool, error)
	Set(ctx context.Context, snapshot Snapshot, ttl time.Duration) error
	Delete(ctx context.Context, key CacheKey) error
}

// Repository 为持久化元数据的接口，允许根据需要落地数据库。
type Repository interface {
	SaveSnapshot(ctx context.Context, snapshot Snapshot) error
	ListSnapshots(ctx context.Context, tenantID string, capabilityIDs []string, clientID string) ([]Snapshot, error)
}

// Replicator 负责将最新快照广播到其他区域。
type Replicator interface {
	Replicate(ctx context.Context, snapshots []Snapshot) error
}

// MetricsRecorder 定义观测指标接口。
type MetricsRecorder interface {
	ObserveSync(ctx context.Context, tenantID, capabilityID string, source SnapshotSource, ttl time.Duration, err error)
	ObserveCacheLookup(ctx context.Context, tenantID, capabilityID string, outcome string)
}

type noopMetricsRecorder struct{}

func (noopMetricsRecorder) ObserveSync(context.Context, string, string, SnapshotSource, time.Duration, error) {
}

func (noopMetricsRecorder) ObserveCacheLookup(context.Context, string, string, string) {
}

// ServiceOptions 注入 Discovery 服务依赖。
type ServiceOptions struct {
	RegistryRepository registry.Repository
	CacheStore         CacheStore
	Repository         Repository
	Replicator         Replicator
	Instrumentation    *domain.Instrumentation
	Metrics            MetricsRecorder
	DefaultTTL         time.Duration
	Clock              func() time.Time
}
