package manager

import (
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func TestMintPluginAccessTokenMatchesPluginAudience(t *testing.T) {
	mgr := &managerImpl{
		opts: Options{
			CoreConfig: &config.Config{
				Auth: config.AuthConfig{
					JWTSecret:    "0123456789abcdef0123456789abcdef",
					Issuer:       "powerx-auth",
					AccessTTLStr: "15m",
				},
			},
		},
	}

	token, err := MintPluginAccessToken(mgr, "com.powerx.plugins.base")
	if err != nil {
		t.Fatalf("MintPluginAccessToken() err = %v", err)
	}
	claims, err := auth.ParseAndValidate(
		token,
		[]byte("0123456789abcdef0123456789abcdef"),
		"powerx-auth",
		"plugin:com.powerx.plugins.base",
	)
	if err != nil {
		t.Fatalf("ParseAndValidate() err = %v", err)
	}
	if !claims.IsRoot {
		t.Fatalf("claims.IsRoot = false, want true")
	}
	if claims.Scope != "access" {
		t.Fatalf("claims.Scope = %q, want access", claims.Scope)
	}
}

func TestMintPluginAccessTokenRequiresManagerConfig(t *testing.T) {
	_, err := MintPluginAccessToken(plugin_mgr.Manager(nil), "com.powerx.plugins.base")
	if err == nil {
		t.Fatalf("expected error")
	}
}
