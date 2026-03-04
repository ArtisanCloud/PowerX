package eventfabric

import (
	"context"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func tenantUUIDFromRequest(ctx context.Context, candidate string) (string, error) {
	value := strings.TrimSpace(candidate)
	if value == "" {
		value = strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	}
	if value == "" {
		return "", status.Error(codes.InvalidArgument, "tenant uuid required")
	}
	canonical, err := reqctx.CanonicalTenantUUID(value)
	if err != nil {
		return "", status.Error(codes.InvalidArgument, "tenant uuid invalid")
	}
	return canonical, nil
}
