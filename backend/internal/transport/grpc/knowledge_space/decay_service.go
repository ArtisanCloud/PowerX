package knowledge_space

import (
	"context"
	"errors"
	"strings"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	decay_guard "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/decay_guard"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func toProtoDecayTasks(tasks []*models.DecayTask) []*knowledgev1.DecayTask {
	if len(tasks) == 0 {
		return []*knowledgev1.DecayTask{}
	}
	result := make([]*knowledgev1.DecayTask, 0, len(tasks))
	for _, task := range tasks {
		if dto := toProtoDecayTask(task); dto != nil {
			result = append(result, dto)
		}
	}
	return result
}

func toProtoDecayTask(task *models.DecayTask) *knowledgev1.DecayTask {
	if task == nil {
		return nil
	}
	return &knowledgev1.DecayTask{
		TaskId:        task.UUID.String(),
		SpaceId:       task.SpaceUUID.String(),
		Category:      task.Category,
		Severity:      task.Severity,
		Status:        task.Status,
		DetectedAt:    timestampValue(task.DetectedAt),
		SlaDueAt:      timestampValue(task.SLADueAt),
		FalsePositive: task.FalsePositive,
	}
}

func mapDecayError(err error) error {
	switch {
	case errors.Is(err, decay_guard.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, decay_guard.ErrTaskNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func (s *Server) RunDecayScan(ctx context.Context, req *knowledgev1.RunDecayScanRequest) (*knowledgev1.RunDecayScanResponse, error) {
	if s.decay == nil {
		return nil, status.Error(codes.Unimplemented, "decay service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	tasks, err := s.decay.RunScan(ctx, spaceID, int(req.GetDetected()))
	if err != nil {
		return nil, mapDecayError(err)
	}
	return &knowledgev1.RunDecayScanResponse{Tasks: toProtoDecayTasks(tasks)}, nil
}

func (s *Server) ListDecayTasks(ctx context.Context, req *knowledgev1.ListDecayTasksRequest) (*knowledgev1.ListDecayTasksResponse, error) {
	if s.decay == nil {
		return nil, status.Error(codes.Unimplemented, "decay service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	tasks, err := s.decay.ListOpen(ctx, spaceID)
	if err != nil {
		return nil, mapDecayError(err)
	}
	return &knowledgev1.ListDecayTasksResponse{Tasks: toProtoDecayTasks(tasks)}, nil
}

func (s *Server) RestoreDecayTask(ctx context.Context, req *knowledgev1.RestoreDecayTaskRequest) (*knowledgev1.RestoreDecayTaskResponse, error) {
	if s.decay == nil {
		return nil, status.Error(codes.Unimplemented, "decay service not available")
	}
	taskID, err := uuid.Parse(strings.TrimSpace(req.GetTaskId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid task id: %v", err)
	}
	task, err := s.decay.Restore(ctx, taskID, req.GetNotes(), req.GetFalsePositive())
	if err != nil {
		return nil, mapDecayError(err)
	}
	return &knowledgev1.RestoreDecayTaskResponse{Task: toProtoDecayTask(task)}, nil
}

