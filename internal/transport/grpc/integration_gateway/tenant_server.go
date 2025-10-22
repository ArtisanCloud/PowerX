package integration_gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	pbintegration "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/integration_gateway/v1"
	integrationTenant "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/tenant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TenantServer 实现租户侧 Integration Gateway gRPC 服务。
type TenantServer struct {
	pbintegration.UnimplementedIntegrationGatewayTenantServiceServer
	svc *integrationTenant.Service
}

// NewTenantServer 构造租户服务。
func NewTenantServer(svc *integrationTenant.Service) *TenantServer {
	return &TenantServer{svc: svc}
}

func (s *TenantServer) ListRoutes(ctx context.Context, req *pbintegration.TenantListRoutesRequest) (*pbintegration.ListRoutesResponse, error) {
	if s == nil || s.svc == nil {
		return nil, status.Error(codes.Unavailable, "tenant service unavailable")
	}
	if req == nil || strings.TrimSpace(req.GetTenantId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}

	routes, err := s.svc.ListRoutes(ctx, strings.TrimSpace(req.GetTenantId()), strings.TrimSpace(req.GetCapabilityId()), strings.TrimSpace(req.GetChannel()))
	if err != nil {
		return nil, status.Error(codes.Internal, "list routes failed")
	}

	items := make([]*pbintegration.IntegrationRouteSummary, 0, len(routes))
	for _, route := range routes {
		items = append(items, routeToSummaryProto(route))
	}

	return &pbintegration.ListRoutesResponse{
		Items: items,
		Total: uint64(len(items)),
	}, nil
}

func (s *TenantServer) GetRoute(ctx context.Context, req *pbintegration.TenantGetRouteRequest) (*pbintegration.TenantGetRouteResponse, error) {
	if s == nil || s.svc == nil {
		return nil, status.Error(codes.Unavailable, "tenant service unavailable")
	}
	if req == nil || strings.TrimSpace(req.GetTenantId()) == "" || strings.TrimSpace(req.GetRouteSlug()) == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id and route_slug are required")
	}

	route, err := s.svc.GetRoute(ctx, strings.TrimSpace(req.GetTenantId()), strings.TrimSpace(req.GetRouteSlug()))
	if err != nil {
		return nil, translateTenantError(err)
	}

	return &pbintegration.TenantGetRouteResponse{
		Route: routeToProto(route),
	}, nil
}

func (s *TenantServer) InvokeRoute(ctx context.Context, req *pbintegration.TenantInvokeRequest) (*pbintegration.TenantInvokeResponse, error) {
	if s == nil || s.svc == nil {
		return nil, status.Error(codes.Unavailable, "tenant service unavailable")
	}
	if req == nil || strings.TrimSpace(req.GetTenantId()) == "" || strings.TrimSpace(req.GetRouteSlug()) == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id and route_slug are required")
	}

	var payload map[string]any
	if len(req.GetPayloadJson()) > 0 {
		if err := json.Unmarshal(req.GetPayloadJson(), &payload); err != nil {
			return nil, status.Error(codes.InvalidArgument, "payload_json must be valid JSON")
		}
	}

	result, err := s.svc.Invoke(ctx, integrationTenant.InvokeInput{
		TenantID:       strings.TrimSpace(req.GetTenantId()),
		RouteSlug:      strings.TrimSpace(req.GetRouteSlug()),
		Channel:        "http",
		Payload:        payload,
		IdempotencyKey: strings.TrimSpace(req.GetIdempotencyKey()),
		TraceID:        strings.TrimSpace(req.GetTraceId()),
	})

	if err != nil {
		var rlErr integrationTenant.RateLimitError
		if errors.As(err, &rlErr) {
			resp := &pbintegration.TenantInvokeResponse{
				Status:       pbintegration.TenantInvokeResponse_RATE_LIMITED,
				TraceId:      result.TraceID,
				ErrorCode:    "rate_limited",
				ErrorMessage: err.Error(),
			}
			return resp, nil
		}
		return nil, translateTenantError(err)
	}

	resp := &pbintegration.TenantInvokeResponse{
		Status:             mapInvokeStatus(result.Status),
		TraceId:            result.TraceID,
		RoutedCapabilityId: result.RoutedCapabilityID,
		RoutedAdapter:      result.RoutedAdapter,
		ErrorCode:          result.ErrorCode,
		ErrorMessage:       result.ErrorMessage,
	}
	if len(result.Result) > 0 {
		if bytes, err := json.Marshal(result.Result); err == nil {
			resp.ResultJson = bytes
		}
	}
	if !result.DispatchedAt.IsZero() {
		resp.DispatchedAt = timestamppb.New(result.DispatchedAt)
	}

	return resp, nil
}

func translateTenantError(err error) error {
	var routeErr integrationTenant.ErrRouteNotAccessible
	if errors.As(err, &routeErr) {
		return status.Error(codes.NotFound, "route not accessible")
	}
	var channelErr integrationTenant.ErrChannelDisabled
	if errors.As(err, &channelErr) {
		return status.Error(codes.NotFound, "channel disabled")
	}
	var grantErr integrationTenant.ErrToolGrantDenied
	if errors.As(err, &grantErr) {
		return status.Error(codes.PermissionDenied, "tool grant denied")
	}
	return status.Error(codes.FailedPrecondition, "integration gateway invoke failed")
}

func mapInvokeStatus(status integrationTenant.InvokeStatus) pbintegration.TenantInvokeResponse_Status {
	switch status {
	case integrationTenant.InvokeStatusAccepted:
		return pbintegration.TenantInvokeResponse_ACCEPTED
	case integrationTenant.InvokeStatusDenied:
		return pbintegration.TenantInvokeResponse_DENIED
	case integrationTenant.InvokeStatusFailed:
		return pbintegration.TenantInvokeResponse_FAILED
	case integrationTenant.InvokeStatusRateLimited:
		return pbintegration.TenantInvokeResponse_RATE_LIMITED
	default:
		return pbintegration.TenantInvokeResponse_OK
	}
}
