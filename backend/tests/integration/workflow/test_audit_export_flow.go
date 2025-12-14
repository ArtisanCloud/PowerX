package workflowintegration

import (
	"context"
	"testing"
	"time"

	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const auditExportTenantUUID = "workflow-integration-tenant-4010"

func TestAuditExportFlow(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	steps := []workflowsvc.StepDefinition{
		{ID: "collect", Type: "system", NextStepIDs: []string{"agent_exec"}},
		{ID: "agent_exec", Type: "agent"},
	}

	definition, err := env.Service.CreateDefinition(ctx, workflowsvc.CreateDefinitionInput{
		TenantUUID:  auditExportTenantUUID,
		Name:        "audit-export",
		Description: "integration export",
		CreatedBy:   uuid.New(),
		Steps:       steps,
	})
	require.NoError(t, err)

	_, err = env.Service.PublishDefinition(ctx, workflowsvc.PublishDefinitionInput{
		TenantUUID:     auditExportTenantUUID,
		DefinitionUUID: definition.UUID,
		PublishedBy:    uuid.New(),
	})
	require.NoError(t, err)

	instance, err := env.Service.StartInstance(ctx, workflowsvc.StartInstanceInput{
		TenantUUID:     auditExportTenantUUID,
		DefinitionUUID: definition.UUID,
		CorrelationID:  "export-int-001",
		Input:          map[string]any{"case": "B"},
		Tags:           map[string]string{"priority": "p1"},
	})
	require.NoError(t, err)

	stepRepo := workflowrepo.NewStepRecordRepository(env.DB)
	records, err := stepRepo.ListByInstance(ctx, instance.UUID)
	require.NoError(t, err)
	require.Len(t, records, 1)

	now := time.Now().UTC()
	err = stepRepo.UpdateState(ctx, records[0].ID, "completed", map[string]interface{}{
		"attempt":            1,
		"completed_at":       now,
		"subject_type":       "system",
		"failure_reason":     "",
		"tool_grant_version": int64(0),
	})
	require.NoError(t, err)

	result, err := env.Service.ExportInstances(ctx, workflowsvc.ExportFilter{
		TenantUUID:         auditExportTenantUUID,
		DefinitionUUID:     &definition.UUID,
		IncludeStepDetails: true,
		Format:             workflowsvc.ExportFormatJSON,
	})
	require.NoError(t, err)
	require.Len(t, result.Rows, 1)
	require.Equal(t, "", result.DownloadURL)

	row := result.Rows[0]
	require.Equal(t, instance.UUID.String(), row.InstanceID)
	require.Equal(t, definition.UUID.String(), row.DefinitionID)
	require.Equal(t, "export-int-001", row.CorrelationID)
	require.NotEmpty(t, row.Steps)
	require.Equal(t, "collect", row.Steps[0].StepID)
	require.Equal(t, 1, row.Steps[0].Attempts)

	// When step details are disabled we only expect top-level rows.
	resultNoSteps, err := env.Service.ExportInstances(ctx, workflowsvc.ExportFilter{
		TenantUUID:         auditExportTenantUUID,
		DefinitionUUID:     &definition.UUID,
		IncludeStepDetails: false,
		Format:             workflowsvc.ExportFormatJSON,
	})
	require.NoError(t, err)
	require.Len(t, resultNoSteps.Rows, 1)
	require.Empty(t, resultNoSteps.Rows[0].Steps)
}
