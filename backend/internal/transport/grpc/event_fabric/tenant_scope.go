package eventfabric

import (
	"context"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func tenantUUIDFromRequest(ctx context.Context, candidate string) (string, error) {
	value := strings.TrimSpace(candidate)
	if value == "" {
		value = strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	}
	if value == "" {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			for _, key := range []string{"x-tenant-uuid", "tenant-uuid", "x-powerx-tenant-uuid"} {
				if vals := md.Get(key); len(vals) > 0 {
					if trimmed := strings.TrimSpace(vals[0]); trimmed != "" {
						value = trimmed
						break
					}
				}
			}
		}
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
