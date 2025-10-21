package workflowintegration

import (
	"context"
	"testing"

	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDefinitionLaunchFlow(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	steps := []workflowsvc.StepDefinition{
		{ID: "prepare", Type: "agent", NextStepIDs: []string{"execute"}},
		{ID: "execute", Type: "system"},
	}

	def, err := env.Service.CreateDefinition(ctx, workflowsvc.CreateDefinitionInput{
		TenantID:    1001,
		Name:        "integration",
		Description: "service level flow",
		CreatedBy:   uuid.New(),
		Steps:       steps,
	})
	require.NoError(t, err)

	_, err = env.Service.PublishDefinition(ctx, workflowsvc.PublishDefinitionInput{
		TenantID:       1001,
		DefinitionUUID: def.UUID,
		PublishedBy:    uuid.New(),
	})
	require.NoError(t, err)

	instance, err := env.Service.StartInstance(ctx, workflowsvc.StartInstanceInput{
		TenantID:       1001,
		DefinitionUUID: def.UUID,
		Input:          map[string]any{"case": "A"},
	})
	require.NoError(t, err)
	require.Equal(t, "running", instance.State)

	stepRepo := workflowrepo.NewStepRecordRepository(env.DB)
	records, err := stepRepo.ListByInstance(ctx, instance.UUID)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "prepare", records[0].StepID)
}
