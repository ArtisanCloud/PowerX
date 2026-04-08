package ops

import (
	"context"
	"errors"
	"strconv"
	"strings"

	platformopsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/platform_ops/v1"
	migrationops "github.com/ArtisanCloud/PowerX/internal/service/migration_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *DeployHandler) TriggerInstanceMigration(ctx context.Context, req *platformopsv1.TriggerInstanceMigrationRequest) (*platformopsv1.TriggerInstanceMigrationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	row, err := h.migrateSvc.TriggerMigration(ctx, migrationops.TriggerRequest{
		SourceEnv: req.GetSourceEnv(),
		TargetEnv: req.GetTargetEnv(),
		DryRun:    req.GetDryRun(),
		Operator:  resolveOperator(ctx),
		TraceID:   strings.TrimSpace(reqctx.GetTraceID(ctx)),
	})
	if err != nil {
		return nil, mapMigrationError(err)
	}
	return &platformopsv1.TriggerInstanceMigrationResponse{MigrationId: strconv.FormatUint(row.ID, 10)}, nil
}

func (h *DeployHandler) GetInstanceMigration(ctx context.Context, req *platformopsv1.GetInstanceMigrationRequest) (*platformopsv1.GetInstanceMigrationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	migrationID, _ := strconv.ParseUint(strings.TrimSpace(req.GetMigrationId()), 10, 64)
	row, err := h.migrateSvc.GetMigration(ctx, migrationID)
	if err != nil {
		return nil, mapMigrationError(err)
	}
	return &platformopsv1.GetInstanceMigrationResponse{
		MigrationId: strconv.FormatUint(row.ID, 10),
		Status:      string(row.Status),
		Summary:     row.Summary,
	}, nil
}

func (h *DeployHandler) TriggerTrafficSwitch(ctx context.Context, req *platformopsv1.TriggerTrafficSwitchRequest) (*platformopsv1.TriggerTrafficSwitchResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	migrationID, _ := strconv.ParseUint(strings.TrimSpace(req.GetMigrationId()), 10, 64)
	operationID, _, err := h.migrateSvc.TriggerTrafficSwitch(ctx, migrationops.SwitchRequest{
		MigrationID: migrationID,
		Rollback:    req.GetRollback(),
		Operator:    resolveOperator(ctx),
		TraceID:     strings.TrimSpace(reqctx.GetTraceID(ctx)),
	})
	if err != nil {
		return nil, mapMigrationError(err)
	}
	return &platformopsv1.TriggerTrafficSwitchResponse{OperationId: operationID}, nil
}

func mapMigrationError(err error) error {
	switch {
	case errors.Is(err, migrationops.ErrInvalidMigrationRequest):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, migrationops.ErrMigrationNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, migrationops.ErrMigrationNotReady):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
