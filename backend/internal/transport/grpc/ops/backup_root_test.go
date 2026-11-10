package ops

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// 直接调用传输边界，nil Service 保证拒绝发生在任何数据库／执行器调用前。
// JWT 签名和 issuer 验证仍属于统一 gRPC 拦截器；本测试不冒充完整网络验收。
func TestLegacyGRPCBackupMethodsRequirePlatformRoot(t *testing.T) {
	h := &DeployHandler{}
	methods := map[string]func(context.Context) error{
		"ListBackupPolicies":  func(ctx context.Context) error { _, e := h.ListBackupPolicies(ctx, nil); return e },
		"UpsertBackupPolicy":  func(ctx context.Context) error { _, e := h.UpsertBackupPolicy(ctx, nil); return e },
		"TriggerBackupJob":    func(ctx context.Context) error { _, e := h.TriggerBackupJob(ctx, nil); return e },
		"ListBackupJobs":      func(ctx context.Context) error { _, e := h.ListBackupJobs(ctx, nil); return e },
		"TriggerRestoreDrill": func(ctx context.Context) error { _, e := h.TriggerRestoreDrill(ctx, nil); return e },
	}
	for _, test := range []struct {
		name   string
		claims *reqctx.CoreXClaims
		code   codes.Code
	}{
		{"anonymous", nil, codes.Unauthenticated},
		{"ordinary user", &reqctx.CoreXClaims{UserID: 1, Scope: "access"}, codes.PermissionDenied},
		{"tenant operator", &reqctx.CoreXClaims{UserID: 1, Scope: "access", PermissionCodes: []string{"admin:root", "ops:backup:execute"}}, codes.PermissionDenied},
		{"API Key", &reqctx.CoreXClaims{UserID: 1, Scope: "access", IsRoot: true, Platforms: []string{"api_key"}}, codes.PermissionDenied},
		{"service", &reqctx.CoreXClaims{UserID: 1, Scope: "access", IsRoot: true, Platforms: []string{"service_actor"}}, codes.PermissionDenied},
		{"plugin", &reqctx.CoreXClaims{UserID: 1, Scope: "access", IsRoot: true, PluginID: "owned.plugin"}, codes.PermissionDenied},
		{"refresh token", &reqctx.CoreXClaims{UserID: 1, Scope: "refresh", IsRoot: true}, codes.PermissionDenied},
		{"Root missing user", &reqctx.CoreXClaims{IsRoot: true, Scope: "access"}, codes.Unauthenticated},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if test.claims != nil {
				ctx = reqctx.WithClaims(ctx, test.claims)
			}
			for name, call := range methods {
				require.Equal(t, test.code, status.Code(call(ctx)), name)
			}
		})
	}
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"})
	for name, call := range methods {
		require.Equal(t, codes.InvalidArgument, status.Code(call(ctx)), name+" must reach request validation for Root")
	}
	claims := &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"}
	claims.Issuer = "powerx-sts"
	for name, call := range methods {
		require.Equal(t, codes.PermissionDenied, status.Code(call(reqctx.WithClaims(context.Background(), claims))), name)
	}
}
