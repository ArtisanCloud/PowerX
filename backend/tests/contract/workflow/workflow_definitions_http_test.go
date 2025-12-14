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

const workflowDefinitionTenantUUID = "0c60d880-6f9a-4e48-947c-6b2af9502bd7"

func TestWorkflowDefinitionHTTPFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := testenv.New(t)

	engine := gin.New()
	public := engine.Group("/api")
	protected := engine.Group("/api")
	protected.Use(requireWorkflowAuth(workflowDefinitionTenantUUID))

	deps := &shared.Deps{
		Workflow: &shared.WorkflowDeps{
			Service: env.Service,
		},
	}

	workflowhttp.RegisterAPIRoutes(public, protected, deps)

	// Unauthorized request
	rr := httptest.NewRecorder()
	reqBody := map[string]any{
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
	req = httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rr = serveWorkflowRequest(t, engine, req, workflowDefinitionTenantUUID)
	require.Equal(t, http.StatusCreated, rr.Code)

	var createResp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &createResp))
	definitionID, _ := createResp.Data["uuid"].(string)
	require.NotEmpty(t, definitionID)
	require.Equal(t, workflowDefinitionTenantUUID, createResp.Data["tenant_uuid"])

	// Publish definition
	publishPayload := map[string]any{}
	publishBody, _ := json.Marshal(publishPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions/"+definitionID+"/publish", bytes.NewReader(publishBody))
	req.Header.Set("Content-Type", "application/json")
	rr = serveWorkflowRequest(t, engine, req, workflowDefinitionTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code)

	// List definitions
	req = httptest.NewRequest(http.MethodGet, "/api/admin/workflows/definitions", nil)
	rr = serveWorkflowRequest(t, engine, req, workflowDefinitionTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code)

	missingHeaderReq := httptest.NewRequest(http.MethodGet, "/api/admin/workflows/definitions", nil)
	missingHeaderReq.Header.Set("Authorization", "Bearer test")
	missingResp := httptest.NewRecorder()
	engine.ServeHTTP(missingResp, missingHeaderReq)
	require.Equal(t, http.StatusUnauthorized, missingResp.Code)

	var listResp struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &listResp))
}
