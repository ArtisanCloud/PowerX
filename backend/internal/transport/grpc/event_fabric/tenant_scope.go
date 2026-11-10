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
	trusted := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if reqctx.GetClaims(ctx) == nil || trusted == "" {
		return "", status.Error(codes.Unauthenticated, "authenticated tenant required")
	}
	// 管理 RPC 不能成为服务凭据自行授予 topic ACL 的旁路。
	// 插件创建自身主题使用正式 topic_bootstrap；发布/消费走独立服务合同。
	c := reqctx.GetClaims(ctx)
	if c.Issuer == "powerx-sts" || c.Scope == "api_key" {
		return "", status.Error(codes.PermissionDenied, "service credentials cannot manage Event Fabric ACL")
	}
	for _, platform := range c.Platforms {
		if platform == "api_key" {
			return "", status.Error(codes.PermissionDenied, "service credentials cannot manage Event Fabric ACL")
		}
	}
	if value == "" {
		value = trusted
	}
	if value != trusted {
		return "", status.Error(codes.PermissionDenied, "tenant mismatch")
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
