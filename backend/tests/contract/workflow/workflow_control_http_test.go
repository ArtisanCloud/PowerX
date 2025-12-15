//go:build ignore

package workflowcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	workflowhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/workflow"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const workflowControlTenantUUID = "6a3f8402-896d-4e2a-9f04-55ac3e59f849"

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
		ctx := reqctx.WithTenantUUID(c.Request.Context(), workflowControlTenantUUID)
		c.Request = c.Request.WithContext(ctx)
		reqctx.CopyCtxToGin(c)
		c.Next()
	})

	deps := &shared.Deps{
		Workflow: &shared.WorkflowDeps{
			Service: env.Service,
		},
	}
	workflowhttp.RegisterAPIRoutes(public, protected, deps)

	defPayload := map[string]any{
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
	req := httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := serveWorkflowRequest(t, engine, req, workflowControlTenantUUID)
	require.Equal(t, http.StatusCreated, rr.Code)

	var createResp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &createResp))
	defID, _ := createResp.Data["uuid"].(string)
	require.NotEmpty(t, defID)

	publishPayload := map[string]any{}
	pubBody, _ := json.Marshal(publishPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/admin/workflows/definitions/"+defID+"/publish", bytes.NewReader(pubBody))
	req.Header.Set("Content-Type", "application/json")
	rr = serveWorkflowRequest(t, engine, req, workflowControlTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code)

	startPayload := map[string]any{
		"definition_id":  defID,
		"input":          map[string]any{"ref": "HTTP-RUNTIME-1"},
		"correlation_id": "http-control",
	}
	startBody, _ := json.Marshal(startPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/admin/workflows/instances", bytes.NewReader(startBody))
	req.Header.Set("Content-Type", "application/json")
	rr = serveWorkflowRequest(t, engine, req, workflowControlTenantUUID)
	require.Equal(t, http.StatusAccepted, rr.Code)

	var startResp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &startResp))
	instanceID, _ := startResp.Data["uuid"].(string)
	require.NotEmpty(t, instanceID)
	require.Equal(t, workflowControlTenantUUID, startResp.Data["tenant_uuid"])

	instanceUUID := uuid.MustParse(instanceID)

	agentStep := modelworkflow.WorkflowStepRecord{
		InstanceUUID:   instanceUUID,
		StepID:         "agent_step",
		Type:           "agent",
		SubjectType:    "agent",
		State:          "queued",
		ScheduledAt:    time.Now().UTC(),
		LastTransition: time.Now().UTC(),
	}
	require.NoError(t, env.DB.Create(&agentStep).Error)
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
		"action":  "retry_step",
		"step_id": "agent_step",
	}
	actionBody, _ := json.Marshal(actionPayload)
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/workflows/instances/%s/actions", instanceID), bytes.NewReader(actionBody))
	req.Header.Set("Content-Type", "application/json")
	rr = serveWorkflowRequest(t, engine, req, workflowControlTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code)

	var actionResp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &actionResp))
	require.Equal(t, 200, actionResp.Code)
	require.Equal(t, "running", actionResp.Data["state"])
	require.Equal(t, workflowControlTenantUUID, actionResp.Data["tenant_uuid"])

	pausePayload := map[string]any{
		"action": "pause",
		"reason": "manual check",
	}
	pauseBody, _ := json.Marshal(pausePayload)
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/workflows/instances/%s/actions", instanceID), bytes.NewReader(pauseBody))
	req.Header.Set("Content-Type", "application/json")
	rr = serveWorkflowRequest(t, engine, req, workflowControlTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code)

	var pauseResp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &pauseResp))
	require.Equal(t, "suspended", pauseResp.Data["state"])
	require.Equal(t, workflowControlTenantUUID, pauseResp.Data["tenant_uuid"])

	resumePayload := map[string]any{
		"action": "resume",
	}
	resumeBody, _ := json.Marshal(resumePayload)
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/admin/workflows/instances/%s/actions", instanceID), bytes.NewReader(resumeBody))
	req.Header.Set("Content-Type", "application/json")
	rr = serveWorkflowRequest(t, engine, req, workflowControlTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code)

	var resumeResp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resumeResp))
	require.Equal(t, "running", resumeResp.Data["state"])
	require.Equal(t, workflowControlTenantUUID, resumeResp.Data["tenant_uuid"])
}
