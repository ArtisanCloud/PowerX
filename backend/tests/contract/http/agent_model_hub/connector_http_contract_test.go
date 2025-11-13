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

func TestConnectorHTTPContract(t *testing.T) {
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

	instancePayload := map[string]any{
		"tenantId":             "demo-tenant",
		"region":               "us-east-1",
		"oauthRef":             "vault://connectors/coze/demo",
		"webhookSigningKeyRef": "vault://connectors/coze/signing",
		"mappingTemplate": map[string]any{
			"workflow": "sync_leads",
		},
		"rateLimitPerMinute": 120,
	}
	body, _ := json.Marshal(instancePayload)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/connector-platforms/coze/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var createResp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &createResp))
	require.Equal(t, http.StatusOK, createResp.Code)
	instance := createResp.Data["instance"].(map[string]interface{})
	instanceID := instance["instance_id"].(string)
	require.NotEmpty(t, instanceID)

	pauseReq := httptest.NewRequest(http.MethodPost, "/api/internal/connector-platforms/coze/instances/"+instanceID+"/pause", nil)
	pauseReq.Header.Set("Authorization", "Bearer token")
	pauseRR := httptest.NewRecorder()
	engine.ServeHTTP(pauseRR, pauseReq)
	require.Equal(t, http.StatusOK, pauseRR.Code)
}
