package discovery

import (
	"context"
	"math"
	"sync/atomic"
	"time"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
)

// MetricsOption 允许自定义指标阈值。
type MetricsOption func(*ObservabilityMetrics)

// WithHitRateThreshold 设置命中率告警阈值。
func WithHitRateThreshold(threshold float64) MetricsOption {
	return func(m *ObservabilityMetrics) {
		if threshold > 0 && threshold <= 1 {
			m.alertThreshold = threshold
		}
	}
}

// WithMinSamples 设置触发告警前最小采样数。
func WithMinSamples(samples uint64) MetricsOption {
	return func(m *ObservabilityMetrics) {
		if samples > 0 {
			m.minSamples = samples
		}
	}
}

// WithTTLEstimate 设置期望 TTL，用于检测异常 TTL。
func WithTTLEstimate(ttl time.Duration) MetricsOption {
	return func(m *ObservabilityMetrics) {
		if ttl > 0 {
			m.ttlExpectation = ttl
		}
	}
}

const (
	defaultHitRateThreshold = 0.80
	defaultMinSamples       = 25
)

// ObservabilityMetrics 基于内存的指标记录器，实现 MetricsRecorder。
type ObservabilityMetrics struct {
	inst *domain.Instrumentation

	cacheHits    atomic.Uint64
	cacheMisses  atomic.Uint64
	syncSuccess  atomic.Uint64
	syncFailures atomic.Uint64

	alertThreshold float64
	minSamples     uint64
	lastAlertTotal atomic.Uint64
	ttlExpectation time.Duration
}

// NewObservabilityMetrics 创建指标记录器。
func NewObservabilityMetrics(inst *domain.Instrumentation, opts ...MetricsOption) *ObservabilityMetrics {
	if inst == nil {
		inst = domain.NewInstrumentation(nil)
	}
	m := &ObservabilityMetrics{
		inst:           inst,
		alertThreshold: defaultHitRateThreshold,
		minSamples:     defaultMinSamples,
		ttlExpectation: 2 * time.Minute,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// ObserveSync 记录同步指标。
func (m *ObservabilityMetrics) ObserveSync(ctx context.Context, tenantUUID, capabilityID string, source SnapshotSource, ttl time.Duration, err error) {
	if err != nil {
		m.syncFailures.Add(1)
		m.inst.Logger(ctx).WarnF(ctx, "[discovery.metrics] sync failed: tenant=%s capability=%s source=%s err=%v", tenantUUID, capabilityID, source, err)
		return
	}
	m.syncSuccess.Add(1)

	// TTL 异常时打印提示，便于运维识别配置问题。
	if ttl > 0 && ttl < (m.ttlExpectation/2) {
		m.inst.Logger(ctx).WarnF(ctx, "[discovery.metrics] ttl below expectation: tenant=%s capability=%s ttl=%s", tenantUUID, capabilityID, ttl)
	}
}

// ObserveCacheLookup 记录缓存命中率。
func (m *ObservabilityMetrics) ObserveCacheLookup(ctx context.Context, tenantUUID, capabilityID, outcome string) {
	switch outcome {
	case "hit":
		m.cacheHits.Add(1)
	case "refresh":
		// 发生刷新代表发生一次失效后补偿，视作 miss。
		m.cacheMisses.Add(1)
	default:
		m.cacheMisses.Add(1)
	}

	m.evaluateHitRate(ctx, tenantUUID, capabilityID)
}

// Snapshot 返回当前指标快照。
func (m *ObservabilityMetrics) Snapshot() MetricsSnapshot {
	hits := m.cacheHits.Load()
	misses := m.cacheMisses.Load()
	success := m.syncSuccess.Load()
	failures := m.syncFailures.Load()
	total := hits + misses

	var rate float64
	if total > 0 {
		rate = float64(hits) / float64(total)
	}

	return MetricsSnapshot{
		CacheHits:       hits,
		CacheMisses:     misses,
		HitRate:         rate,
		SyncSuccess:     success,
		SyncFailures:    failures,
		SamplesObserved: total,
	}
}

// MetricsSnapshot 描述关键指标数据。
type MetricsSnapshot struct {
	CacheHits       uint64
	CacheMisses     uint64
	HitRate         float64
	SyncSuccess     uint64
	SyncFailures    uint64
	SamplesObserved uint64
}

func (m *ObservabilityMetrics) evaluateHitRate(ctx context.Context, tenantUUID, capabilityID string) {
	hits := m.cacheHits.Load()
	misses := m.cacheMisses.Load()
	total := hits + misses

	if total < m.minSamples {
		return
	}

	rate := float64(hits) / float64(total)
	if rate >= m.alertThreshold {
		return
	}

	prev := m.lastAlertTotal.Load()
	if total-prev < m.minSamples/2 {
		// 避免短时间内重复告警。
		return
	}
	if m.lastAlertTotal.CompareAndSwap(prev, total) {
		percentage := math.Round(rate*1000) / 10 // 小数点后1位
		m.inst.Logger(ctx).WarnF(ctx,
			"[discovery.metrics] cache hit rate below threshold: tenant=%s capability=%s hit_rate=%.1f%% threshold=%.0f%% samples=%d",
			tenantUUID,
			capabilityID,
			percentage,
			m.alertThreshold*100,
			total,
		)
	}
}
