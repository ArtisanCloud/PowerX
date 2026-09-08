package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestJwtMiddlewareUsesGrantStatusEnvelopeBeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(JwtMiddleware([]byte("test-secret"), "powerx-auth", []string{"powerx:api"}, nil, nil))
	router.POST("/api/v1/tenant/capabilities:grant-status", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/tenant/capabilities:grant-status", nil))
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Contains(t, response.Body.String(), `"error_code":"CAPABILITY_GRANT_STATUS_UNAUTHORIZED"`)
	require.Contains(t, response.Body.String(), `"reason_code":"CAPABILITY_GRANT_STATUS_UNAUTHORIZED"`)
}

func TestJwtMiddlewareUsesTenantHostEnvelopeBeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		path string
		code string
	}{
		{"/api/v1/tenant/media/assets", "MEDIA_UNAUTHORIZED"},
		{"/api/v1/tenant/customer/auth/login", "CUSTOMER_UNAUTHORIZED"},
		{"/api/v1/tenant/plugin-release/install-sessions", "PLUGIN_RELEASE_UNAUTHORIZED"},
		{"/api/v1/tenant/metadata/dictionaries", "METADATA_UNAUTHORIZED"},
	} {
		t.Run(tt.code, func(t *testing.T) {
			router := gin.New()
			router.Use(JwtMiddleware([]byte("test-secret"), "powerx-auth", []string{"powerx:api"}, nil, nil))
			router.POST(tt.path, func(c *gin.Context) { c.Status(http.StatusNoContent) })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, tt.path, nil))
			require.Equal(t, http.StatusUnauthorized, response.Code)
			require.Contains(t, response.Body.String(), `"error_code":"`+tt.code+`"`)
			require.Contains(t, response.Body.String(), `"reason_code":"`+tt.code+`"`)
		})
	}
}
