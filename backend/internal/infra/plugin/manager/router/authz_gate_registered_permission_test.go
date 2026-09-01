package router

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

func TestCheckAndMintUsesRegisteredRoutePermissionCode(t *testing.T) {
	auth.SetJWTSecret([]byte("test-secret"))
	az := &registeredRouteAuthorizer{
		routes: map[string]Permission{
			"POST:/api/v1/example/records/42/approve": {
				Module:   "workspace",
				Resource: "case_file",
				Action:   "approve",
			},
		},
		perms: []string{"workspace.case_file:approve"},
	}
	g := newAuthzGate(az, "powerx-auth", time.Minute)

	token, allowed, reason := g.CheckAndMint(context.Background(), "demo.plugin", "POST", "/api/v1/example/records/42/approve", reqctx.CoreXClaims{
		TenantUUID: "tenant-uuid",
		MemberID:   22,
		UserID:     33,
	})
	if !allowed {
		t.Fatalf("allowed=false reason=%q", reason)
	}
	if token == "" {
		t.Fatalf("expected minted token")
	}
	claims, err := auth.ParseAndValidate(token, []byte("test-secret"), "powerx-auth", "plugin:demo.plugin")
	if err != nil {
		t.Fatalf("ParseAndValidate error: %v", err)
	}
	if len(claims.PermissionCodes) != 1 || claims.PermissionCodes[0] != "workspace.case_file:approve" {
		t.Fatalf("permission codes = %#v", claims.PermissionCodes)
	}
	if claims.PermsHash == "" {
		t.Fatalf("expected perms_hash")
	}
	if claims.PolicyVersion == "" {
		t.Fatalf("expected policy_version")
	}
	if err := ValidatePermissionSnapshot(*claims, ""); err != nil {
		t.Fatalf("ValidatePermissionSnapshot error: %v", err)
	}
}

func TestCheckAndMintRejectsMissingRegisteredRouteBindingBeforeRootBypass(t *testing.T) {
	auth.SetJWTSecret([]byte("test-secret"))
	az := &registeredRouteAuthorizer{routes: map[string]Permission{}}
	g := newAuthzGate(az, "powerx-auth", time.Minute)

	_, allowed, reason := g.CheckAndMint(context.Background(), "demo.plugin", "POST", "/api/v1/unregistered", reqctx.CoreXClaims{
		TenantUUID: "tenant-uuid",
		MemberID:   22,
		UserID:     33,
		IsRoot:     true,
	})
	if allowed {
		t.Fatalf("expected deny for missing registered binding")
	}
	if reason != "no registered permission binding for this route" {
		t.Fatalf("reason=%q", reason)
	}
}

func TestCheckAndMintRejectsMissingPermissionCode(t *testing.T) {
	auth.SetJWTSecret([]byte("test-secret"))
	az := &registeredRouteAuthorizer{
		routes: map[string]Permission{
			"POST:/api/v1/example/records/42/approve": {
				Module:   "workspace",
				Resource: "case_file",
				Action:   "approve",
			},
		},
		perms: []string{"workspace.case_file:read"},
	}
	g := newAuthzGate(az, "powerx-auth", time.Minute)

	_, allowed, reason := g.CheckAndMint(context.Background(), "demo.plugin", "POST", "/api/v1/example/records/42/approve", reqctx.CoreXClaims{
		TenantUUID: "tenant-uuid",
		MemberID:   22,
		UserID:     33,
	})
	if allowed {
		t.Fatalf("expected deny for missing permission")
	}
	if reason != "permission required: case_file:approve" {
		t.Fatalf("reason=%q", reason)
	}
}

func TestCheckAndMintAllowsRegisteredRuntimeContractWithoutRolePermission(t *testing.T) {
	auth.SetJWTSecret([]byte("test-secret"))
	az := &registeredRouteAuthorizer{
		routes: map[string]Permission{
			"POST:/admin/runtime/ws-bus/grant": {
				Module:   "runtime",
				Resource: "contract",
				Action:   "tenant_context",
			},
		},
		perms: []string{"operations.ai_craft:read"},
	}
	g := newAuthzGate(az, "powerx-auth", time.Minute)

	token, allowed, reason := g.CheckAndMint(context.Background(), "demo.plugin", "POST", "/admin/runtime/ws-bus/grant", reqctx.CoreXClaims{
		TenantUUID: "tenant-uuid",
		MemberID:   22,
		UserID:     33,
	})
	if !allowed {
		t.Fatalf("allowed=false reason=%q", reason)
	}
	claims, err := auth.ParseAndValidate(token, []byte("test-secret"), "powerx-auth", "plugin:demo.plugin")
	if err != nil {
		t.Fatalf("ParseAndValidate error: %v", err)
	}
	if len(claims.PermissionCodes) != 1 || claims.PermissionCodes[0] != "runtime.contract:tenant_context" {
		t.Fatalf("permission codes = %#v", claims.PermissionCodes)
	}
	if err := ValidatePermissionSnapshot(*claims, ""); err != nil {
		t.Fatalf("ValidatePermissionSnapshot error: %v", err)
	}
}

