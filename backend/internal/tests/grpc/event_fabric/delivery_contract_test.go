package eventfabric

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	eventfabricv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/event_fabric/v1"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	sharedsvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	eventfabricgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/event_fabric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const (
	grpcBufSize      = 1024 * 1024
	sampleTenantUUID = "00000000-0000-0000-0000-000000000001"
)

func TestEventDeliveryGRPCContracts(t *testing.T) {
	env := newEventDeliveryGRPCTestEnv(t)
	t.Cleanup(env.Close)

	baseCtx := context.Background()
	tenantCtx := eventFabricGRPCContext(t, baseCtx, sampleTenantUUID)

	pubResp, err := env.deliveryClient.PublishEvent(tenantCtx, &eventfabricv1.PublishEventRequest{
		TenantUuid:    sampleTenantUUID,
		Topic:         "tenant-corex.corex.workflow.approved",
		EventId:       "evt-001",
		TraceId:       "trace-abc",
		Version:       "v1",
		Payload:       []byte("hello"),
		PayloadFormat: "json",
		Attributes:    map[string]string{"principal_id": "svc-publisher"},
	})
	if err != nil {
		t.Fatalf("PublishEvent unexpected error: %v", err)
	}
	assertNoEventFabricTenantLeakProto(t, pubResp)
	if len(env.stub.publishRequests) != 1 {
		t.Fatalf("expected 1 publish call got %d", len(env.stub.publishRequests))
	}

	ackResp, err := env.deliveryClient.AckDelivery(tenantCtx, &eventfabricv1.AckDeliveryRequest{
		DeliveryId:   "delivery-001",
		SubscriberId: "svc-sub",
	})
	if err != nil {
		t.Fatalf("AckDelivery unexpected error: %v", err)
	}
	assertNoEventFabricTenantLeakProto(t, ackResp)

	env.stub.nackPlan = delivery.RetryPlan{MaxAttempts: 5, RemainingAttempts: 3, NextDelay: 2 * time.Second, Strategy: "exponential-jitter"}
	nackResp, err := env.deliveryClient.NackDelivery(tenantCtx, &eventfabricv1.NackDeliveryRequest{
		DeliveryId:   "delivery-001",
		SubscriberId: "svc-sub",
		Reason:       "temporary failure",
	})
	if err != nil {
		t.Fatalf("NackDelivery unexpected error: %v", err)
	}
	assertNoEventFabricTenantLeakProto(t, nackResp)
	if nackResp.GetRemainingAttempts() != 3 || nackResp.GetNextDelaySeconds() != 2 {
		t.Fatalf("unexpected nack response: %+v", nackResp)
	}
}

func TestEventDeliveryErrorMapping(t *testing.T) {
	env := newEventDeliveryGRPCTestEnv(t)
	t.Cleanup(env.Close)

	env.stub.ackErr = sharedsvc.ErrUnauthorized
	_, err := env.deliveryClient.AckDelivery(eventFabricGRPCContext(t, context.Background(), sampleTenantUUID), &eventfabricv1.AckDeliveryRequest{
		DeliveryId:   "delivery-err",
		SubscriberId: "svc-sub",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied got %v", status.Code(err))
	}

	env.stub.nackErr = sharedsvc.ErrRetryExhausted
	_, err = env.deliveryClient.NackDelivery(eventFabricGRPCContext(t, context.Background(), sampleTenantUUID), &eventfabricv1.NackDeliveryRequest{
		DeliveryId:   "delivery-err",
		SubscriberId: "svc-sub",
		Reason:       "exhausted",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition got %v", status.Code(err))
	}
}

func TestEventSubscriberStream(t *testing.T) {
	env := newEventDeliveryGRPCTestEnv(t)
	t.Cleanup(env.Close)

	message := delivery.DeliveryAttempt{
		AttemptNumber: 1,
		SubscriberID:  "svc-sub",
		DeliveryUUID:  "attempt-001",
		EnvelopeUUID:  "env-001",
		EventID:       "evt-001",
		TopicFullName: "tenant-corex.corex.workflow.approved",
		Version:       "v1",
		PayloadFormat: "json",
		Payload:       []byte(`{"hello":"world"}`),
		TraceID:       "trace-abc",
		AckTimeout:    2 * time.Second,
		MaxAttempts:   5,
		Remaining:     4,
	}
	env.stub.enqueuePollResult(map[string][]delivery.DeliveryAttempt{
		message.EventID: {message},
	})

	ctx, cancel := context.WithCancel(eventFabricGRPCContext(t, context.Background(), sampleTenantUUID))
	defer cancel()
	stream, err := env.subscriberClient.Subscribe(ctx, &eventfabricv1.SubscribeRequest{
		TenantUuid:        sampleTenantUUID,
		SubscriberId:      "svc-sub",
		BatchSize:         10,
		CompatibilityMode: eventfabricv1.VersionCompatibilityMode_VERSION_COMPATIBILITY_MODE_BACKWARD,
		SupportedVersions: []string{"v2"},
	})
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}

	msg, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv error: %v", err)
	}
	if msg.GetDeliveryId() != "attempt-001" || msg.GetEventId() != "evt-001" {
		t.Fatalf("unexpected message payload: %+v", msg)
	}
	assertNoEventFabricTenantLeakProto(t, msg)
	if msg.GetSubscriberId() != "svc-sub" || msg.GetTopic() != "tenant-corex.corex.workflow.approved" {
		t.Fatalf("unexpected subscriber/topic: %+v", msg)
	}
	if msg.GetAckDeadline().AsTime().Before(time.Now()) {
		t.Fatalf("ack deadline should be in future")
	}
	cancel()
}

