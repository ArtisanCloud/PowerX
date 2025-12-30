package workflowintegration

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const (
	isolationTenantUUIDA = "workflow-tenant-a"
	isolationTenantUUIDB = "workflow-tenant-b"
)

func TestWorkflowTenantIsolation(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	definition, err := env.Service.CreateDefinition(ctx, workflow.CreateDefinitionInput{
		TenantUUID: isolationTenantUUIDA,
		Name:       "tenant-a-demo",
		CreatedBy:  uuid.New(),
		Steps: []workflow.StepDefinition{
			{
				ID:          "start",
				Type:        "system",
				NextStepIDs: []string{"agent_step"},
			},
			{
				ID:          "agent_step",
				Type:        "agent",
				NextStepIDs: []string{"complete"},
				Config: map[string]any{
					"capability": "demo.capability",
					"agent_id":   uuid.New().String(),
				},
			},
			{
				ID:   "complete",
				Type: "system",
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, definition)

	_, err = env.Service.PublishDefinition(ctx, workflow.PublishDefinitionInput{
		TenantUUID:     isolationTenantUUIDA,
		DefinitionUUID: definition.UUID,
		PublishedBy:    uuid.New(),
	})
	require.NoError(t, err)

	instance, err := env.Service.StartInstance(ctx, workflow.StartInstanceInput{
		TenantUUID:     isolationTenantUUIDA,
		DefinitionUUID: definition.UUID,
		Input:          map[string]any{"ref": "tenant-a"},
	})
	require.NoError(t, err)
	require.NotNil(t, instance)

	list, total, err := env.Service.ListInstances(ctx, workflowrepo.InstanceListFilter{
		TenantUUID: isolationTenantUUIDA,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, list, 1)

	listOther, totalOther, err := env.Service.ListInstances(ctx, workflowrepo.InstanceListFilter{
		TenantUUID: isolationTenantUUIDB,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), totalOther)
	require.Empty(t, listOther)

	_, _, err = env.Service.GetInstance(ctx, isolationTenantUUIDB, instance.UUID, false)
	require.Error(t, err)

	_, err = env.Service.ControlInstance(ctx, workflow.ControlInstanceInput{
		TenantUUID:   isolationTenantUUIDB,
		InstanceUUID: instance.UUID,
		Action:       "pause",
	})
	require.Error(t, err)
}
