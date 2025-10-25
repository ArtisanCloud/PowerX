package workflowunit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/workflow"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestValidateStepDefinitionsSuccess(t *testing.T) {
	defs := []workflow.StepDefinition{
		{ID: "start", Type: "system", NextStepIDs: []string{"branch"}},
		{ID: "branch", Type: "parallel", NextStepIDs: []string{"agent", "system"}},
		{ID: "agent", Type: "agent", DependsOn: []string{"branch"}},
		{ID: "system", Type: "system", DependsOn: []string{"branch"}},
	}

	result, err := workflow.ValidateStepDefinitions(defs)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"start"}, result.StartStepIDs)
	require.Len(t, result.Adjacency["branch"], 2)
}

func TestValidateStepDefinitionsDuplicate(t *testing.T) {
	defs := []workflow.StepDefinition{
		{ID: "start", Type: "system"},
		{ID: "start", Type: "agent"},
	}
	_, err := workflow.ValidateStepDefinitions(defs)
	require.Error(t, err)
}

func TestValidateStepDefinitionsCycle(t *testing.T) {
	defs := []workflow.StepDefinition{
		{ID: "a", Type: "system", NextStepIDs: []string{"b"}},
		{ID: "b", Type: "system", NextStepIDs: []string{"a"}},
	}
	_, err := workflow.ValidateStepDefinitions(defs)
	require.Error(t, err)
}

func TestParseRetryPolicy(t *testing.T) {
	payload := map[string]any{
		"max_attempts":        5,
		"initial_interval_ms": 15000,
		"backoff_multiplier":  3.0,
		"max_interval_ms":     60000,
		"jitter_ms":           500,
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	policy := workflow.ParseRetryPolicy(datatypes.JSON(raw))
	require.Equal(t, 5, policy.MaxAttempts)
	require.Equal(t, 15*time.Second, policy.InitialInterval)
	require.Equal(t, 3.0, policy.BackoffMultiplier)
	require.Equal(t, 60*time.Second, policy.MaxInterval)
	require.Equal(t, 500*time.Millisecond, policy.Jitter)
}

func TestParseRetryPolicyDefaults(t *testing.T) {
	policy := workflow.ParseRetryPolicy(nil)
	require.Equal(t, 3, policy.MaxAttempts)
	require.Equal(t, 30*time.Second, policy.InitialInterval)
	require.Equal(t, 2.0, policy.BackoffMultiplier)
	require.Equal(t, 5*time.Minute, policy.MaxInterval)
}
