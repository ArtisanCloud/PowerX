package scheduler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	schedulerv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/scheduler/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	runtimescheduler "github.com/ArtisanCloud/PowerX/internal/service/runtime_scheduler"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	schedulerv1.UnimplementedSchedulerServiceServer

	svc *runtimescheduler.Service
}

func NewServer(deps *shared.Deps) *Server {
	if deps == nil {
		return nil
	}
	if deps.RuntimeScheduler != nil && deps.RuntimeScheduler.Service != nil {
		return &Server{svc: deps.RuntimeScheduler.Service}
	}
	if deps.DB == nil {
		return nil
	}
	return &Server{svc: runtimescheduler.NewService(runtimescheduler.Options{DB: deps.DB, EventBus: deps.EventBus})}
}

func RegisterServer(s grpc.ServiceRegistrar, deps *shared.Deps) {
	server := NewServer(deps)
	if server == nil {
		return
	}
	schedulerv1.RegisterSchedulerServiceServer(s, server)
}

func (s *Server) CreateJob(ctx context.Context, req *schedulerv1.CreateJobRequest) (*schedulerv1.CreateJobResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Unavailable, "scheduler service unavailable")
	}
	jobReq := req.GetJob()
	if jobReq == nil {
		return nil, status.Error(codes.InvalidArgument, "job is required")
	}
	payload, err := decodePayload(jobReq.GetPayloadJson())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "payload_json must be a JSON object")
	}
	job, err := s.svc.CreateJob(ctx, runtimescheduler.JobSpec{
		TenantUUID:   jobReq.GetTenantUuid(),
		OwnerType:    jobReq.GetOwnerType(),
		OwnerID:      jobReq.GetOwnerId(),
		Name:         jobReq.GetName(),
		ScheduleType: jobReq.GetScheduleType(),
		ScheduleExpr: jobReq.GetScheduleExpr(),
		Timezone:     jobReq.GetTimezone(),
		Payload:      payload,
	}, operatorFromContext(ctx), reqctx.GetTraceID(ctx))
	if err != nil {
		return nil, grpcError(err)
	}
	return &schedulerv1.CreateJobResponse{Job: toPBJob(job)}, nil
}

