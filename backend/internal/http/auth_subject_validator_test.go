package http

import (
	"context"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
)

func TestSTSAllowedHTTPRoutesRequireExplicitPolicy(t *testing.T) {
	routes := stsAllowedHTTPRoutes()
	if len(routes) <= len(stsStaticAllowedHTTPRoutes) {
		t.Fatalf("STS route policy did not load platform capability routes: routes=%d static=%d", len(routes), len(stsStaticAllowedHTTPRoutes))
	}
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		if strings.TrimSpace(route.Method) == "" {
			t.Fatalf("STS route policy missing method: %#v", route)
		}
		if strings.TrimSpace(route.Pattern) == "" {
			t.Fatalf("STS route policy missing pattern: %#v", route)
		}
		switch route.Match {
		case stsRouteMatchSuffix, stsRouteMatchCorePattern:
		default:
			t.Fatalf("STS route policy has unsupported match mode: %#v", route)
		}
		if route.Match == stsRouteMatchCorePattern && strings.HasPrefix(strings.TrimSpace(route.Pattern), "/api/") {
			t.Fatalf("STS core route policy must be normalized without api prefix: %#v", route)
		}
		key := strings.ToUpper(strings.TrimSpace(route.Method)) + " " + strings.TrimSpace(route.Pattern) + " " + string(route.Match)
		if _, ok := seen[key]; ok {
			t.Fatalf("STS route policy duplicated: %s", key)
		}
		seen[key] = struct{}{}
	}
}

func TestValidateSTSRouteOnlyAllowsGatewayAndCoreCapabilityRoutes(t *testing.T) {
	claims := &reqctx.CoreXClaims{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "powerx-sts",
			Audience: jwt.ClaimStrings{"powerx:api"},
		},
	}

	for _, tt := range []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/tenant/invocations"},
		{"POST", "/api/v1/tenant/invocations/stream"},
		{"GET", "/api/v1/tenant/iam/roles"},
		{"POST", "/api/v1/tenant/iam/roles/provision"},
		{"POST", "/api/v1/tenant/iam/members/provision"},
		{"POST", "/api/v1/admin/runtime/ws-bus/grant"},
		{"POST", "/api/v1/admin/runtime/ws-bus/publish"},
		{"POST", "/api/v1/admin/runtime/task-queue/enqueue"},
		{"POST", "/api/v1/admin/runtime/task-queue/dequeue"},
		{"POST", "/api/v1/admin/runtime/task-queue/ack"},
		{"POST", "/api/v1/admin/runtime/task-queue/nack"},
		{"POST", "/api/v1/admin/runtime/task-queue/retry"},
		{"POST", "/api/v1/notifications/test"},
		{"GET", "/api/v1/media/assets"},
		{"POST", "/api/v1/media/assets"},
		{"GET", "/api/v1/media/assets/asset-001"},
		{"PATCH", "/api/v1/media/assets/asset-001"},
		{"DELETE", "/api/v1/media/assets/asset-001"},
		{"POST", "/api/v1/media/assets/asset-001/presign"},
		{"POST", "/custom-prefix/media/assets/asset-001/presign"},
		{"POST", "/api/v1/media/assets/asset-001/variants/thumb"},
		{"PUT", "/api/v1/media/assets/asset-001/variants/thumb"},
		{"POST", "/api/v1/media/assets/asset-001/variants/thumb/presign"},
		{"GET", "/api/v1/media/assets/asset-001/variants/thumb/resource"},
		{"POST", "/api/v1/agents/invoke"},
		{"GET", "/api/v1/agents/stream/sse"},
		{"POST", "/api/v1/agents/sessions"},
		{"POST", "/custom-prefix/agents/invoke"},
		{"POST", "/api/v1/ai/llm/invoke"},
		{"POST", "/api/v1/ai/llm/stream"},
		{"GET", "/api/v1/ai/llm/models"},
		{"POST", "/api/v1/ai/llm/sessions"},
		{"POST", "/api/v1/ai/llm/sessions/session-001/messages"},
		{"POST", "/api/v1/ai/image/invoke"},
		{"POST", "/api/v1/ai/video/invoke"},
		{"POST", "/api/v1/ai/tts/invoke"},
		{"POST", "/api/v1/ai/embedding/invoke"},
		{"POST", "/api/v1/ai/vlm/invoke"},
		{"POST", "/custom-prefix/ai/llm/invoke"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			ctx := reqctx.WithRequestPath(context.Background(), tt.path)
			ctx = reqctx.WithRequestMethod(ctx, tt.method)
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

	for _, tt := range []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/admin/iam/members"},
		{"POST", "/api/v1/tenant/iam/roles"},
		{"GET", "/api/v1/tenant/iam/members/provision"},
		{"POST", "/api/v1/admin/iam/members"},
		{"GET", "/api/v1/admin/ai/settings"},
		{"GET", "/api/v1/admin/media/assets"},
		{"GET", "/api/v1/admin/media/assets/asset-001"},
		{"POST", "/api/v1/admin/media/assets/asset-001/presign"},
		{"GET", "/api/v1/media/assets/asset-001/resource"},
		{"POST", "/api/v1/media/assets/asset-001/presign/extra"},
		{"GET", "/api/v1/media/assets/asset-001/unknown"},
		{"GET", "/api/v1/admin/agents"},
		{"GET", "/api/v1/admin/agents/providers"},
		{"GET", "/api/v1/agents/sessions/session-001"},
		{"POST", "/api/v1/agents/sessions/session-001/messages"},
		{"GET", "/api/v1/agents/stream/mock"},
		{"GET", "/api/v1/ai/llm/sessions/session-001/stream"},
		{"POST", "/api/v1/ai/llm/sessions/session-001/messages/extra"},
		{"POST", "/api/v1/some-ai-like/path"},
		{"GET", "/api/v1/tenant/invocations"},
		{"GET", "/api/v1/admin/runtime/task-queue/enqueue"},
		{"POST", "/api/v1/admin/runtime/task-queue/unknown"},
		{"POST", "/api/v1/admin/tenants"},
		{"GET", "/api/v1/admin/scheduler/jobs"},
		{"GET", "/api/v1/internal/plugins/templates"},
		{"POST", "/api/v1/public/saas/signup"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			ctx := reqctx.WithRequestPath(context.Background(), tt.path)
			ctx = reqctx.WithRequestMethod(ctx, tt.method)
			ctx = reqctx.WithTenantUUID(ctx, claims.TenantUUID)
			if err := validateSTSRouteOnly(ctx, claims); err == nil {
				t.Fatal("validateSTSRouteOnly() err = nil, want rejection")
			}
		})
	}
}

