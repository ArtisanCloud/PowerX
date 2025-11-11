//go:build ignore

package workflowcontract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	workflowhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/workflow"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestWorkflowExportHTTP(t *testing.T) {
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

	ctx := context.Background()

	definition, err := env.Service.CreateDefinition(ctx, workflowsvc.CreateDefinitionInput{
		TenantID:    2001,
		Name:        "export-http",
		Description: "http export contract",
		CreatedBy:   uuid.New(),
		Steps: []workflowsvc.StepDefinition{
			{ID: "ingest", Type: "system", NextStepIDs: []string{"agent_review"}},
			{ID: "agent_review", Type: "agent"},
		},
	})
	require.NoError(t, err)

	_, err = env.Service.PublishDefinition(ctx, workflowsvc.PublishDefinitionInput{
		TenantID:       2001,
		DefinitionUUID: definition.UUID,
		PublishedBy:    uuid.New(),
	})
	require.NoError(t, err)

	instance, err := env.Service.StartInstance(ctx, workflowsvc.StartInstanceInput{
		TenantID:       2001,
		DefinitionUUID: definition.UUID,
		CorrelationID:  "audit-001",
		Input:          map[string]any{"topic": "compliance"},
	})
	require.NoError(t, err)

	stepRepo := workflowrepo.NewStepRecordRepository(env.DB)
	records, err := stepRepo.ListByInstance(ctx, instance.UUID)
	require.NoError(t, err)
	require.NotEmpty(t, records)

	now := time.Now().UTC()
	agentID := uuid.New()

	err = stepRepo.UpdateState(ctx, records[0].ID, "completed", map[string]interface{}{
		"attempt":            2,
		"subject_uuid":       agentID,
		"tool_grant_id":      "grant-http",
		"tool_grant_version": int64(3),
		"failure_reason":     "",
		"completed_at":       now,
	})
	require.NoError(t, err)

	_, err = stepRepo.AppendRecord(ctx, &modelworkflow.WorkflowStepRecord{
		InstanceUUID:   instance.UUID,
		StepID:         "agent_review",
		Type:           "agent",
		State:          "failed",
		SubjectType:    "agent",
		SubjectUUID:    agentID,
		Attempt:        1,
		FailureReason:  "timeout",
		ScheduledAt:    now.Add(2 * time.Minute),
		LastTransition: now.Add(2 * time.Minute),
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/workflows/instances/export?tenant_id=%d&format=json", 2001), nil)
	req.Header.Set("Authorization", "Bearer token")

	rr := httptest.NewRecorder()
	engine.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp exportHTTPResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Equal(t, 200, resp.Code)
	require.NotEmpty(t, resp.Data.Rows)

	row := resp.Data.Rows[0]
	require.Equal(t, instance.UUID.String(), row.InstanceID)
	require.Equal(t, definition.UUID.String(), row.DefinitionID)
	require.Equal(t, "audit-001", row.CorrelationID)
	require.NotEmpty(t, row.Steps)

	var hasAgent bool
	for _, step := range row.Steps {
		if step.StepID == "agent_review" {
			hasAgent = true
			require.Equal(t, "failed", step.State)
			require.Equal(t, "agent", step.SubjectType)
			require.Equal(t, "timeout", step.LastError)
		}
	}
	require.True(t, hasAgent, "expected agent_review step in export payload")
}

type exportHTTPResponse struct {
	Code int `json:"code"`
	Data struct {
		Rows []struct {
			InstanceID        string           `json:"instance_id"`
			DefinitionID      string           `json:"definition_id"`
			DefinitionVersion int32            `json:"definition_version"`
			State             string           `json:"state"`
			CorrelationID     string           `json:"correlation_id"`
			Steps             []exportHTTPStep `json:"steps"`
		} `json:"rows"`
	} `json:"data"`
}

type exportHTTPStep struct {
	StepID           string `json:"step_id"`
	Type             string `json:"type"`
	State            string `json:"state"`
	SubjectType      string `json:"subject_type"`
	SubjectID        string `json:"subject_id"`
	Attempts         int    `json:"attempts"`
	ToolGrantVersion int64  `json:"tool_grant_version"`
	LastError        string `json:"last_error"`
}
