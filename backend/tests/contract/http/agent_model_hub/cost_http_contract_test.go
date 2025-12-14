//go:build ignore

package agentmodelhubcontract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentmodelhubhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent_model_hub"
	ammatestenv "github.com/ArtisanCloud/PowerX/tests/agent_model_hub/testenv"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCostHTTPContract(t *testing.T) {
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

	usagePayload := map[string]any{
		"providerId": "provider-alpha",
		"events": []map[string]any{
			{
				"traceId":   "trace-1",
				"tokens":    1200,
				"costUsd":   0.42,
				"timestamp": "2025-11-10T08:00:00Z",
			},
		},
	}
	usageBody, _ := json.Marshal(usagePayload)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/provider-usage/report", bytes.NewReader(usageBody))
	req.Header.Set("Content-Type", "application/json")
	rr := serveAgentModelHubRequest(t, engine, req, ammatestenv.AgentModelHubTenantUUID)
	require.Equal(t, http.StatusAccepted, rr.Code, "usage ingestion should accept payloads")

	quotaReq := httptest.NewRequest(http.MethodGet, "/api/internal/provider-quotas", nil)
	rr = serveAgentModelHubRequest(t, engine, quotaReq, ammatestenv.AgentModelHubTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code, "quota snapshot should be readable")

	enforcePayload := map[string]any{
		"providerId": "provider-alpha",
		"action":     "throttle",
		"reason":     "Exceeded budget",
		"requestedBy": map[string]any{
			"actor": "finops@example.com",
			"role":  "FinOps",
		},
		"parameters": map[string]any{
			"limitPct": 50,
		},
	}
	enforceBody, _ := json.Marshal(enforcePayload)
	req = httptest.NewRequest(http.MethodPost, "/api/internal/provider-quotas/enforce", bytes.NewReader(enforceBody))
	req.Header.Set("Content-Type", "application/json")
	rr = serveAgentModelHubRequest(t, engine, req, ammatestenv.AgentModelHubTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code, "enforcement endpoint should acknowledge actions")
}
