package workflowcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	workflowhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/workflow"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestWorkflowControlHTTP(t *testing.T) {
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

	defPayload := map[string]any{
		"tenant_id":   2002,
		"name":        "runtime-http-demo",
		"description": "http control contract",
		"steps": []map[string]any{
			{
				"id":            "prepare",
				"type":          "system",
				"next_step_ids": []string{"agent_step"},
			},
			{
				"id":            "agent_step",
				"type":          "agent",
				"next_step_ids": []string{"finalize"},
				"compensatable": true,
				"config": map[string]any{
					"capability": "demo.capability",
					"agent_id":   uuid.New().String(),
				},
			},
			{
				"id":   "finalize",
				"type": "system",
			},
		},
	}

	body, _ := json.Marshal(defPayload)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	var createResp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &createResp))
	defID, _ := createResp.Data["uuid"].(string)
	require.NotEmpty(t, defID)

	publishPayload := map[string]any{
		"tenant_id": 2002,
	}
	pubBody, _ := json.Marshal(publishPayload)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions/"+defID+"/publish", bytes.NewReader(pubBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	startPayload := map[string]any{
		"tenant_id":      2002,
		"definition_id":  defID,
		"input":          map[string]any{"ref": "HTTP-RUNTIME-1"},
		"correlation_id": "http-control",
	}
	startBody, _ := json.Marshal(startPayload)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/admin/workflows/instances", bytes.NewReader(startBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code)

	var startResp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &startResp))
	instanceID, _ := startResp.Data["uuid"].(string)
	require.NotEmpty(t, instanceID)

	instanceUUID := uuid.MustParse(instanceID)

	var agentStep modelworkflow.WorkflowStepRecord
	require.NoError(t, env.DB.
		Where("instance_uuid = ? AND step_id = ?", instanceUUID, "agent_step").
		First(&agentStep).Error)
	require.NoError(t, env.DB.
		Model(&modelworkflow.WorkflowStepRecord{}).
		Where("id = ?", agentStep.ID).
		Updates(map[string]any{
			"state":          "failed",
			"attempt":        1,
			"failure_reason": "manual failure",
		}).Error)

	require.NoError(t, env.DB.
		Model(&modelworkflow.WorkflowInstance{}).
		Where("uuid = ?", instanceUUID).
		Update("state", "waiting").Error)

	actionPayload := map[string]any{
		"tenant_id": 2002,
		"action":    "retry_step",
		"step_id":   "agent_step",
	}
	actionBody, _ := json.Marshal(actionPayload)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/workflows/instances/%s/actions", instanceID), bytes.NewReader(actionBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var actionResp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &actionResp))
	require.Equal(t, 200, actionResp.Code)
	require.Equal(t, "running", actionResp.Data["state"])

	pausePayload := map[string]any{
		"tenant_id": 2002,
		"action":    "pause",
		"reason":    "manual check",
	}
	pauseBody, _ := json.Marshal(pausePayload)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/workflows/instances/%s/actions", instanceID), bytes.NewReader(pauseBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var pauseResp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &pauseResp))
	require.Equal(t, "suspended", pauseResp.Data["state"])

	resumePayload := map[string]any{
		"tenant_id": 2002,
		"action":    "resume",
	}
	resumeBody, _ := json.Marshal(resumePayload)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/workflows/instances/%s/actions", instanceID), bytes.NewReader(resumeBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resumeResp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resumeResp))
	require.Equal(t, "running", resumeResp.Data["state"])
}