func TestCheckAndMintAllowsTenantContextContractWithoutModule(t *testing.T) {
	auth.SetJWTSecret([]byte("test-secret"))
	az := &registeredRouteAuthorizer{
		routes: map[string]Permission{
			"POST:/admin/runtime/ws-bus/grant": {
				Resource: "contract",
				Action:   "tenant_context",
			},
		},
		perms: []string{"operations.ai_craft:read"},
	}
	g := newAuthzGate(az, "powerx-auth", time.Minute)

	token, allowed, reason := g.CheckAndMint(context.Background(), "demo.plugin", "POST", "/admin/runtime/ws-bus/grant", reqctx.CoreXClaims{
		TenantUUID: "tenant-uuid",
		MemberID:   22,
		UserID:     33,
	})
	if !allowed {
		t.Fatalf("allowed=false reason=%q", reason)
	}
	claims, err := auth.ParseAndValidate(token, []byte("test-secret"), "powerx-auth", "plugin:demo.plugin")
	if err != nil {
		t.Fatalf("ParseAndValidate error: %v", err)
	}
	if len(claims.PermissionCodes) != 1 || claims.PermissionCodes[0] != "contract:tenant_context" {
		t.Fatalf("permission codes = %#v", claims.PermissionCodes)
	}
	if err := ValidatePermissionSnapshot(*claims, ""); err != nil {
		t.Fatalf("ValidatePermissionSnapshot error: %v", err)
	}
}

func TestCheckAndMintRejectsRuntimeContractWithoutTenantContext(t *testing.T) {
	auth.SetJWTSecret([]byte("test-secret"))
	az := &registeredRouteAuthorizer{
		routes: map[string]Permission{
			"POST:/admin/runtime/ws-bus/grant": {
				Module:   "runtime",
				Resource: "contract",
				Action:   "tenant_context",
			},
		},
		perms: []string{"operations.ai_craft:read"},
	}
	g := newAuthzGate(az, "powerx-auth", time.Minute)

	_, allowed, reason := g.CheckAndMint(context.Background(), "demo.plugin", "POST", "/admin/runtime/ws-bus/grant", reqctx.CoreXClaims{
		UserID:   33,
		MemberID: 22,
	})
	if allowed {
		t.Fatalf("expected deny without tenant context")
	}
	if reason != "runtime contract tenant context missing" {
		t.Fatalf("reason=%q", reason)
	}
}

func TestValidatePermissionSnapshotRejectsMissingOrExpiredClaims(t *testing.T) {
	if err := ValidatePermissionSnapshot(reqctx.CoreXClaims{}, ""); !errors.Is(err, ErrPermissionSnapshotClaimsMissing) {
		t.Fatalf("missing claims error=%v", err)
	}

	claims := reqctx.CoreXClaims{
		PermissionCodes: []string{"workspace.case_file:read"},
		PermsHash:       permissionCodesHash([]string{"workspace.case_file:read"}),
		PolicyVersion:   permissionPolicyVersion(permissionCodesHash([]string{"workspace.case_file:read"})),
	}
	if err := ValidatePermissionSnapshot(claims, "iam:stale"); !errors.Is(err, ErrPermissionSnapshotExpired) {
		t.Fatalf("stale policy version error=%v", err)
	}

	claims.PermsHash = permissionCodesHash([]string{"workspace.case_file:delete"})
	if err := ValidatePermissionSnapshot(claims, ""); !errors.Is(err, ErrPermissionSnapshotExpired) {
		t.Fatalf("stale hash error=%v", err)
	}
}

type registeredRouteAuthorizer struct {
	routes map[string]Permission
	perms  []string
}

func (a *registeredRouteAuthorizer) Permissions(context.Context, uint64, uint64, string) ([]string, string, error) {
	return nil, "", fmt.Errorf("claims required")
}

func (a *registeredRouteAuthorizer) PermissionsForClaims(context.Context, reqctx.CoreXClaims, string) ([]string, string, error) {
	return append([]string(nil), a.perms...), "", nil
}

func (a *registeredRouteAuthorizer) IsSuperAdmin(context.Context, uint64, uint64, []string) bool {
	return false
}

func (a *registeredRouteAuthorizer) RoutePermission(_ context.Context, _ string, method string, reqPath string) (*Permission, error) {
	perm, ok := a.routes[method+":"+reqPath]
	if !ok {
		return nil, nil
	}
	copy := perm
	return &copy, nil
}
