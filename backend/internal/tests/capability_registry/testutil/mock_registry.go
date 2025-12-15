package testutil

import (
	"context"
	"errors"
	"strings"
	"sync"

	"gorm.io/gorm"

	capabilityRegistryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
)

// MockRegistryRepository 提供线程安全的内存实现，便于测试场景自定义注册信息。
type MockRegistryRepository struct {
	mu            sync.RWMutex
	registrations map[string]capabilityRegistryService.Registration
	forcedErr     error
}

// NewMockRegistryRepository 根据给定的注册列表构造内存仓储。
func NewMockRegistryRepository(registrations []capabilityRegistryService.Registration) *MockRegistryRepository {
	store := make(map[string]capabilityRegistryService.Registration, len(registrations))
	for _, reg := range registrations {
		store[keyFor(reg.CapabilityID, tenantKeyFromRegistration(reg))] = reg
	}
	return &MockRegistryRepository{
		registrations: store,
	}
}

// SetError 让仓储在访问时返回指定错误。
func (m *MockRegistryRepository) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forcedErr = err
}

// UpsertRegistration 调整或新增注册信息，方便测试覆盖不同版本。
func (m *MockRegistryRepository) UpsertRegistration(reg capabilityRegistryService.Registration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registrations[keyFor(reg.CapabilityID, tenantKeyFromRegistration(reg))] = reg
}

// DeleteRegistration 移除指定注册。
func (m *MockRegistryRepository) DeleteRegistration(capabilityID, tenantUUID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.registrations, keyFor(capabilityID, canonicalTenantKey(tenantUUID)))
}

func (m *MockRegistryRepository) Create(context.Context, *gorm.DB, capabilityRegistryService.Registration) (capabilityRegistryService.Registration, error) {
	return capabilityRegistryService.Registration{}, errors.New("mock: not implemented")
}

func (m *MockRegistryRepository) Update(context.Context, *gorm.DB, capabilityRegistryService.Registration, uint64) (capabilityRegistryService.Registration, error) {
	return capabilityRegistryService.Registration{}, errors.New("mock: not implemented")
}

func (m *MockRegistryRepository) Disable(context.Context, *gorm.DB, string, string, string, string, uint64, capabilityRegistryService.Registration) (capabilityRegistryService.Registration, error) {
	return capabilityRegistryService.Registration{}, errors.New("mock: not implemented")
}

func (m *MockRegistryRepository) GetLatest(ctx context.Context, _ *gorm.DB, capabilityID, tenantUUID string) (capabilityRegistryService.Registration, error) {
	m.mu.RLock()
	err := m.forcedErr
	reg, ok := m.registrations[keyFor(capabilityID, canonicalTenantKey(tenantUUID))]
	m.mu.RUnlock()
	if err != nil {
		return capabilityRegistryService.Registration{}, err
	}
	if !ok {
		return capabilityRegistryService.Registration{}, capabilityRegistryService.ErrRegistrationNotFound
	}
	return reg, nil
}

func (m *MockRegistryRepository) GetVersion(ctx context.Context, db *gorm.DB, capabilityID, tenantUUID string, version uint64) (capabilityRegistryService.Registration, error) {
	return m.GetLatest(ctx, db, capabilityID, tenantUUID)
}

func (m *MockRegistryRepository) ListLatest(ctx context.Context, _ *gorm.DB, tenantUUID string, limit, offset int) ([]capabilityRegistryService.Registration, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.forcedErr != nil {
		return nil, 0, m.forcedErr
	}
	result := make([]capabilityRegistryService.Registration, 0, len(m.registrations))
	canonicalTenant := canonicalTenantKey(tenantUUID)
	for _, reg := range m.registrations {
		if tenantKeyFromRegistration(reg) == canonicalTenant {
			result = append(result, reg)
		}
	}
	total := int64(len(result))
	if offset > len(result) {
		return []capabilityRegistryService.Registration{}, total, nil
	}
	end := len(result)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return result[offset:end], total, nil
}

func keyFor(capabilityID, tenant string) string {
	return tenant + "::" + capabilityID
}

func tenantKeyFromRegistration(reg capabilityRegistryService.Registration) string {
	return canonicalTenantKey(reg.TenantUUID)
}

func canonicalTenantKey(value string) string {
	return strings.TrimSpace(value)
}
