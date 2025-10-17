package health

import (
	"context"
	"sync"

	"gorm.io/gorm"

	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
)

// MemoryRepository 提供简单的内存健康状态存储，便于单机与测试环境使用。
type MemoryRepository struct {
	mu      sync.RWMutex
	records map[string]map[string]router.HealthProbeRecord
}

// NewMemoryRepository 创建内存仓储实例。
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{records: make(map[string]map[string]router.HealthProbeRecord)}
}

func key(capabilityID, tenantID string) string {
	return tenantID + "::" + capabilityID
}

// SaveProbeResult 存储健康探测结果（忽略 db 参数以保持接口兼容）。
func (r *MemoryRepository) SaveProbeResult(_ context.Context, _ *gorm.DB, result router.HealthProbeRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	routeKey := key(result.CapabilityID, result.TenantID)
	adapters := r.records[routeKey]
	if adapters == nil {
		adapters = make(map[string]router.HealthProbeRecord)
		r.records[routeKey] = adapters
	}
	adapters[result.AdapterID] = result
	return nil
}

// GetLatest 返回给定能力/租户的最新探测结果列表。
func (r *MemoryRepository) GetLatest(_ context.Context, _ *gorm.DB, capabilityID, tenantID string) ([]router.HealthProbeRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	routeKey := key(capabilityID, tenantID)
	adapters := r.records[routeKey]
	if len(adapters) == 0 {
		return nil, nil
	}
	results := make([]router.HealthProbeRecord, 0, len(adapters))
	for _, record := range adapters {
		results = append(results, record)
	}
	return results, nil
}

