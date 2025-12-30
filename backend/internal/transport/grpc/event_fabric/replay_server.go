package eventfabric

import (
	"context"
	"fmt"
	"time"

eventfabricv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/event_fabric/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/replay"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ReplayService 定义回放服务的必要能力。
type ReplayService interface {
	CreateTask(ctx context.Context, input replay.CreateTaskInput) (*replay.Task, error)
	GetTask(ctx context.Context, id string) (*replay.Task, error)
	CancelTask(ctx context.Context, id string, operator string) error
}

// EventReplayServer 提供 gRPC 回放接口。
type EventReplayServer struct {
	eventfabricv1.UnimplementedEventReplayServiceServer
	service ReplayService
}

// NewEventReplayServer 创建 gRPC 回放服务。
func NewEventReplayServer(service ReplayService) *EventReplayServer {
	return &EventReplayServer{service: service}
}

func (s *EventReplayServer) CreateReplayTask(ctx context.Context, req *eventfabricv1.CreateReplayTaskRequest) (*eventfabricv1.ReplayTask, error) {
	if s.service == nil {
		return nil, status.Error(codes.FailedPrecondition, "replay service unavailable")
	}
	tenantUUID, err := tenantUUIDFromRequest(ctx, req.GetTenantUuid())
	if err != nil {
		return nil, err
	}
	start, end, err := parseWindow(req.GetWindow())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid time window: %v", err)
	}
	task, err := s.service.CreateTask(ctx, replay.CreateTaskInput{
		TenantKey:   tenantUUID,
		Topic:       req.GetTopic(),
		TraceID:     req.GetTraceId(),
		WindowStart: start,
		WindowEnd:   end,
		Reason:      req.GetReason(),
		Operator:    req.GetOperatorId(),
		Shadow:      req.GetShadow(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create replay task failed: %v", err)
	}
	return toProtoTask(task), nil
}

func (s *EventReplayServer) GetReplayTask(ctx context.Context, req *eventfabricv1.GetReplayTaskRequest) (*eventfabricv1.ReplayTask, error) {
	if s.service == nil {
		return nil, status.Error(codes.FailedPrecondition, "replay service unavailable")
	}
	task, err := s.service.GetTask(ctx, req.GetTaskId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get replay task failed: %v", err)
	}
	if task == nil {
		return nil, status.Error(codes.NotFound, "replay task not found")
	}
	return toProtoTask(task), nil
}

func (s *EventReplayServer) CancelReplayTask(ctx context.Context, req *eventfabricv1.CancelReplayTaskRequest) (*emptypb.Empty, error) {
	if s.service == nil {
		return nil, status.Error(codes.FailedPrecondition, "replay service unavailable")
	}
	if err := s.service.CancelTask(ctx, req.GetTaskId(), req.GetOperatorId()); err != nil {
		return nil, status.Errorf(codes.Internal, "cancel replay task failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func parseWindow(window *eventfabricv1.ReplayWindow) (time.Time, time.Time, error) {
	if window == nil {
		return time.Time{}, time.Time{}, nil
	}
	var start, end time.Time
	if window.GetStart() != nil {
		start = window.GetStart().AsTime()
	}
	if window.GetEnd() != nil {
		end = window.GetEnd().AsTime()
	}
	if !end.IsZero() && !start.IsZero() && start.After(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("start after end")
	}
	return start, end, nil
}

func toProtoTask(task *replay.Task) *eventfabricv1.ReplayTask {
	if task == nil {
		return nil
	}
	protoTask := &eventfabricv1.ReplayTask{
		Id:            task.ID,
		TenantUuid:    task.TenantKey,
		Topic:         task.Topic,
		TraceId:       task.TraceID,
		Status:        task.Status,
		Shadow:        task.Shadow,
		RequestedBy:   task.RequestedBy,
		FailureReason: task.FailureReason,
	}
	if !task.SubmittedAt.IsZero() {
		protoTask.SubmittedAt = timestamppb.New(task.SubmittedAt)
	}
	if task.CompletedAt != nil && !task.CompletedAt.IsZero() {
		protoTask.CompletedAt = timestamppb.New(task.CompletedAt.UTC())
	}
	return protoTask
}

// RegisterEventReplayServer 注册回放服务。
func RegisterEventReplayServer(deps *shared.Deps) Registrar {
	return func(server grpc.ServiceRegistrar) {
		if deps == nil || deps.EventFabric == nil || deps.EventFabric.Replay == nil {
			return
		}
		eventfabricv1.RegisterEventReplayServiceServer(server, NewEventReplayServer(deps.EventFabric.Replay))
	}
}
