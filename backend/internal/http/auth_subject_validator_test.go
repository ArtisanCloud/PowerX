package http

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
)

func TestValidateSTSRouteOnlyAllowsGatewayAndAICapabilityRoutes(t *testing.T) {
	claims := &reqctx.CoreXClaims{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "powerx-sts",
			Audience: jwt.ClaimStrings{"powerx:api"},
		},
	}

	for _, path := range []string{
		"/api/v1/tenant/invocations",
		"/api/v1/tenant/invocations/stream",
		"/api/v1/admin/runtime/ws-bus/grant",
		"/api/v1/admin/runtime/ws-bus/publish",
		"/api/v1/notifications/test",
		"/api/v1/ai/llm/invoke",
		"/api/v1/ai/llm/models",
		"/custom-prefix/ai/llm/invoke",
	} {
		t.Run(path, func(t *testing.T) {
			ctx := reqctx.WithRequestPath(context.Background(), path)
			ctx = reqctx.WithTenantUUID(ctx, claims.TenantUUID)
			if err := validateSTSRouteOnly(ctx, claims); err != nil {
				t.Fatalf("validateSTSRouteOnly() err = %v", err)
			}
		})
	}
}

func TestValidateSTSRouteOnlyRejectsNonGatewayRoutes(t *testing.T) {
	claims := &reqctx.CoreXClaims{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "powerx-sts",
			Audience: jwt.ClaimStrings{"powerx:api"},
		},
	}

	for _, path := range []string{
		"/api/v1/admin/iam/members",
		"/api/v1/admin/ai/settings",
		"/api/v1/some-ai-like/path",
	} {
		t.Run(path, func(t *testing.T) {
			ctx := reqctx.WithRequestPath(context.Background(), path)
			ctx = reqctx.WithTenantUUID(ctx, claims.TenantUUID)
			if err := validateSTSRouteOnly(ctx, claims); err == nil {
				t.Fatal("validateSTSRouteOnly() err = nil, want rejection")
			}
		})
	}
}
