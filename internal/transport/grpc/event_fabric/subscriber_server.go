package eventfabric

import (
	"context"
	"strings"
	"time"

	eventfabricv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/corex/event_fabric/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	sharedsvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultPollInterval = 200 * time.Millisecond

// EventSubscriberServer 提供流式订阅能力。
type EventSubscriberServer struct {
	eventfabricv1.UnimplementedEventSubscriberServiceServer
	delivery     delivery.Service
	pollInterval time.Duration
}

// NewEventSubscriberServer 构建订阅服务实例。
func NewEventSubscriberServer(svc delivery.Service, interval time.Duration) *EventSubscriberServer {
	if interval <= 0 {
		interval = defaultPollInterval
	}
	return &EventSubscriberServer{
		delivery:     svc,
		pollInterval: interval,
	}
}

func (s *EventSubscriberServer) Subscribe(req *eventfabricv1.SubscribeEventsRequest, stream eventfabricv1.EventSubscriberService_SubscribeServer) error {
	if s.delivery == nil {
		return status.Error(codes.FailedPrecondition, "event delivery service unavailable")
	}

	tenant := strings.TrimSpace(req.GetTenantId())
	if tenant == "" {
		return status.Error(codes.InvalidArgument, "tenant_id is required")
	}
	subscriber := strings.TrimSpace(req.GetSubscriberId())
	if subscriber == "" {
		return status.Error(codes.InvalidArgument, "subscriber_id is required")
	}

	batch := int(req.GetBatchSize())
	if batch <= 0 {
		batch = 50
	}

	ctx := stream.Context()
	ctx = context.WithValue(ctx, sharedsvc.ContextTenantKey, tenant)
	ctx = context.WithValue(ctx, sharedsvc.ContextSubscriberKey, subscriber)

	if topics := normalizeTopics(req.GetTopics()); len(topics) > 0 {
		ctx = context.WithValue(ctx, sharedsvc.ContextTopicsKey, topics)
	}

	sleep := s.pollInterval
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		results, err := s.delivery.PollRetry(ctx, batch)
		if err != nil {
			if errors.Is(err, sharedsvc.ErrTenantMismatch) {
				return status.Error(codes.InvalidArgument, err.Error())
			}
			return status.Errorf(codes.Internal, "poll retry failed: %v", err)
		}

		if len(results) == 0 {
			time.Sleep(sleep)
			continue
		}

		for _, attempts := range results {
			for _, attempt := range attempts {
				deadline := time.Now().Add(attempt.AckTimeout)
				msg := &eventfabricv1.DeliveryMessage{
					DeliveryId:        attempt.DeliveryUUID,
					EventId:           attempt.EventID,
					Topic:             attempt.TopicFullName,
					TraceId:           attempt.TraceID,
					Version:           attempt.Version,
					Payload:           attempt.Payload,
					PayloadFormat:     attempt.PayloadFormat,
					Headers:           attempt.Headers,
					Attempt:           attempt.AttemptNumber,
					MaxAttempts:       attempt.MaxAttempts,
					RemainingAttempts: attempt.Remaining,
					AckDeadline:       timestamppb.New(deadline),
					SubscriberId:      attempt.SubscriberID,
				}
				if err := stream.Send(msg); err != nil {
					return err
				}
			}
		}
	}
}

func normalizeTopics(topics []string) map[string]struct{} {
	if len(topics) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		if t := strings.TrimSpace(topic); t != "" {
			set[strings.ToLower(t)] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

// RegisterEventSubscriberServer 返回注册函数。
func RegisterEventSubscriberServer(deps *shared.Deps) Registrar {
	return func(server grpc.ServiceRegistrar) {
		if deps == nil || deps.EventFabric == nil || deps.EventFabric.Delivery == nil {
			return
		}
		eventfabricv1.RegisterEventSubscriberServiceServer(server, NewEventSubscriberServer(deps.EventFabric.Delivery, defaultPollInterval))
	}
}
