package agentgrpc

import (
	"context"
	"strings"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func requireTenantUUID(ctx context.Context, rpcCtx *commonv1.RequestContext) (string, error) {
	uuid := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if uuid == "" && rpcCtx != nil {
		if attrs := rpcCtx.GetAttributes(); attrs != nil {
			uuid = strings.TrimSpace(attrs["tenant_uuid"])
		}
	}
	if uuid == "" {
		return "", status.Error(codes.InvalidArgument, "tenant uuid is required")
	}
	canonical, err := reqctx.CanonicalTenantUUID(uuid)
	if err != nil {
		return "", status.Error(codes.InvalidArgument, "tenant uuid is invalid")
	}
	return canonical, nil
}

func requireTenantUUIDPtr(ctx context.Context, rpcCtx *commonv1.RequestContext) (*string, error) {
	uuid, err := requireTenantUUID(ctx, rpcCtx)
	if err != nil {
		return nil, err
	}
	return &uuid, nil
}
