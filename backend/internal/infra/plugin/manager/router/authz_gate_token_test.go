package router

import (
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

func TestMintPluginTokenIncludesUserIdentityClaims(t *testing.T) {
	secret := []byte("test-secret")
	auth.SetJWTSecret(secret)

	g := newAuthzGate(nil, "powerx-auth", time.Minute)
	token, err := g.mintPluginToken("com.powerx.plugins.test", reqctx.CoreXClaims{
		TenantUUID: "tenant-uuid",
		TenantID:   11,
		MemberUUID: "member-uuid",
		MemberID:   22,
		UserUUID:   "user-uuid",
		UserID:     33,
		Email:      "  USER@EXAMPLE.INVALID ",
		Phone:      " 13800000000 ",
		Platforms:  []string{"web"},
		IsRoot:     true,
	})
	if err != nil {
		t.Fatalf("mintPluginToken error: %v", err)
	}

	claims, err := auth.ParseAndValidate(token, secret, "powerx-auth", "plugin:com.powerx.plugins.test")
	if err != nil {
		t.Fatalf("ParseAndValidate error: %v", err)
	}
	if claims.Email != "user@example.invalid" {
		t.Fatalf("email = %q, want user@example.invalid", claims.Email)
	}
	if claims.Phone != "13800000000" {
		t.Fatalf("phone = %q, want 13800000000", claims.Phone)
	}
	if claims.UserUUID != "user-uuid" {
		t.Fatalf("user uuid = %q, want user-uuid", claims.UserUUID)
	}
}
