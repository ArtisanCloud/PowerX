package auth

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
)

func TestPlatformRootSessionRequiresVerifiedUserContext(t *testing.T) {
	base := reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"}
	for _, test := range []struct {
		name string
		edit func(*reqctx.CoreXClaims)
	}{
		{"user missing", func(c *reqctx.CoreXClaims) { c.UserID = 0 }},
		{"root missing", func(c *reqctx.CoreXClaims) { c.IsRoot = false }},
		{"refresh token", func(c *reqctx.CoreXClaims) { c.Scope = "refresh" }},
		{"plugin token", func(c *reqctx.CoreXClaims) { c.PluginID = "owned.plugin" }},
		{"STS issuer", func(c *reqctx.CoreXClaims) { c.Issuer = " PowerX-STS " }},
		{"API key", func(c *reqctx.CoreXClaims) { c.Platforms = []string{"admin", " API_KEY "} }},
		{"service", func(c *reqctx.CoreXClaims) { c.Platforms = []string{" Service_Actor "} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims := base
			test.edit(&claims)
			require.False(t, IsPlatformRootSession(reqctx.WithClaims(context.Background(), &claims)))
		})
	}
	require.False(t, IsPlatformRootSession(reqctx.WithIsRoot(context.Background(), true)))
	require.True(t, IsPlatformRootSession(reqctx.WithClaims(context.Background(), &base)))
}
