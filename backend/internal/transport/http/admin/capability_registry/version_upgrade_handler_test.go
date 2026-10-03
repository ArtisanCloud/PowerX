package capability_registry

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestVersionUpgradeHTTPRejectsServiceCredentialsAndCallerActor(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		claims     *reqctx.CoreXClaims
		apiKey     bool
		status     int
	}{
		{name: "anonymous", body: `{}`, status: 401},
		{name: "member", body: `{}`, claims: &reqctx.CoreXClaims{UserID: 1}, status: 403},
		{name: "api_key", body: `{}`, claims: &reqctx.CoreXClaims{UserID: 1, IsRoot: true}, apiKey: true, status: 403},
		{name: "caller_actor", body: `{"actor":"root","capabilities_hash":"x","reason":"x"}`, claims: &reqctx.CoreXClaims{UserID: 1, IsRoot: true}, status: 400},
		{name: "trailing_json", body: `{} {}`, claims: &reqctx.CoreXClaims{UserID: 1, IsRoot: true}, status: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				if tc.claims != nil {
					c.Request = c.Request.WithContext(reqctx.WithClaims(c.Request.Context(), tc.claims))
				}
				if tc.apiKey {
					c.Set("auth_source", "api_key")
				}
				c.Next()
			})
			handler := &versionUpgradeHandler{}
			engine.POST("/upgrade", middleware.AdminOnlyMiddleware(), handler.Confirm)
			request := httptest.NewRequest("POST", "/upgrade", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code)
		})
	}
}