func (s *Server) UpdateJob(ctx context.Context, req *schedulerv1.UpdateJobRequest) (*schedulerv1.UpdateJobResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Unavailable, "scheduler service unavailable")
	}
	jobReq := req.GetJob()
	if jobReq == nil {
		return nil, status.Error(codes.InvalidArgument, "job is required")
	}
	payload, err := decodePayload(jobReq.GetPayloadJson())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "payload_json must be a JSON object")
	}
	name := optionalString(jobReq.GetName())
	scheduleType := optionalString(jobReq.GetScheduleType())
	scheduleExpr := optionalString(jobReq.GetScheduleExpr())
	timezone := optionalString(jobReq.GetTimezone())
	job, err := s.svc.UpdateJob(ctx, runtimescheduler.UpdateJobInput{
		JobID:        jobReq.GetJobId(),
		Name:         name,
		ScheduleType: scheduleType,
		ScheduleExpr: scheduleExpr,
		Timezone:     timezone,
		Payload:      payload,
		Operator:     operatorFromContext(ctx),
		TraceID:      reqctx.GetTraceID(ctx),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &schedulerv1.UpdateJobResponse{Job: toPBJob(job)}, nil
}

func (s *Server) PauseJob(ctx context.Context, req *schedulerv1.PauseJobRequest) (*schedulerv1.PauseJobResponse, error) {
	job, err := s.svc.PauseJob(withTenantFromRequest(ctx, req.GetTenantUuid()), req.GetJobId(), operatorFromContext(ctx), reqctx.GetTraceID(ctx))
	if err != nil {
		return nil, grpcError(err)
	}
	return &schedulerv1.PauseJobResponse{Job: toPBJob(job)}, nil
}

func (s *Server) ResumeJob(ctx context.Context, req *schedulerv1.ResumeJobRequest) (*schedulerv1.ResumeJobResponse, error) {
	job, err := s.svc.ResumeJob(withTenantFromRequest(ctx, req.GetTenantUuid()), req.GetJobId(), operatorFromContext(ctx), reqctx.GetTraceID(ctx))
	if err != nil {
		return nil, grpcError(err)
	}
	return &schedulerv1.ResumeJobResponse{Job: toPBJob(job)}, nil
}

func (s *Server) TriggerJob(ctx context.Context, req *schedulerv1.TriggerJobRequest) (*schedulerv1.TriggerJobResponse, error) {
	result, err := s.svc.TriggerJob(withTenantFromRequest(ctx, req.GetTenantUuid()), req.GetJobId(), operatorFromContext(ctx), reqctx.GetTraceID(ctx))
	if err != nil {
		return nil, grpcError(err)
	}
	return &schedulerv1.TriggerJobResponse{Job: toPBJob(result.Job)}, nil
}

func (s *Server) GetJob(ctx context.Context, req *schedulerv1.GetJobRequest) (*schedulerv1.GetJobResponse, error) {
	job, err := s.svc.GetJob(withTenantFromRequest(ctx, req.GetTenantUuid()), req.GetJobId())
	if err != nil {
		return nil, grpcError(err)
	}
	return &schedulerv1.GetJobResponse{Job: toPBJob(job)}, nil
}

func (s *Server) ListJobs(ctx context.Context, req *schedulerv1.ListJobsRequest) (*schedulerv1.ListJobsResponse, error) {
	pageSize := int(req.GetLimit())
	if pageSize <= 0 {
		pageSize = 50
	}
	jobs, _, err := s.svc.ListJobs(withTenantFromRequest(ctx, req.GetTenantUuid()), runtimescheduler.ListJobsInput{
		TenantUUID: req.GetTenantUuid(),
		Page:       1,
		PageSize:   pageSize,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	items := make([]*schedulerv1.SchedulerJob, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, toPBJob(job))
	}
	return &schedulerv1.ListJobsResponse{Jobs: items}, nil
}

func decodePayload(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func optionalString(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func withTenantFromRequest(ctx context.Context, tenantUUID string) context.Context {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" || strings.TrimSpace(reqctx.GetTenantUUID(ctx)) != "" {
		return ctx
	}
	return reqctx.WithTenantUUID(ctx, tenantUUID)
}

func operatorFromContext(ctx context.Context) string {
	if subject := strings.TrimSpace(reqctx.GetSubject(ctx)); subject != "" {
		return subject
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil {
		return ""
	}
	if claims.MemberUUID != "" {
		return claims.MemberUUID
	}
	if claims.UserUUID != "" {
		return claims.UserUUID
	}
	return ""
}

func toPBJob(job *models.SchedulerJob) *schedulerv1.SchedulerJob {
	if job == nil {
		return nil
	}
	return &schedulerv1.SchedulerJob{
		JobId:        job.UUID.String(),
		TenantUuid:   job.TenantUUID,
		OwnerType:    job.OwnerType,
		OwnerId:      job.OwnerID,
		Name:         job.Name,
		ScheduleType: job.ScheduleType,
		ScheduleExpr: job.ScheduleExpr,
		Timezone:     job.Timezone,
		PayloadJson:  []byte(job.PayloadJSON),
		Status:       job.Status,
		NextRunAt:    formatTime(job.NextRunAt),
		LastRunAt:    formatTime(job.LastRunAt),
	}
}

func formatTime(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.UTC().Format(time.RFC3339Nano)
}

func grpcError(err error) error {
	if err == nil {
		return nil
	}
	msg := dto.MessageOf(err)
	if msg == "" {
		msg = err.Error()
	}
	switch dto.StatusCode(err) {
	case http.StatusBadRequest:
		return status.Error(codes.InvalidArgument, msg)
	case http.StatusUnauthorized:
		return status.Error(codes.Unauthenticated, msg)
	case http.StatusForbidden:
		return status.Error(codes.PermissionDenied, msg)
	case http.StatusNotFound, http.StatusGone:
		return status.Error(codes.NotFound, msg)
	case http.StatusConflict:
		return status.Error(codes.FailedPrecondition, msg)
	case http.StatusServiceUnavailable:
		return status.Error(codes.Unavailable, msg)
	default:
		return status.Error(codes.Internal, msg)
	}
}
