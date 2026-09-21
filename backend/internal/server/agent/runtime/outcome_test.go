package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/stretchr/testify/require"
)

func TestClassifyRuntimeOutcomeContinuedFailureIsPartial(t *testing.T) {
	report := &agent.PlanExecutionReport{Tasks: []agent.PlanTaskExecution{
		{TaskID: "failed", Status: agent.PlanTaskExecutionFailed},
		{TaskID: "completed", Status: agent.PlanTaskExecutionCompleted},
	}}
	got := classifyRuntimeOutcome(report, &agentschema.ExecutionResult{Success: true}, nil)
	require.Equal(t, RuntimeOutcomePartial, got.Status)
	require.Equal(t, "execution.partial", got.ReasonCode)
}

func TestClassifyRuntimeOutcomeAwaitingParamsNeedsInput(t *testing.T) {
	report := &agent.PlanExecutionReport{Tasks: []agent.PlanTaskExecution{{TaskID: "collect", Status: agent.PlanTaskExecutionCompleted}}}
	got := classifyRuntimeOutcome(report, &agentschema.ExecutionResult{Success: true, Data: map[string]any{"status": "awaiting_params"}}, nil)
	require.Equal(t, RuntimeOutcomeNeedsInput, got.Status)
	require.Equal(t, "input.required", got.ReasonCode)
}

func TestClassifyRuntimeOutcomeCancellation(t *testing.T) {
	got := classifyRuntimeOutcome(nil, nil, context.Canceled)
	require.Equal(t, RuntimeOutcomeCancelled, got.Status)
	require.Equal(t, "run.canceled", got.ReasonCode)
}

func TestClassifyRecoveryDoesNotRetryPartialFailure(t *testing.T) {
	report := &agent.PlanExecutionReport{Tasks: []agent.PlanTaskExecution{{Status: agent.PlanTaskExecutionFailed}, {Status: agent.PlanTaskExecutionCompleted}}}
	got := classifyRecovery(nil, report, nil)
	require.Equal(t, "partial", got.Class)
	require.Equal(t, "execution.partial", got.ReasonCode)
}

func TestRuntimeFailureOutcomeDoesNotInspectErrorText(t *testing.T) {
	got := runtimeFailureOutcome("authorization.denied", errors.New("arbitrary backend text containing permission"))
	require.Equal(t, RuntimeOutcomeBlocked, got.Status)
	require.Equal(t, "authorization.denied", got.ReasonCode)
}

func TestRuntimeFailureOutcomeClassifiesResponseContractRejection(t *testing.T) {
	got := runtimeFailureOutcome("final.response_contract_invalid", errors.New("schema rejected"))
	require.Equal(t, RuntimeOutcomeFailed, got.Status)
	require.Equal(t, "final.response_contract_invalid", got.ReasonCode)
}
