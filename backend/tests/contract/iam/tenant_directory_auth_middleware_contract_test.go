package iamcontract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTenantMemberDirectoryAuthenticationHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for name, authorization := range map[string]string{
		"missing_authorization": "",
		"invalid_bearer":        "Bearer malformed-token",
	} {
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			router.Use(middleware.APIKeyOrJwtMiddleware(nil, []byte("test-signing-key"), "powerx", []string{"powerx:api"}, nil, nil))
			router.GET("/api/v1/tenant/iam/members/:member_uuid", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})
			router.POST("/api/v1/tenant/iam/members:operation", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			for _, endpoint := range []struct {
				method string
				path   string
			}{
				{method: http.MethodGet, path: "/api/v1/tenant/iam/members/6c1f9e8d-7a62-4810-aec3-d2c63a866358"},
				{method: http.MethodPost, path: "/api/v1/tenant/iam/members:batch-resolve"},
				{method: http.MethodPost, path: "/api/v1/tenant/iam/members:batch-find-by-display-names"},
			} {
				req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
				if authorization != "" {
					req.Header.Set("Authorization", authorization)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, req)

				require.Equal(t, http.StatusUnauthorized, response.Code)
				body := normalizeDirectoryErrorEnvelope(t, response.Body.Bytes())
				require.JSONEq(t, `{"code":401,"message":"IAM_UNAUTHORIZED","error":"IAM_UNAUTHORIZED: IAM_UNAUTHORIZED","error_code":"IAM_UNAUTHORIZED","reason_code":"IAM_UNAUTHORIZED","timestamp":0}`, body)
			}
		})
	}
}

func normalizeDirectoryErrorEnvelope(t *testing.T, raw []byte) string {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	body["timestamp"] = float64(0)
	normalized, err := json.Marshal(body)
	require.NoError(t, err)
	return string(normalized)
}
