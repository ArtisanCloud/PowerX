package router

import (
	"context"
	"sync"

	"gorm.io/gorm"
)

// MemoryRepository 提供线程安全的内存健康状态存储，适用于单机或测试环境。
type MemoryRepository struct {
	mu      sync.RWMutex
	records map[string]map[string]HealthProbeRecord
}

// NewMemoryRepository 创建默认的内存健康仓储实现。
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{records: make(map[string]map[string]HealthProbeRecord)}
}

func memoryKey(capabilityID, tenantID string) string {
	return tenantID + "::" + capabilityID
}

// SaveProbeResult 存储最新健康探测结果。
func (r *MemoryRepository) SaveProbeResult(_ context.Context, _ *gorm.DB, result HealthProbeRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := memoryKey(result.CapabilityID, result.TenantID)
	adapters := r.records[key]
	if adapters == nil {
		adapters = make(map[string]HealthProbeRecord)
		r.records[key] = adapters
	}
	adapters[result.AdapterID] = result
	return nil
}

// GetLatest 返回指定能力的最新健康探测结果。
func (r *MemoryRepository) GetLatest(_ context.Context, _ *gorm.DB, capabilityID, tenantID string) ([]HealthProbeRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := memoryKey(capabilityID, tenantID)
	adapters := r.records[key]
	if len(adapters) == 0 {
		return nil, nil
	}
	results := make([]HealthProbeRecord, 0, len(adapters))
	for _, record := range adapters {
		results = append(results, record)
	}
	return results, nil
}
