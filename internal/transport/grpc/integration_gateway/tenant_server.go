package integration_gateway

import (
	"context"

	pbintegration "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/integration/gateway/v1"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TenantServer 暂未实现租户侧能力，返回 Unimplemented。
type TenantServer struct {
	pbintegration.UnimplementedIntegrationGatewayTenantServiceServer
	svc *manager.Service
}

// NewTenantServer 构造租户服务。
func NewTenantServer(svc *manager.Service) *TenantServer {
	return &TenantServer{svc: svc}
}

func (s *TenantServer) ListRoutes(context.Context, *pbintegration.TenantListRoutesRequest) (*pbintegration.ListRoutesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "tenant API not implemented yet")
}

func (s *TenantServer) GetRoute(context.Context, *pbintegration.TenantGetRouteRequest) (*pbintegration.TenantGetRouteResponse, error) {
	return nil, status.Error(codes.Unimplemented, "tenant API not implemented yet")
}

func (s *TenantServer) InvokeRoute(context.Context, *pbintegration.TenantInvokeRequest) (*pbintegration.TenantInvokeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "tenant API not implemented yet")
}