type eventDeliveryGRPCTestEnv struct {
	listener         *bufconn.Listener
	server           *grpc.Server
	stub             *grpcDeliveryStub
	deliveryClient   eventfabricv1.EventDeliveryServiceClient
	subscriberClient eventfabricv1.EventSubscriberServiceClient
}

func newEventDeliveryGRPCTestEnv(t *testing.T) *eventDeliveryGRPCTestEnv {
	listener := bufconn.Listen(grpcBufSize)
	server := grpc.NewServer()

	stub := newGRPCDeliveryStub()
	eventfabricv1.RegisterEventDeliveryServiceServer(server, eventfabricgrpc.NewEventDeliveryServer(stub))
	eventfabricv1.RegisterEventSubscriberServiceServer(server, eventfabricgrpc.NewEventSubscriberServer(stub, 20*time.Millisecond))

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Logf("gRPC server closed: %v", err)
		}
	}()

	conn, err := grpc.DialContext(context.Background(), "bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	return &eventDeliveryGRPCTestEnv{
		listener:         listener,
		server:           server,
		stub:             stub,
		deliveryClient:   eventfabricv1.NewEventDeliveryServiceClient(conn),
		subscriberClient: eventfabricv1.NewEventSubscriberServiceClient(conn),
	}
}

func (e *eventDeliveryGRPCTestEnv) Close() {
	e.server.GracefulStop()
	e.listener.Close()
}

type grpcDeliveryStub struct {
	mu              sync.Mutex
	publishRequests []delivery.PublishRequest
	ackCalls        []struct {
		deliveryID   string
		subscriberID string
	}
	nackCalls []struct {
		deliveryID   string
		subscriberID string
		reason       string
	}
	pollQueue []map[string][]delivery.DeliveryAttempt

	publishErr error
	ackErr     error
	nackErr    error
	nackPlan   delivery.RetryPlan
	pollErr    error
}

func newGRPCDeliveryStub() *grpcDeliveryStub {
	return &grpcDeliveryStub{
		pollQueue: make([]map[string][]delivery.DeliveryAttempt, 0),
		nackPlan:  delivery.RetryPlan{},
	}
}

func (s *grpcDeliveryStub) Publish(ctx context.Context, req delivery.PublishRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishRequests = append(s.publishRequests, req)
	return s.publishErr
}

func (s *grpcDeliveryStub) Ack(ctx context.Context, deliveryID string, subscriberID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ackCalls = append(s.ackCalls, struct {
		deliveryID   string
		subscriberID string
	}{deliveryID: deliveryID, subscriberID: subscriberID})
	return s.ackErr
}

func (s *grpcDeliveryStub) Nack(ctx context.Context, deliveryID string, subscriberID string, reason string) (delivery.RetryPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nackCalls = append(s.nackCalls, struct {
		deliveryID   string
		subscriberID string
		reason       string
	}{deliveryID: deliveryID, subscriberID: subscriberID, reason: reason})
	return s.nackPlan, s.nackErr
}

func (s *grpcDeliveryStub) PollRetry(ctx context.Context, limit int) (map[string][]delivery.DeliveryAttempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pollQueue) == 0 {
		return map[string][]delivery.DeliveryAttempt{}, s.pollErr
	}
	result := s.pollQueue[0]
	s.pollQueue = s.pollQueue[1:]
	return result, nil
}

func (s *grpcDeliveryStub) enqueuePollResult(result map[string][]delivery.DeliveryAttempt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pollQueue = append(s.pollQueue, result)
}
