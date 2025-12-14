//go:build ignore

package agentmodelhubcontract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentmodelhubhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent_model_hub"
	ammatestenv "github.com/ArtisanCloud/PowerX/tests/agent_model_hub/testenv"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProviderHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := ammatestenv.New(t)
	env.MustInsertTenant(5001, ammatestenv.AgentModelHubTenantUUID)

	engine := gin.New()
	public := engine.Group("/api")
	protected := engine.Group("/api")
	protected.Use(ammatestenv.RequireAgentModelHubAuth())

	deps := &shared.Deps{DB: env.DB}
	agentmodelhubhttp.RegisterAPIRoutes(public, protected, deps)

	registerPayload := map[string]any{
		"env":              "default",
		"name":             "openai",
		"capabilities":     []string{"llm", "embedding"},
		"primary_endpoint": "https://api.openai.com/v1",
		"regions":          []string{"us-east-1"},
		"tenantWhitelist": []map[string]string{
			{"tenant_uuid": ammatestenv.AgentModelHubTenantUUID, "environment": "staging"},
		},
		"credentials": map[string]string{
			"api_key": "sk-test-123",
		},
	}
	body, _ := json.Marshal(registerPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/providers/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := serveAgentModelHubRequest(t, engine, req, ammatestenv.AgentModelHubTenantUUID)
	require.Equal(t, http.StatusAccepted, rr.Code)
	require.NotContains(t, rr.Body.String(), "sk-test-123", "should not leak api keys")

	var registerResp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &registerResp))
	require.Equal(t, http.StatusAccepted, registerResp.Code)
	provider := registerResp.Data["provider"].(map[string]interface{})
	providerID := provider["provider_id"].(string)
	require.NotEmpty(t, providerID)
	require.Equal(t, "draft", strings.ToLower(provider["rollout_status"].(string)))
	for _, field := range []string{"api_key", "apikey", "secret", "secretKey"} {
		_, exists := provider[field]
		require.Falsef(t, exists, "provider JSON must not expose field %s", field)
	}

	failReport := map[string]any{
		"report": map[string]any{
			"providerId":  providerID,
			"suite":       "full",
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
			"stats": map[string]any{
				"total":  2,
				"passed": 1,
				"failed": 1,
			},
			"results": []map[string]any{
				{"name": "llm ping", "modality": "llm", "success": true},
				{"name": "embedding ping", "modality": "embed", "success": false, "error": "timeout"},
			},
		},
	}
	body, _ = json.Marshal(failReport)
	req = httptest.NewRequest(http.MethodPost, "/api/internal/providers/"+providerID+"/validate?suite=full", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = serveAgentModelHubRequest(t, engine, req, ammatestenv.AgentModelHubTenantUUID)
	require.Equal(t, http.StatusAccepted, rr.Code)

	// Publish should be blocked due to failed validation
	publishPayload := map[string]any{
		"rolloutStrategy": "full",
	}
	pubBody, _ := json.Marshal(publishPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/internal/providers/"+providerID+"/publish", bytes.NewReader(pubBody))
	req.Header.Set("Content-Type", "application/json")
	rr = serveAgentModelHubRequest(t, engine, req, ammatestenv.AgentModelHubTenantUUID)
	require.Equal(t, http.StatusBadRequest, rr.Code)

	// Re-run validation with passing report
	passReport := map[string]any{
		"report": map[string]any{
			"providerId":  providerID,
			"suite":       "full",
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
			"stats": map[string]any{
				"total":  1,
				"passed": 1,
				"failed": 0,
			},
			"results": []map[string]any{
				{"name": "llm ping", "modality": "llm", "success": true},
			},
		},
	}
	body, _ = json.Marshal(passReport)
	req = httptest.NewRequest(http.MethodPost, "/api/internal/providers/"+providerID+"/validate?suite=full", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = serveAgentModelHubRequest(t, engine, req, ammatestenv.AgentModelHubTenantUUID)
	require.Equal(t, http.StatusAccepted, rr.Code)

	// Publish succeeds after passing validation
	req = httptest.NewRequest(http.MethodPost, "/api/internal/providers/"+providerID+"/publish", bytes.NewReader(pubBody))
	req.Header.Set("Content-Type", "application/json")
	rr = serveAgentModelHubRequest(t, engine, req, ammatestenv.AgentModelHubTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code)
}
