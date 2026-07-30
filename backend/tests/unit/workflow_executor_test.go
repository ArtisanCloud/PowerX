package workflowunit

import (
	"testing"

	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	"github.com/stretchr/testify/require"
)

func TestValidateStepDefinitions(t *testing.T) {
	steps := []workflowsvc.StepDefinition{
		{ID: "start", Type: "agent", NextStepIDs: []string{"end"}},
		{ID: "end", Type: "system"},
	}
	result, err := workflowsvc.ValidateStepDefinitions(steps)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"start"}, result.StartStepIDs)
}

func TestValidateStepDefinitionsDetectCycle(t *testing.T) {
	steps := []workflowsvc.StepDefinition{
		{ID: "a", Type: "agent", NextStepIDs: []string{"b"}},
		{ID: "b", Type: "system", NextStepIDs: []string{"a"}},
	}
	_, err := workflowsvc.ValidateStepDefinitions(steps)
	require.Error(t, err)
}

func TestValidateStepDefinitionsUnknownReference(t *testing.T) {
	steps := []workflowsvc.StepDefinition{
		{ID: "a", Type: "agent", NextStepIDs: []string{"missing"}},
	}
	_, err := workflowsvc.ValidateStepDefinitions(steps)
	require.Error(t, err)
}

func TestValidateHumanApprovalRequiresReviewConfig(t *testing.T) {
	steps := []workflowsvc.StepDefinition{
		{ID: "review", Type: "human_approval"},
	}
	_, err := workflowsvc.ValidateStepDefinitions(steps)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires config")
}

func TestValidateHumanApprovalRejectsMissingReviewRoute(t *testing.T) {
	steps := []workflowsvc.StepDefinition{
		{
			ID:   "review",
			Type: "human_approval",
			Config: map[string]any{
				"review_type":         "knowledge_publish",
				"approver_policy":     map[string]any{"roles": []any{"knowledge_reviewer"}},
				"review_payload_path": "$.draft",
				"approved_route":      "approved",
			},
		},
	}
	_, err := workflowsvc.ValidateStepDefinitions(steps)
	require.Error(t, err)
	require.Contains(t, err.Error(), "rejected_route")
}

func TestDecisionExecutorRoutesByResult(t *testing.T) {
	router := workflowsvc.NewExecutorRouter()
	step := workflowsvc.StepDefinition{
		ID:   "decide",
		Type: "decision",
		Config: map[string]any{
			"routes": map[string]any{
				"approve": "next-approve",
				"reject":  []any{"next-reject"},
			},
			"default_route": "fallback",
		},
	}
	require.NoError(t, router.Validate(step))

	next, err := router.NextSteps(step, workflowsvc.StepResult{Decision: "approve"})
	require.NoError(t, err)
	require.Equal(t, []string{"next-approve"}, next)

	next, err = router.NextSteps(step, workflowsvc.StepResult{Decision: "unknown"})
	require.NoError(t, err)
	require.Equal(t, []string{"fallback"}, next)
}

func TestParallelExecutorSelectedBranches(t *testing.T) {
	router := workflowsvc.NewExecutorRouter()
	step := workflowsvc.StepDefinition{
		ID:          "split",
		Type:        "parallel",
		NextStepIDs: []string{"a", "b", "c"},
	}
	require.NoError(t, router.Validate(step))

	next, err := router.NextSteps(step, workflowsvc.StepResult{SelectedBranches: []string{"c", "a"}})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"c", "a"}, next)
}

func TestHumanApprovalExecutorRejectsRoute(t *testing.T) {
	router := workflowsvc.NewExecutorRouter()
	step := workflowsvc.StepDefinition{
		ID:   "approval",
		Type: "human_approval",
		Config: map[string]any{
			"review_type":         "knowledge_publish",
			"approver_policy":     map[string]any{"roles": []any{"knowledge_reviewer"}},
			"review_payload_path": "$.draft",
			"approved_route":      "approved",
			"rejected_route":      "rejected",
		},
	}
	require.NoError(t, router.Validate(step))

	next, err := router.NextSteps(step, workflowsvc.StepResult{
		HasApproval: true,
		Approved:    false,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"rejected"}, next)
}
