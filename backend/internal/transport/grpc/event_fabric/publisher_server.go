package eventfabric

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"strings"

	eventfabricv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/event_fabric/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	servicechannel "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/service_channel"
	sharedsvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// EventDeliveryServer 提供 Publish/Ack/Nack gRPC 能力。
type ServiceChannel interface {
	Publish(context.Context, servicechannel.Request) (map[string]any, error)
	Settle(context.Context, servicechannel.Request) (map[string]any, error)
	Stream(context.Context, servicechannel.Request, func(string, any) error) error
}

type EventDeliveryServer struct {
	eventfabricv1.UnimplementedEventDeliveryServiceServer
	delivery ServiceChannel
}

// NewEventDeliveryServer 构建 gRPC 服务实例。
func NewEventDeliveryServer(svc ServiceChannel) *EventDeliveryServer {
	return &EventDeliveryServer{delivery: svc}
}

func (s *EventDeliveryServer) PublishEvent(ctx context.Context, req *eventfabricv1.PublishEventRequest) (*emptypb.Empty, error) {
	if s.delivery == nil {
		return nil, status.Error(codes.FailedPrecondition, "event delivery service unavailable")
	}

	_, err := s.delivery.Publish(ctx, servicechannel.Request{
		ContractVersion: req.GetContractVersion(), Operation: "publish", TenantUUID: req.GetTenantUuid(), PluginID: req.GetPluginId(),
		Event: &servicechannel.Event{EventID: req.GetEventId(), Topic: req.GetTopic(), Version: req.GetVersion(), Payload: req.GetPayload(), IdempotencyKey: req.GetIdempotencyKey(), TraceID: req.GetTraceId(), Attributes: req.GetAttributes()},
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

	if _, err := s.delivery.Settle(ctx, servicechannel.Request{ContractVersion: req.GetContractVersion(), Operation: "ack", TenantUUID: req.GetTenantUuid(), PluginID: req.GetPluginId(), SubscriberID: req.GetSubscriberId(), DeliveryID: req.GetDeliveryId()}); err != nil {
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

	result, err := s.delivery.Settle(ctx, servicechannel.Request{ContractVersion: req.GetContractVersion(), Operation: "nack", TenantUUID: req.GetTenantUuid(), PluginID: req.GetPluginId(), SubscriberID: req.GetSubscriberId(), DeliveryID: req.GetDeliveryId(), Reason: req.GetReason()})
	if err != nil {
		return nil, convertDeliveryError(err)
	}
	return &eventfabricv1.NackDeliveryResponse{Strategy: result["outcome"].(string), RemainingAttempts: int32(result["remaining_attempts"].(int)), NextDelaySeconds: int32(result["next_delay_seconds"].(int))}, nil
}

func convertDeliveryError(err error) error {
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "EVENT_FABRIC_CANCELLED")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "EVENT_FABRIC_DEADLINE_EXCEEDED")
	}
	if code := dto.CodeOf(err); code != "" {
		mapped := codes.Unavailable
		switch dto.StatusCode(err) {
		case 400:
			mapped = codes.InvalidArgument
		case 401:
			mapped = codes.Unauthenticated
		case 403:
			mapped = codes.PermissionDenied
		case 409:
			mapped = codes.FailedPrecondition
		}
		return status.Error(mapped, code)
	}
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
		eventfabricv1.RegisterEventDeliveryServiceServer(server, NewEventDeliveryServer(servicechannel.NewService(deps.DB, deps.EventFabric.Audit)))
	}
}
