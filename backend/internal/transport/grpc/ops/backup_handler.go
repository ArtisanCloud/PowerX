package ops

import (
	"context"
	"errors"
	"strconv"
	"strings"

	platformopsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/platform_ops/v1"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *DeployHandler) ListBackupPolicies(ctx context.Context, req *platformopsv1.ListBackupPoliciesRequest) (*platformopsv1.ListBackupPoliciesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	items, _, err := h.policySvc.ListPolicies(ctx, backupops.ListPolicyOptions{EnabledOnly: req.GetEnabledOnly(), Page: 1, PageSize: 200})
	if err != nil {
		return nil, mapBackupError(err)
	}

	out := make([]*platformopsv1.BackupPolicy, 0, len(items))
	for i := range items {
		out = append(out, &platformopsv1.BackupPolicy{
			Id:            strconv.FormatUint(uint64(items[i].ID), 10),
			Name:          items[i].Name,
			BackupType:    string(items[i].BackupType),
			Schedule:      items[i].Schedule,
			RetentionDays: items[i].RetentionDays,
			Enabled:       items[i].Enabled,
			StorageTarget: items[i].StorageTarget,
		})
	}
	return &platformopsv1.ListBackupPoliciesResponse{Items: out}, nil
}

func (h *DeployHandler) UpsertBackupPolicy(ctx context.Context, req *platformopsv1.UpsertBackupPolicyRequest) (*platformopsv1.UpsertBackupPolicyResponse, error) {
	if req == nil || req.GetPolicy() == nil {
		return nil, status.Error(codes.InvalidArgument, "policy is required")
	}
	row, err := h.policySvc.UpsertPolicy(ctx, backupops.UpsertPolicyRequest{
		Name:          req.GetPolicy().GetName(),
		BackupType:    req.GetPolicy().GetBackupType(),
		Schedule:      req.GetPolicy().GetSchedule(),
		RetentionDays: int(req.GetPolicy().GetRetentionDays()),
		Enabled:       req.GetPolicy().GetEnabled(),
		StorageTarget: req.GetPolicy().GetStorageTarget(),
		Operator:      resolveOperator(ctx),
		TraceID:       strings.TrimSpace(reqctx.GetTraceID(ctx)),
	})
	if err != nil {
		return nil, mapBackupError(err)
	}
	return &platformopsv1.UpsertBackupPolicyResponse{PolicyId: strconv.FormatUint(uint64(row.ID), 10)}, nil
}

func (h *DeployHandler) TriggerBackupJob(ctx context.Context, req *platformopsv1.TriggerBackupJobRequest) (*platformopsv1.TriggerBackupJobResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	policyID, _ := strconv.ParseUint(strings.TrimSpace(req.GetPolicyId()), 10, 64)
	row, err := h.backupSvc.TriggerJob(ctx, backupops.TriggerJobRequest{PolicyID: policyID, Operator: resolveOperator(ctx), TraceID: strings.TrimSpace(reqctx.GetTraceID(ctx))})
	if err != nil {
		return nil, mapBackupError(err)
	}
	return &platformopsv1.TriggerBackupJobResponse{JobId: strconv.FormatUint(uint64(row.ID), 10)}, nil
}

func (h *DeployHandler) ListBackupJobs(ctx context.Context, req *platformopsv1.ListBackupJobsRequest) (*platformopsv1.ListBackupJobsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	policyID, _ := strconv.ParseUint(strings.TrimSpace(req.GetPolicyId()), 10, 64)
	items, total, err := h.backupSvc.ListJobs(ctx, backupops.ListJobOptions{PolicyID: policyID, Page: int(req.GetPage()), PageSize: int(req.GetPageSize())})
	if err != nil {
		return nil, mapBackupError(err)
	}

	out := make([]*platformopsv1.BackupJob, 0, len(items))
	for i := range items {
		out = append(out, &platformopsv1.BackupJob{
			Id:           strconv.FormatUint(uint64(items[i].ID), 10),
			PolicyId:     strconv.FormatUint(items[i].PolicyID, 10),
			Status:       string(items[i].Status),
			TriggerType:  string(items[i].TriggerType),
			StartedAt:    formatTime(items[i].StartedAt),
			EndedAt:      formatTime(items[i].EndedAt),
			ErrorMessage: items[i].ErrorMessage,
			Operator:     items[i].Operator,
			TraceId:      items[i].TraceID,
		})
	}
	return &platformopsv1.ListBackupJobsResponse{Items: out, Total: total}, nil
}

func (h *DeployHandler) TriggerRestoreDrill(ctx context.Context, req *platformopsv1.TriggerRestoreDrillRequest) (*platformopsv1.TriggerRestoreDrillResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	sourceJobID, _ := strconv.ParseUint(strings.TrimSpace(req.GetSourceJobId()), 10, 64)
	row, err := h.restoreSvc.Trigger(ctx, backupops.TriggerRestoreDrillRequest{SourceJobID: sourceJobID, Operator: resolveOperator(ctx), TraceID: strings.TrimSpace(reqctx.GetTraceID(ctx))})
	if err != nil {
		return nil, mapBackupError(err)
	}
	return &platformopsv1.TriggerRestoreDrillResponse{DrillId: strconv.FormatUint(uint64(row.ID), 10)}, nil
}

func mapBackupError(err error) error {
	switch {
	case errors.Is(err, backupops.ErrInvalidBackupPolicy), errors.Is(err, backupops.ErrInvalidBackupRequest), errors.Is(err, backupops.ErrInvalidRestoreDrillRequest):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, backupops.ErrBackupPolicyNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
