package workflowintegration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/workflow"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/stretchr/testify/require"
)

func TestExpertKnowledgeCapturePackSeedsPublishedDefinitionWithReviewGate(t *testing.T) {
	assertWorkflowPackReviewGate(t, "expert_knowledge_capture")
}

func TestMarketingKnowledgeCapturePackSeedsPublishedDefinitionWithReviewGate(t *testing.T) {
	assertWorkflowPackReviewGate(t, "marketing_knowledge_capture")
}

func assertWorkflowPackReviewGate(t *testing.T, workflowKey string) {
	t.Helper()
	chdirBackendRoot(t)
	env := testenv.New(t)
	ctx := context.Background()

	result, err := env.Service.SeedWorkflowPacks(ctx, workflow.WorkflowPackSeedInput{
		TenantUUID: testenv.ContractTenantUUID,
		ConfigDir:  "config/workflow_packs",
		Keys:       []string{workflowKey},
	})
	require.NoError(t, err)
	require.Len(t, result.Seeded, 1)

	record := result.Seeded[0]
	definition, err := env.Service.GetDefinition(ctx, testenv.ContractTenantUUID, record.DefinitionUUID, nil)
	require.NoError(t, err)
	require.Equal(t, "published", definition.Status)
	require.Equal(t, workflowKey, definition.WorkflowPackKey)

	var steps []workflow.StepDefinition
	require.NoError(t, json.Unmarshal(definition.StepGraph, &steps))

	var reviewStep *workflow.StepDefinition
	var publishStep *workflow.StepDefinition
	for i := range steps {
		switch steps[i].NodeKind {
		case "human.review":
			reviewStep = &steps[i]
		case "knowledge.publish":
			publishStep = &steps[i]
		}
	}
	require.NotNil(t, reviewStep, "workflow pack must contain human.review gate")
	require.NotNil(t, publishStep, "workflow pack must contain knowledge.publish node")
	require.Equal(t, "knowledge_publish", reviewStep.Config["review_type"])
	require.Equal(t, publishStep.ID, reviewStep.Config["approved_route"])
	require.NotEqual(t, publishStep.ID, reviewStep.Config["rejected_route"])
	require.Contains(t, publishStep.DependsOn, reviewStep.ID)
	require.Equal(t, "review_required", publishStep.Config["publish_policy"])
}

func chdirBackendRoot(t *testing.T) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	current, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { require.NoError(t, os.Chdir(current)) })
}