func TestValidateSTSRouteOnlyDoesNotBlockUserJWTAdminRoutes(t *testing.T) {
	claims := &reqctx.CoreXClaims{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		UserID:     100,
		MemberID:   200,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "powerx",
			Audience: jwt.ClaimStrings{"user"},
		},
	}

	for _, tt := range []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/admin/iam/members"},
		{"POST", "/api/v1/admin/iam/roles"},
		{"GET", "/api/v1/admin/plugins"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			ctx := reqctx.WithRequestPath(context.Background(), tt.path)
			ctx = reqctx.WithRequestMethod(ctx, tt.method)
			ctx = reqctx.WithTenantUUID(ctx, claims.TenantUUID)
			if err := validateSTSRouteOnly(ctx, claims); err != nil {
				t.Fatalf("validateSTSRouteOnly() err = %v", err)
			}
		})
	}
}

func TestValidateSTSRouteOnlyAllowsTenantLookupGetOnly(t *testing.T) {
	claims := &reqctx.CoreXClaims{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "powerx-sts",
			Audience: jwt.ClaimStrings{"powerx:api"},
		},
	}

	ctx := reqctx.WithRequestPath(context.Background(), "/api/v1/admin/tenants")
	ctx = reqctx.WithRequestMethod(ctx, "GET")
	ctx = reqctx.WithTenantUUID(ctx, claims.TenantUUID)
	if err := validateSTSRouteOnly(ctx, claims); err != nil {
		t.Fatalf("validateSTSRouteOnly() err = %v", err)
	}
}

func TestValidateSTSRouteOnlyRejectsTenantMutationMethods(t *testing.T) {
	claims := &reqctx.CoreXClaims{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "powerx-sts",
			Audience: jwt.ClaimStrings{"powerx:api"},
		},
	}

	for _, method := range []string{"", "POST", "PUT", "PATCH", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			ctx := reqctx.WithRequestPath(context.Background(), "/api/v1/admin/tenants")
			ctx = reqctx.WithRequestMethod(ctx, method)
			ctx = reqctx.WithTenantUUID(ctx, claims.TenantUUID)
			if err := validateSTSRouteOnly(ctx, claims); err == nil {
				t.Fatal("validateSTSRouteOnly() err = nil, want rejection")
			}
		})
	}
}
