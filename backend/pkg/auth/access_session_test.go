package auth

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAccessTokenRetainsRefreshSessionIdentity(t *testing.T) {
	secret := []byte("a-test-signing-secret-with-thirty-two-characters")
	claims := reqctx.CoreXClaims{TenantUUID: uuid.NewString(), UserUUID: uuid.NewString(), MemberUUID: uuid.NewString()}
	claims.ID = uuid.NewString()
	token, err := GenerateAccessJWT(claims, "fixture", []string{"user"}, time.Minute, secret)
	require.NoError(t, err)
	parsed, err := ParseAndValidate(token, secret, "fixture", "user")
	require.NoError(t, err)
	require.Equal(t, claims.ID, parsed.ID)
}
