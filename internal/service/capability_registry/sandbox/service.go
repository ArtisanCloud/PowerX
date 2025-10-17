package sandbox

import (
	"context"
	"errors"

	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
)

// Service 提供 Router Sandbox 能力。
type Service struct {
	registryRepo registry.Repository
	routerSvc    *router.Service
}

// NewService 创建 Sandbox 服务实例。
func NewService(registryRepo registry.Repository, routerSvc *router.Service) *Service {
	if registryRepo == nil {
		panic("sandbox service requires registry repository")
	}
	if routerSvc == nil {
		panic("sandbox service requires router service")
	}
	return &Service{registryRepo: registryRepo, routerSvc: routerSvc}
}

// SimulateInvoke 使用现有注册信息模拟路由结果，不影响真实路由状态。
func (s *Service) SimulateInvoke(ctx context.Context, capabilityID, tenantID string, req router.InvokeRequest, override *registry.Registration) (router.InvokeResult, error) {
	if capabilityID == "" || tenantID == "" {
		return router.InvokeResult{}, errors.New("sandbox: capability/tenant required")
	}
	var reg registry.Registration
	var err error
	if override != nil && override.CapabilityID != "" {
		reg = *override
		if reg.CapabilityID == "" {
			reg.CapabilityID = capabilityID
		}
		if reg.TenantID == "" {
			reg.TenantID = tenantID
		}
	} else {
		reg, err = s.registryRepo.GetLatest(ctx, nil, capabilityID, tenantID)
		if err != nil {
			return router.InvokeResult{}, err
		}
	}
	req.CapabilityID = capabilityID
	req.TenantID = tenantID
	return s.routerSvc.Simulate(ctx, reg, req)
}
