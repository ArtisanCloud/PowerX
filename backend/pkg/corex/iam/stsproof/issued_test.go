package stsproof

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSharedJWTSecretCannotForgeCoreIssuance(t *testing.T) {
	ctx := context.Background()
	store := cache.NewMemoryCache()
	c := reqctx.CoreXClaims{TenantUUID: uuid.NewString(), PluginID: "com.example.source", Scope: "access", RegisteredClaims: jwt.RegisteredClaims{ID: uuid.NewString(), Issuer: "powerx-sts", Subject: "client:source", Audience: []string{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
	// 同样能通过旧 HMAC 验签的令牌，没有 Core 签发记录仍然无效。
	secret := []byte("fixture-legacy-shared-key")
	encoded, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
	require.NoError(t, err)
	parsed, err := auth.ParseAndValidate(encoded, secret, "powerx-sts", "powerx:api")
	require.NoError(t, err)
	require.ErrorIs(t, Verify(ctx, store, parsed), ErrNotIssued)
	require.NoError(t, Record(ctx, store, &c))
	require.NoError(t, Verify(ctx, store, parsed))
	for _, alter := range []func(*reqctx.CoreXClaims){func(c *reqctx.CoreXClaims) { c.PluginID = "com.example.other" }, func(c *reqctx.CoreXClaims) { c.TenantUUID = uuid.NewString() }, func(c *reqctx.CoreXClaims) { c.Subject = "client:other" }, func(c *reqctx.CoreXClaims) { c.IsRoot = true }, func(c *reqctx.CoreXClaims) { c.ID = uuid.NewString() }} {
		forged := c
		alter(&forged)
		encoded, err := jwt.NewWithClaims(jwt.SigningMethodHS256, forged).SignedString(secret)
		require.NoError(t, err)
		claims, err := auth.ParseAndValidate(encoded, secret, "powerx-sts", "powerx:api")
		require.NoError(t, err)
		require.ErrorIs(t, Verify(ctx, store, claims), ErrNotIssued)
	}
	require.ErrorIs(t, Verify(ctx, nil, &c), ErrUnavailable)
}
