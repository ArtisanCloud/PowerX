package sandbox

import (
	"context"
	"errors"
	"strings"

	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	registryRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"gorm.io/gorm"
)

// Service 提供 Router Sandbox 能力。
type Service struct {
	registryRepo registry.Repository
	routerSvc    *router.Service
}

// ServiceOptions 聚合 Sandbox 构造依赖。
type ServiceOptions struct {
	DB                 *gorm.DB
	RegistryRepository registry.Repository
	RouterService      *router.Service
}

// NewService 创建 Sandbox 服务实例。
func NewService(opts ServiceOptions) *Service {
	repository := opts.RegistryRepository
	if repository == nil {
		if opts.DB == nil {
			panic("sandbox service requires DB when RegistryRepository is nil")
		}
		repository = registryRepo.NewCapabilityRegistryRepository(opts.DB)
	}
	if opts.RouterService == nil {
		panic("sandbox service requires router service")
	}
	return &Service{registryRepo: repository, routerSvc: opts.RouterService}
}

// SimulateInvoke 使用现有注册信息模拟路由结果，不影响真实路由状态。
func (s *Service) SimulateInvoke(ctx context.Context, capabilityID, tenantUUID string, req router.InvokeRequest, override *registry.Registration) (router.InvokeResult, error) {
	if capabilityID == "" || tenantUUID == "" {
		return router.InvokeResult{}, errors.New("sandbox: capability/tenant required")
	}
	var reg registry.Registration
	var err error
	if override != nil && override.CapabilityID != "" {
		reg = *override
		if reg.CapabilityID == "" {
			reg.CapabilityID = capabilityID
		}
		if strings.TrimSpace(reg.TenantUUID) == "" {
			reg.TenantUUID = tenantUUID
		}
	} else {
		reg, err = s.registryRepo.GetLatest(ctx, nil, capabilityID, tenantUUID)
		if err != nil {
			return router.InvokeResult{}, err
		}
	}
	req.CapabilityID = capabilityID
	req.TenantUUID = tenantUUID
	return s.routerSvc.Simulate(ctx, reg, req)
}
