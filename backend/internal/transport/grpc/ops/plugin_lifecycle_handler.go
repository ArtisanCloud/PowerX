package ops

import (
	"context"
	"errors"
	"strconv"
	"strings"

	platformopsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/platform_ops/v1"
	deployops "github.com/ArtisanCloud/PowerX/internal/service/deploy_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *DeployHandler) ListPluginLifecycleAudits(ctx context.Context, req *platformopsv1.ListPluginLifecycleAuditsRequest) (*platformopsv1.ListPluginLifecycleAuditsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	items, total, err := h.pluginSvc.ListAudits(ctx, deployops.PluginLifecycleListOptions{
		PluginID: req.GetPluginId(),
		Page:     int(req.GetPage()),
		PageSize: int(req.GetPageSize()),
	})
	if err != nil {
		return nil, mapPluginServiceError(err)
	}

	rows := make([]*platformopsv1.PluginLifecycleAudit, 0, len(items))
	for i := range items {
		rows = append(rows, &platformopsv1.PluginLifecycleAudit{
			Id:          strconv.FormatUint(uint64(items[i].ID), 10),
			PluginId:    items[i].PluginID,
			FromVersion: items[i].FromVersion,
			ToVersion:   items[i].ToVersion,
			Action:      string(items[i].Action),
			Result:      string(items[i].Result),
			GateResult:  items[i].GateResult,
			GateReason:  items[i].GateReason,
			Operator:    items[i].Operator,
			TraceId:     items[i].TraceID,
			Detail:      items[i].Detail,
		})
	}
	return &platformopsv1.ListPluginLifecycleAuditsResponse{Items: rows, Total: total}, nil
}

func (h *DeployHandler) TriggerPluginLifecycleAction(ctx context.Context, req *platformopsv1.TriggerPluginLifecycleActionRequest) (*platformopsv1.TriggerPluginLifecycleActionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	row, err := h.pluginSvc.TriggerAction(ctx, deployops.PluginLifecycleActionRequest{
		PluginID:    req.GetPluginId(),
		FromVersion: req.GetFromVersion(),
		ToVersion:   req.GetToVersion(),
		Action:      req.GetAction(),
		Reason:      req.GetReason(),
		Operator:    resolveOperator(ctx),
		TraceID:     strings.TrimSpace(reqctx.GetTraceID(ctx)),
	})
	if err != nil {
		return nil, mapPluginServiceError(err)
	}
	return &platformopsv1.TriggerPluginLifecycleActionResponse{AuditId: strconv.FormatUint(uint64(row.ID), 10)}, nil
}

func mapPluginServiceError(err error) error {
	switch {
	case errors.Is(err, deployops.ErrInvalidPluginLifecycleRequest), errors.Is(err, deployops.ErrUnsupportedPluginAction):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
