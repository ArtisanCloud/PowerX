package agentmodelhubintegration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentmodelhubhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent_model_hub"
	ammatestenv "github.com/ArtisanCloud/PowerX/tests/agent_model_hub/testenv"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProviderOnboardingIntegration(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	env := ammatestenv.New(t)

	engine := gin.New()
	public := engine.Group("/api")
	protected := engine.Group("/api")
	protected.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})

	deps := &shared.Deps{DB: env.DB}
	agentmodelhubhttp.RegisterAPIRoutes(public, protected, deps)

	req := httptest.NewRequest(http.MethodPost, "/api/internal/providers/register", bytes.NewReader(mustJSON(t, map[string]any{
		"env":              "default",
		"name":             "provider-int",
		"capabilities":     []string{"llm"},
		"primary_endpoint": "https://example.invalid",
		"regions":          []string{"us"},
		"tenantWhitelist": []map[string]string{
			{"tenantId": "demo", "environment": "staging"},
		},
		"credentials": map[string]string{
			"api_key": "sk-integration-test",
		},
	})))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code)

	var reg struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &reg))
	provider := reg.Data["provider"].(map[string]interface{})
	providerID := provider["provider_id"].(string)
	require.NotEmpty(t, providerID)

	publishPayload := map[string]any{
		"rolloutStrategy": "full",
	}
	// validate with failing report
	req = httptest.NewRequest(http.MethodPost, "/api/internal/providers/"+providerID+"/validate?suite=full", bytes.NewReader(mustJSON(t, map[string]any{
		"report": map[string]any{
			"providerId":  providerID,
			"suite":       "full",
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
			"stats":       map[string]any{"total": 1, "passed": 0, "failed": 1},
			"results": []map[string]any{
				{"name": "llm ping", "modality": "llm", "success": false, "error": "timeout"},
			},
		},
	})))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code)

	// publish should still be blocked
	req = httptest.NewRequest(http.MethodPost, "/api/internal/providers/"+providerID+"/publish", bytes.NewReader(mustJSON(t, publishPayload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)

	// validate with passing report
	req = httptest.NewRequest(http.MethodPost, "/api/internal/providers/"+providerID+"/validate?suite=full", bytes.NewReader(mustJSON(t, map[string]any{
		"report": map[string]any{
			"providerId":  providerID,
			"suite":       "full",
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
			"stats":       map[string]any{"total": 1, "passed": 1, "failed": 0},
			"results": []map[string]any{
				{"name": "llm ping", "modality": "llm", "success": true},
			},
		},
	})))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code)

	// publish succeeds now
	req = httptest.NewRequest(http.MethodPost, "/api/internal/providers/"+providerID+"/publish", bytes.NewReader(mustJSON(t, publishPayload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
}

func mustJSON(t *testing.T, payload any) []byte {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}
