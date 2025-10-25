package eventfabric

import (
	"context"
	"errors"
	"strings"
	"time"

	eventfabricv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/corex/event_fabric/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	sharedsvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// EventDeliveryServer 提供 Publish/Ack/Nack gRPC 能力。
type EventDeliveryServer struct {
	eventfabricv1.UnimplementedEventDeliveryServiceServer
	delivery delivery.Service
}

// NewEventDeliveryServer 构建 gRPC 服务实例。
func NewEventDeliveryServer(svc delivery.Service) *EventDeliveryServer {
	return &EventDeliveryServer{delivery: svc}
}

func (s *EventDeliveryServer) PublishEvent(ctx context.Context, req *eventfabricv1.PublishEventRequest) (*emptypb.Empty, error) {
	if s.delivery == nil {
		return nil, status.Error(codes.FailedPrecondition, "event delivery service unavailable")
	}

	tenant := strings.TrimSpace(req.GetTenantId())
	if tenant == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	topic := strings.TrimSpace(req.GetTopic())
	if topic == "" {
		return nil, status.Error(codes.InvalidArgument, "topic is required")
	}

	attributes := make(map[string]string, len(req.GetAttributes()))
	for k, v := range req.GetAttributes() {
		if key := strings.TrimSpace(k); key != "" {
			attributes[key] = v
		}
	}
	if _, ok := attributes["principal_id"]; !ok {
		// 默认使用 gRPC metadata 中的主体也可，但此处保持空值由服务内部审计处理。
		attributes["principal_id"] = ""
	}

	err := s.delivery.Publish(ctx, delivery.PublishRequest{
		TenantID:       tenant,
		Topic:          topic,
		EventID:        strings.TrimSpace(req.GetEventId()),
		TraceID:        strings.TrimSpace(req.GetTraceId()),
		Version:        strings.TrimSpace(req.GetVersion()),
		Payload:        req.GetPayload(),
		PayloadFormat:  strings.TrimSpace(req.GetPayloadFormat()),
		IdempotencyKey: strings.TrimSpace(req.GetIdempotencyKey()),
		Attributes:     attributes,
	})
	if err != nil {
		return nil, convertDeliveryError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *EventDeliveryServer) AckDelivery(ctx context.Context, req *eventfabricv1.AckDeliveryRequest) (*emptypb.Empty, error) {
	if s.delivery == nil {
		return nil, status.Error(codes.FailedPrecondition, "event delivery service unavailable")
	}
	if strings.TrimSpace(req.GetDeliveryId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "delivery_id is required")
	}

	if err := s.delivery.Ack(ctx, strings.TrimSpace(req.GetDeliveryId()), strings.TrimSpace(req.GetSubscriberId())); err != nil {
		return nil, convertDeliveryError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *EventDeliveryServer) NackDelivery(ctx context.Context, req *eventfabricv1.NackDeliveryRequest) (*eventfabricv1.NackDeliveryResponse, error) {
	if s.delivery == nil {
		return nil, status.Error(codes.FailedPrecondition, "event delivery service unavailable")
	}
	if strings.TrimSpace(req.GetDeliveryId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "delivery_id is required")
	}

	plan, err := s.delivery.Nack(ctx, strings.TrimSpace(req.GetDeliveryId()), strings.TrimSpace(req.GetSubscriberId()), strings.TrimSpace(req.GetReason()))
	if err != nil {
		return nil, convertDeliveryError(err)
	}

	seconds := int32(plan.NextDelay / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	return &eventfabricv1.NackDeliveryResponse{
		RemainingAttempts: plan.RemainingAttempts,
		NextDelaySeconds:  seconds,
		Strategy:          plan.Strategy,
	}, nil
}

func convertDeliveryError(err error) error {
	switch {
	case errors.Is(err, sharedsvc.ErrTenantMismatch):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, sharedsvc.ErrUnauthorized):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, sharedsvc.ErrAckTimeout):
		return status.Error(codes.DeadlineExceeded, err.Error())
	case errors.Is(err, sharedsvc.ErrRetryExhausted):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Errorf(codes.Internal, "event delivery failure: %v", err)
	}
}

// RegisterEventDeliveryServer 返回注册函数，便于集中挂载。
func RegisterEventDeliveryServer(deps *shared.Deps) Registrar {
	return func(server grpc.ServiceRegistrar) {
		if deps == nil || deps.EventFabric == nil || deps.EventFabric.Delivery == nil {
			return
		}
		eventfabricv1.RegisterEventDeliveryServiceServer(server, NewEventDeliveryServer(deps.EventFabric.Delivery))
	}
}
