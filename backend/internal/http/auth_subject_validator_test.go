package http

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
)

func TestValidateSTSRouteOnlyAllowsGatewayAndCoreCapabilityRoutes(t *testing.T) {
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
		"/api/v1/media/assets",
		"/api/v1/media/assets/asset-001",
		"/api/v1/media/assets/asset-001/presign",
		"/custom-prefix/media/assets/asset-001/presign",
		"/api/v1/agents/invoke",
		"/api/v1/agents/stream/sse",
		"/api/v1/agents/sessions",
		"/custom-prefix/agents/invoke",
		"/api/v1/ai/llm/invoke",
		"/api/v1/ai/llm/stream",
		"/api/v1/ai/llm/models",
		"/api/v1/ai/llm/sessions",
		"/api/v1/ai/llm/sessions/session-001/messages",
		"/api/v1/ai/image/invoke",
		"/api/v1/ai/video/invoke",
		"/api/v1/ai/tts/invoke",
		"/api/v1/ai/embedding/invoke",
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
		"/api/v1/admin/media/assets",
		"/api/v1/admin/media/assets/asset-001",
		"/api/v1/admin/media/assets/asset-001/presign",
		"/api/v1/media/assets/asset-001/resource",
		"/api/v1/media/assets/asset-001/presign/extra",
		"/api/v1/media/assets/asset-001/unknown",
		"/api/v1/admin/agents",
		"/api/v1/admin/agents/providers",
		"/api/v1/agents/sessions/session-001",
		"/api/v1/agents/sessions/session-001/messages",
		"/api/v1/agents/stream/mock",
		"/api/v1/ai/vlm/invoke",
		"/api/v1/ai/llm/sessions/session-001/stream",
		"/api/v1/ai/llm/sessions/session-001/messages/extra",
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
