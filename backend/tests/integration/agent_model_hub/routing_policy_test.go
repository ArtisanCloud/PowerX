package agentmodelhubintegration

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

func TestRoutingPolicyIntegration(t *testing.T) {
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

	policyPayload := map[string]any{
		"env":         "default",
		"tenantScope": "tenant-routing-int",
		"rules": []map[string]any{
			{
				"taskPattern": "chat/*",
				"candidates": []map[string]any{
					{"providerId": "provider-alpha", "weight": 0.9},
					{"providerId": "provider-beta", "weight": 0.4},
				},
				"sla": map[string]any{
					"latencyMs":   900,
					"costCeiling": 0.002,
				},
			},
		},
		"fallbackChain": []string{"provider-fallback"},
	}

	// create policy draft
	req := httptest.NewRequest(http.MethodPost, "/api/internal/model-routing/policies", bytes.NewReader(mustJSON(t, policyPayload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code)

	// promote to active
	statusPayload := map[string]any{
		"tenantScope":  "tenant-routing-int",
		"targetStatus": "active",
	}
	req = httptest.NewRequest(http.MethodPost, "/api/internal/model-routing/policies/status", bytes.NewReader(mustJSON(t, statusPayload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// route decision should pick provider-alpha
	routePayload := map[string]any{
		"env":      "default",
		"tenantId": "tenant-routing-int",
		"taskContext": map[string]any{
			"taskType": "chat/general",
		},
	}
	result := routeRequest(t, engine, routePayload)
	require.Equal(t, "provider-alpha", result["primaryProviderId"])
	require.False(t, result["fallbackUsed"].(bool))

	// enable safe-mode forcing fallback
	safeModePayload := map[string]any{
		"tenantScope": "tenant-routing-int",
		"enabled":     true,
		"ttlSeconds":  60,
		"reason":      "incident",
	}
	req = httptest.NewRequest(http.MethodPost, "/api/internal/model-routing/safe-mode", bytes.NewReader(mustJSON(t, safeModePayload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	result = routeRequest(t, engine, routePayload)
	require.Equal(t, "provider-fallback", result["primaryProviderId"])
	require.True(t, result["safeMode"].(bool))
	require.True(t, result["fallbackUsed"].(bool))

	// disable safe-mode
	safeModePayload["enabled"] = false
	req = httptest.NewRequest(http.MethodPost, "/api/internal/model-routing/safe-mode", bytes.NewReader(mustJSON(t, safeModePayload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	rr = httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	result = routeRequest(t, engine, routePayload)
	require.Equal(t, "provider-alpha", result["primaryProviderId"])
	require.False(t, result["safeMode"].(bool))
	require.False(t, result["fallbackUsed"].(bool))
}

func routeRequest(t *testing.T, engine *gin.Engine, payload map[string]any) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/internal/model-routing/route", bytes.NewReader(mustJSON(t, payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	return resp.Data
}
