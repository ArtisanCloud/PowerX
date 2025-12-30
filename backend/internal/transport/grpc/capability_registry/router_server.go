package capability_registry

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	capabilityRegistryPB "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/registry/v1"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	routerService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
)

// RouterServer 提供 CapabilityRouterService gRPC 实现（占位）。
type RouterServer struct {
	capabilityRegistryPB.UnimplementedCapabilityRouterServiceServer
	service *routerService.Service
}

// NewRouterServer 创建 Router gRPC 服务器。
func NewRouterServer(service *routerService.Service) *RouterServer {
	return &RouterServer{service: service}
}

// RegisterCapabilityRouterServer 注册服务到 gRPC Server。
func RegisterCapabilityRouterServer(server *grpc.Server, svc *routerService.Service) {
	capabilityRegistryPB.RegisterCapabilityRouterServiceServer(server, &RouterServer{service: svc})
}

func (s *RouterServer) Invoke(ctx context.Context, req *capabilityRegistryPB.InvokeRequest) (*capabilityRegistryPB.InvokeResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.FailedPrecondition, "router service unavailable")
	}
	if req.GetCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "capability id required")
	}
	tenantUUID, err := tenantUUIDFromScopedID(req.GetCapability())
	if err != nil {
		return nil, err
	}
	result, err := s.service.Invoke(ctx, routerService.InvokeRequest{
		CapabilityID: req.GetCapability().GetCapabilityId(),
		TenantUUID:   tenantUUID,
		Payload:      req.GetPayload(),
		Timeout:      time.Duration(req.GetTimeoutMs()) * time.Millisecond,
		StickyKey:    req.GetStickyKey(),
	})
	if err != nil {
		if errors.Is(err, registry.ErrRegistrationNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	resp := &capabilityRegistryPB.InvokeResponse{
		AdapterId:    result.AdapterID,
		Payload:      result.Payload,
		FallbackUsed: result.FallbackUsed,
		LatencyMs:    uint32(result.Latency / time.Millisecond),
	}
	if result.Error != nil {
		resp.Error = &capabilityRegistryPB.InvokeError{
			Code:    "router.invoke_error",
			Message: result.Error.Error(),
		}
	}
	return resp, nil
}

func (s *RouterServer) StreamInvoke(req *capabilityRegistryPB.StreamInvokeRequest, stream capabilityRegistryPB.CapabilityRouterService_StreamInvokeServer) error {
	if s.service == nil {
		return status.Error(codes.FailedPrecondition, "router service unavailable")
	}
	if req == nil {
		return status.Error(codes.InvalidArgument, "request required")
	}
	resp, err := s.Invoke(stream.Context(), req.GetRequest())
	if err != nil {
		return err
	}
	if err := stream.Send(&capabilityRegistryPB.StreamInvokeResponse{
		AdapterId:   resp.GetAdapterId(),
		Chunk:       resp.GetPayload(),
		EndOfStream: true,
		Error:       resp.GetError(),
		TraceId:     resp.GetTraceId(),
	}); err != nil {
		return err
	}
	return nil
}

func (s *RouterServer) ReportHealth(ctx context.Context, req *capabilityRegistryPB.ReportHealthRequest) (*capabilityRegistryPB.ReportHealthResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.FailedPrecondition, "router service unavailable")
	}
	if req.GetId() == nil {
		return nil, status.Error(codes.InvalidArgument, "capability id required")
	}
	tenantUUID, err := tenantUUIDFromScopedID(req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.service.ReportHealth(ctx, routerService.ReportHealthInput{
		CapabilityID: req.GetId().GetCapabilityId(),
		TenantUUID:   tenantUUID,
		AdapterID:    req.GetAdapterId(),
		Status:       req.GetStatus(),
		Reason:       req.GetReason(),
		Failures:     req.GetFailures(),
	}); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &capabilityRegistryPB.ReportHealthResponse{Accepted: true}, nil
}
