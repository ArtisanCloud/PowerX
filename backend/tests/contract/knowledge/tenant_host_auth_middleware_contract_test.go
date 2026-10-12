package knowledgecontract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTenantKnowledgeAuthenticationUsesStableEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, authorization := range map[string]string{"missing_authorization": "", "invalid_bearer": "Bearer malformed-token"} {
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			router.Use(middleware.APIKeyOrJwtMiddleware(nil, []byte("test-signing-key"), "powerx", []string{"powerx:api"}, nil, nil))
			router.GET("/api/v1/tenant/knowledge/spaces", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			req := httptest.NewRequest(http.MethodGet, "/api/v1/tenant/knowledge/spaces", nil)
			if authorization != "" {
				req.Header.Set("Authorization", authorization)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.Equal(t, http.StatusUnauthorized, response.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Equal(t, "KNOWLEDGE_UNAUTHORIZED", body["error_code"])
			require.Equal(t, "KNOWLEDGE_UNAUTHORIZED", body["reason_code"])
		})
	}
}
