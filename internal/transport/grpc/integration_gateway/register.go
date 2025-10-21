package integration_gateway

import (
	pbintegration "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/integration/gateway/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"google.golang.org/grpc"
)

// RegisterServers 注册 Integration Gateway gRPC 服务。
func RegisterServers(s *grpc.Server, deps *shared.Deps) {
	if s == nil || deps == nil || deps.IntegrationGateway == nil || deps.IntegrationGateway.Manager == nil {
		return
	}

	adminServer := NewAdminServer(deps.IntegrationGateway.Manager)
	tenantServer := NewTenantServer(deps.IntegrationGateway.Manager)

	pbintegration.RegisterIntegrationGatewayAdminServiceServer(s, adminServer)
	pbintegration.RegisterIntegrationGatewayTenantServiceServer(s, tenantServer)
}
