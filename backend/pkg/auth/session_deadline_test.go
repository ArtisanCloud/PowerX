package auth

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSessionAccessJWTNeverOutlivesDeadline(t *testing.T) {
	secret := []byte("controlled-deadline-key")
	deadline := time.Now().Add(2 * time.Minute).Truncate(time.Second)
	token, err := GenerateAccessJWTUntil(reqctx.CoreXClaims{}, "test", []string{"user"}, 720*time.Hour, secret, deadline)
	require.NoError(t, err)
	claims, err := ParseAndValidate(token, secret, "test", "user")
	require.NoError(t, err)
	require.Equal(t, deadline, claims.ExpiresAt.Time)
	_, err = GenerateAccessJWTUntil(reqctx.CoreXClaims{}, "test", []string{"user"}, time.Hour, secret, time.Now().Add(-time.Second))
	require.Error(t, err)
}
