//go:build ignore

package workflowcontract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	workflowhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/workflow"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWorkflowDefinitionHTTPFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := testenv.New(t)

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

	deps := &shared.Deps{
		Workflow: &shared.WorkflowDeps{
			Service: env.Service,
		},
	}

	workflowhttp.RegisterAPIRoutes(public, protected, deps)

	// Unauthorized request
	rr := httptest.NewRecorder()
	reqBody := map[string]any{
		"tenant_id":   1001,
		"name":        "http-demo",
		"description": "workflow via http",
		"steps": []map[string]any{
			{"id": "start", "type": "agent", "next_step_ids": []string{"finish"}},
			{"id": "finish", "type": "system"},
		},
	}
	buf, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusUnauthorized, rr.Code)

	// Create definition
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test")
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	var createResp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &createResp))
	definitionID, _ := createResp.Data["uuid"].(string)
	require.NotEmpty(t, definitionID)

	// Publish definition
	publishPayload := map[string]any{
		"tenant_id": 1001,
	}
	publishBody, _ := json.Marshal(publishPayload)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions/"+definitionID+"/publish", bytes.NewReader(publishBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test")
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// List definitions
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/workflows/definitions?tenant_id=1001", nil)
	req.Header.Set("Authorization", "Bearer test")
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var listResp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &listResp))
}
