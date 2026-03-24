package ops

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	platformopsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/platform_ops/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	deployops "github.com/ArtisanCloud/PowerX/internal/service/deploy_ops"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type DeployHandler struct {
	platformopsv1.UnimplementedOpsAdminServiceServer
	svc       *deployops.Service
	pluginSvc *deployops.PluginLifecycleService
}

func NewDeployHandler(deps *shared.Deps) *DeployHandler {
	if deps == nil || deps.DB == nil {
		return nil
	}
	return &DeployHandler{
		svc:       deployops.NewService(deps.DB),
		pluginSvc: deployops.NewPluginLifecycleService(deps.DB),
	}
}

func (h *DeployHandler) ListDeployReleases(ctx context.Context, req *platformopsv1.ListDeployReleasesRequest) (*platformopsv1.ListDeployReleasesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	items, total, err := h.svc.ListReleases(ctx, deployops.ListReleaseOptions{
		Environment: req.GetEnvironment(),
		Page:        int(req.GetPage()),
		PageSize:    int(req.GetPageSize()),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	out := make([]*platformopsv1.DeployReleaseRecord, 0, len(items))
	for i := range items {
		out = append(out, toPBRecord(items[i]))
	}
	return &platformopsv1.ListDeployReleasesResponse{Items: out, Total: total}, nil
}

func (h *DeployHandler) TriggerDeployRelease(ctx context.Context, req *platformopsv1.TriggerDeployReleaseRequest) (*platformopsv1.TriggerDeployReleaseResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	mode, tickets := parseModeAndTickets(req.GetEnvironment())
	row, err := h.svc.TriggerRelease(ctx, deployops.ReleaseRequest{
		Environment:     req.GetEnvironment(),
		BackendVersion:  req.GetBackendVersion(),
		WebAdminVersion: req.GetWebAdminVersion(),
		Mode:            mode,
		Operator:        resolveOperator(ctx),
		TraceID:         strings.TrimSpace(reqctx.GetTraceID(ctx)),
		ApprovalTickets: tickets,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &platformopsv1.TriggerDeployReleaseResponse{ReleaseId: strconv.FormatUint(uint64(row.ID), 10)}, nil
}

func (h *DeployHandler) TriggerDeployRollback(ctx context.Context, req *platformopsv1.TriggerDeployRollbackRequest) (*platformopsv1.TriggerDeployRollbackResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	mode, tickets := parseModeAndTickets(req.GetEnvironment())
	row, err := h.svc.TriggerRollback(ctx, deployops.RollbackRequest{
		Environment:     req.GetEnvironment(),
		TargetVersion:   req.GetTargetVersion(),
		Mode:            mode,
		Operator:        resolveOperator(ctx),
		TraceID:         strings.TrimSpace(reqctx.GetTraceID(ctx)),
		ApprovalTickets: tickets,
	})
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &platformopsv1.TriggerDeployRollbackResponse{ReleaseId: strconv.FormatUint(uint64(row.ID), 10)}, nil
}

func (h *DeployHandler) GetDeployHealth(ctx context.Context, _ *platformopsv1.GetDeployHealthRequest) (*platformopsv1.GetDeployHealthResponse, error) {
	health, err := h.svc.GetHealth(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &platformopsv1.GetDeployHealthResponse{Status: health.Status, Summary: health.Summary}, nil
}

func toPBRecord(in modelops.DeployReleaseRecord) *platformopsv1.DeployReleaseRecord {
	return &platformopsv1.DeployReleaseRecord{
		Id:              strconv.FormatUint(uint64(in.ID), 10),
		Environment:     in.Environment,
		BackendVersion:  in.BackendVersion,
		WebAdminVersion: in.WebAdminVersion,
		Action:          string(in.Action),
		Status:          string(in.Status),
		Operator:        in.Operator,
		TraceId:         in.TraceID,
		StartedAt:       formatTime(in.StartedAt),
		EndedAt:         formatTime(in.EndedAt),
		ErrorMessage:    in.ErrorMessage,
	}
}

func formatTime(v *time.Time) string {
	if v == nil || v.IsZero() {
		return ""
	}
	return v.UTC().Format(time.RFC3339)
}

func resolveOperator(ctx context.Context) string {
	if reqctx.IsRoot(ctx) {
		return "root"
	}
	if reqctx.GetMemberID(ctx) > 0 {
		return "member"
	}
	return "system"
}

func parseModeAndTickets(environment string) (mode string, tickets int) {
	env := strings.TrimSpace(strings.ToLower(environment))
	if strings.Contains(env, ":") {
		parts := strings.SplitN(env, ":", 2)
		if len(parts) == 2 {
			mode = strings.TrimSpace(parts[1])
		}
	}
	if mode == "" {
		mode = deployops.DeployModeDocker
	}
	if strings.HasPrefix(env, "prod") {
		tickets = 2
	}
	return mode, tickets
}

func mapServiceError(err error) error {
	switch {
	case errors.Is(err, deployops.ErrInvalidDeployRequest), errors.Is(err, deployops.ErrInvalidDeployMode):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, deployops.ErrReleaseInProgress):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, deployops.ErrApprovalRequired):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
