package eventfabric

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	eventfabricv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/corex/event_fabric/v1"
	replay "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/replay"
	eventfabricgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/event_fabric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const replayBufSize = 1024 * 1024

func TestEventReplayGRPCContracts(t *testing.T) {
	env := newReplayGRPCTestEnv(t)
	t.Cleanup(env.Close)

	baseCtx := context.Background()
	tenantCtx := eventFabricGRPCContext(t, baseCtx, "tenant-corex")
	window := &eventfabricv1.ReplayWindow{
		Start: timestamppb.New(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		End:   timestamppb.New(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)),
	}

	createResp, err := env.client.CreateReplayTask(tenantCtx, &eventfabricv1.CreateReplayTaskRequest{
		TenantUuid: "tenant-corex",
		Topic:      "tenant-corex.corex.workflow.approved",
		TraceId:    "trace-abc",
		Window:     window,
		Reason:     "backfill",
		OperatorId: "ops-user",
		Shadow:     true,
	})
	if err != nil {
		t.Fatalf("CreateReplayTask unexpected error: %v", err)
	}
	assertNoEventFabricTenantLeakProto(t, createResp)
	if createResp.GetId() != "task-1" {
		t.Fatalf("unexpected task id %s", createResp.GetId())
	}

	got, err := env.client.GetReplayTask(tenantCtx, &eventfabricv1.GetReplayTaskRequest{TaskId: "task-1"})
	if err != nil {
		t.Fatalf("GetReplayTask unexpected error: %v", err)
	}
	assertNoEventFabricTenantLeakProto(t, got)
	if got.GetStatus() != "completed" {
		t.Fatalf("expected status completed got %s", got.GetStatus())
	}

	cancelResp, err := env.client.CancelReplayTask(tenantCtx, &eventfabricv1.CancelReplayTaskRequest{TaskId: "task-1", OperatorId: "ops-user"})
	if err != nil {
		t.Fatalf("CancelReplayTask unexpected error: %v", err)
	}
	assertNoEventFabricTenantLeakProto(t, cancelResp)
}

func TestEventReplayGRPCErrors(t *testing.T) {
	env := newReplayGRPCTestEnv(t)
	t.Cleanup(env.Close)

	env.stub.shouldFail = true
	_, err := env.client.CreateReplayTask(eventFabricGRPCContext(t, context.Background(), "tenant-corex"), &eventfabricv1.CreateReplayTaskRequest{TenantUuid: "tenant-corex", Topic: "topic"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal error got %v", status.Code(err))
	}
}

type replayGRPCTestEnv struct {
	listener *bufconn.Listener
	server   *grpc.Server
	client   eventfabricv1.EventReplayServiceClient
	stub     *stubReplayGRPCService
}

func newReplayGRPCTestEnv(t *testing.T) *replayGRPCTestEnv {
	listener := bufconn.Listen(replayBufSize)
	server := grpc.NewServer()

	stub := &stubReplayGRPCService{}
	eventfabricv1.RegisterEventReplayServiceServer(server, eventfabricgrpc.NewEventReplayServer(stub))

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Logf("replay grpc server stopped: %v", err)
		}
	}()

	conn, err := grpc.DialContext(context.Background(), "bufconn", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	client := eventfabricv1.NewEventReplayServiceClient(conn)

	return &replayGRPCTestEnv{
		listener: listener,
		server:   server,
		client:   client,
		stub:     stub,
	}
}

func (env *replayGRPCTestEnv) Close() {
	env.server.GracefulStop()
	env.listener.Close()
}

type stubReplayGRPCService struct {
	mu         sync.Mutex
	shouldFail bool
}

func (s *stubReplayGRPCService) CreateTask(ctx context.Context, input replay.CreateTaskInput) (*replay.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shouldFail {
		return nil, fmt.Errorf("create failed")
	}
	return &replay.Task{
		ID:          "task-1",
		TenantKey:   input.TenantKey,
		Topic:       input.Topic,
		TraceID:     input.TraceID,
		Status:      "completed",
		RequestedBy: input.Operator,
		SubmittedAt: time.Now().UTC(),
	}, nil
}

func (s *stubReplayGRPCService) GetTask(ctx context.Context, id string) (*replay.Task, error) {
	return &replay.Task{
		ID:          id,
		TenantKey:   "tenant-corex",
		Topic:       "tenant-corex.corex.workflow.approved",
		TraceID:     "trace-abc",
		Status:      "completed",
		RequestedBy: "ops-user",
		SubmittedAt: time.Now().UTC(),
	}, nil
}

func (s *stubReplayGRPCService) CancelTask(ctx context.Context, id string, operator string) error {
	if s.shouldFail {
		return fmt.Errorf("cancel failed")
	}
	return nil
}
