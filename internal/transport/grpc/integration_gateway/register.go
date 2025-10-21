package integration_gateway

import (
	pbintegration "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/integration/gateway/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"google.golang.org/grpc"
)

var (
	_ = pbintegration.RegisterIntegrationGatewayAdminServiceServer
	_ = pbintegration.RegisterIntegrationGatewayTenantServiceServer
)

// RegisterServers 预留 gRPC 服务注册入口。
func RegisterServers(_ *grpc.Server, _ *shared.Deps) {
	// Phase 3 会实现 IntegrationGatewayAdminService 与 IntegrationGatewayTenantService。
}
