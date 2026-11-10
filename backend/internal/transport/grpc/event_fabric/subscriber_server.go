package eventfabric

import (
	"strings"
	"time"

	eventfabricv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/event_fabric/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	servicechannel "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/service_channel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultPollInterval = 200 * time.Millisecond

// EventSubscriberServer 提供流式订阅能力。
type EventSubscriberServer struct {
	eventfabricv1.UnimplementedEventSubscriberServiceServer
	delivery     ServiceChannel
	pollInterval time.Duration
}

// NewEventSubscriberServer 构建订阅服务实例。
func NewEventSubscriberServer(svc ServiceChannel, interval time.Duration) *EventSubscriberServer {
	if interval <= 0 {
		interval = defaultPollInterval
	}
	return &EventSubscriberServer{
		delivery:     svc,
		pollInterval: interval,
	}
}

func (s *EventSubscriberServer) Subscribe(req *eventfabricv1.SubscribeRequest, stream eventfabricv1.EventSubscriberService_SubscribeServer) error {
	if s.delivery == nil {
		return status.Error(codes.FailedPrecondition, "event delivery service unavailable")
	}

	in := servicechannel.Request{ContractVersion: req.GetContractVersion(), Operation: "subscribe", TenantUUID: req.GetTenantUuid(), PluginID: req.GetPluginId(), SubscriberID: req.GetSubscriberId(), Topics: req.GetTopics(), BatchSize: int(req.GetBatchSize()), SupportedVersions: req.GetSupportedVersions(), CompatibilityMode: compatibilityModeToString(req.GetCompatibilityMode())}
	err := s.delivery.Stream(stream.Context(), in, func(kind string, value any) error {
		if kind != "delivery" {
			return nil
		}
		item := value.(servicechannel.Message)
		return stream.Send(&eventfabricv1.DeliveryMessage{DeliveryId: item.DeliveryID, EventId: item.EventID, IdempotencyKey: item.IdempotencyKey, Attributes: item.Attributes, Topic: item.Topic, TraceId: item.TraceID, Version: item.Version, Payload: item.Payload, PayloadFormat: "json", Attempt: int32(item.Attempt), MaxAttempts: int32(item.MaxAttempts), RemainingAttempts: int32(item.MaxAttempts - item.Attempt), AckDeadline: timestamppb.New(item.AckDeadline), SubscriberId: item.SubscriberID, TenantUuid: item.TenantUUID, ContractVersion: item.ContractVersion, PublishedBy: item.PublishedBy, PublisherPluginId: item.PublisherPluginID})
	})
	if err != nil {
		return convertDeliveryError(err)
	}
	return nil
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

func compatibilityModeToString(mode eventfabricv1.VersionCompatibilityMode) string {
	switch mode {
	case eventfabricv1.VersionCompatibilityMode_VERSION_COMPATIBILITY_MODE_STRICT:
		return "strict"
	case eventfabricv1.VersionCompatibilityMode_VERSION_COMPATIBILITY_MODE_BACKWARD:
		return "backward"
	case eventfabricv1.VersionCompatibilityMode_VERSION_COMPATIBILITY_MODE_ANY:
		return "any"
	default:
		return ""
	}
}

func normalizeSupportedVersions(versions []string) []string {
	if len(versions) == 0 {
		return nil
	}
	result := make([]string, 0, len(versions))
	for _, v := range versions {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// RegisterEventSubscriberServer 返回注册函数。
func RegisterEventSubscriberServer(deps *shared.Deps) Registrar {
	return func(server grpc.ServiceRegistrar) {
		if deps == nil || deps.EventFabric == nil || deps.EventFabric.Delivery == nil {
			return
		}
		eventfabricv1.RegisterEventSubscriberServiceServer(server, NewEventSubscriberServer(servicechannel.NewService(deps.DB, deps.EventFabric.Audit), defaultPollInterval))
	}
}
