package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCurrentSessionMiddlewareKeepsAuthorityAndFailureClassification(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		ttl        time.Duration
		credential bool
		callback   error
		status     int
		code       string
	}{
		{"missing", time.Hour, false, nil, 401, "AUTH_SESSION_INVALID"},
		{"expired", -time.Hour, true, nil, 401, "AUTH_ACCESS_EXPIRED"},
		{"unavailable", time.Hour, true, &dto.AppError{HTTPCode: 503, Code: "AUTH_SESSION_UNAVAILABLE"}, 503, "AUTH_SESSION_UNAVAILABLE"},
		{"forbidden", time.Hour, true, &dto.AppError{HTTPCode: 403, Code: "AUTH_USER_FORBIDDEN"}, 403, "AUTH_USER_FORBIDDEN"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			router := gin.New()
			secret := []byte("controlled-middleware-key")
			router.Use(JwtMiddleware(secret, "powerx-auth", []string{"user"}, []string{"access"}, func(context.Context, *reqctx.CoreXClaims) error { return scenario.callback }))
			reached := false
			router.GET("/api/v1/admin/user/auth/me/session", func(c *gin.Context) { reached = true; c.Status(200) })
			request := httptest.NewRequest("GET", "/api/v1/admin/user/auth/me/session", nil)
			if scenario.credential {
				token, err := auth.GenerateAccessJWT(reqctx.CoreXClaims{TenantUUID: uuid.NewString()}, "powerx-auth", []string{"user"}, scenario.ttl, secret)
				require.NoError(t, err)
				request.Header.Set("Authorization", "Bearer "+token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, scenario.status, response.Code)
			require.Contains(t, response.Body.String(), scenario.code)
			require.False(t, reached)
		})
	}
}
func TestCurrentSessionRevocationEnvelopeIsPrivateToProbe(t *testing.T) {
	for _, path := range []string{"/api/v1/admin/user/auth/me/session", "/api/v1/admin/other", "/_p/com.example.plugin/api/v1/admin/user/auth/me/session"} {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodGet, path, nil)
		handled := abortCurrentUserSessionAuthError(c, 401, "AUTH_SESSION_REVOKED")
		if path != "/api/v1/admin/user/auth/me/session" {
			require.False(t, handled)
		} else {
			require.True(t, handled)
			require.Contains(t, response.Body.String(), "AUTH_SESSION_REVOKED")
		}
	}
}
