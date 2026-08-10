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
			"POST:/api/v1/sample-tracks/42/nodes/sample-schedule": {
				Module:   "production",
				Resource: "sample_track",
				Action:   "factory_schedule",
			},
		},
		perms: []string{"production.sample_track:factory_schedule"},
	}
	g := newAuthzGate(az, "powerx-auth", time.Minute)

	token, allowed, reason := g.CheckAndMint(context.Background(), "demo.plugin", "POST", "/api/v1/sample-tracks/42/nodes/sample-schedule", reqctx.CoreXClaims{
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
	if len(claims.PermissionCodes) != 1 || claims.PermissionCodes[0] != "production.sample_track:factory_schedule" {
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
			"POST:/api/v1/sample-tracks/42/nodes/sample-schedule": {
				Module:   "production",
				Resource: "sample_track",
				Action:   "factory_schedule",
			},
		},
		perms: []string{"production.sample_track:read"},
	}
	g := newAuthzGate(az, "powerx-auth", time.Minute)

	_, allowed, reason := g.CheckAndMint(context.Background(), "demo.plugin", "POST", "/api/v1/sample-tracks/42/nodes/sample-schedule", reqctx.CoreXClaims{
		TenantUUID: "tenant-uuid",
		MemberID:   22,
		UserID:     33,
	})
	if allowed {
		t.Fatalf("expected deny for missing permission")
	}
	if reason != "permission required: sample_track:factory_schedule" {
		t.Fatalf("reason=%q", reason)
	}
}

func TestValidatePermissionSnapshotRejectsMissingOrExpiredClaims(t *testing.T) {
	if err := ValidatePermissionSnapshot(reqctx.CoreXClaims{}, ""); !errors.Is(err, ErrPermissionSnapshotClaimsMissing) {
		t.Fatalf("missing claims error=%v", err)
	}

	claims := reqctx.CoreXClaims{
		PermissionCodes: []string{"production.sample_track:read"},
		PermsHash:       permissionCodesHash([]string{"production.sample_track:read"}),
		PolicyVersion:   permissionPolicyVersion(permissionCodesHash([]string{"production.sample_track:read"})),
	}
	if err := ValidatePermissionSnapshot(claims, "iam:stale"); !errors.Is(err, ErrPermissionSnapshotExpired) {
		t.Fatalf("stale policy version error=%v", err)
	}

	claims.PermsHash = permissionCodesHash([]string{"production.sample_track:delivery"})
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
