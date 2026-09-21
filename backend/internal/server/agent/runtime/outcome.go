package runtime

import (
	"context"
	"errors"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
)

const (
	RuntimeOutcomeCompleted  = "completed"
	RuntimeOutcomePartial    = "partial"
	RuntimeOutcomeNeedsInput = "needs_input"
	RuntimeOutcomeBlocked    = "blocked"
	RuntimeOutcomeFailed     = "failed"
	RuntimeOutcomeCancelled  = "cancelled"
)

type runtimeOutcome struct {
	Status     string
	ReasonCode string
}

type recoveryDecision struct {
	Class      string
	ReasonCode string
}

func classifyRecovery(err error, report *agent.PlanExecutionReport, out *agentschema.ExecutionResult) recoveryDecision {
	verdict, verifyErr := (DeterministicExecutionVerifier{}).Verify(context.Background(), report, out, err)
	if verifyErr != nil {
		return recoveryDecision{Class: VerificationFatal, ReasonCode: "verification.contract_invalid"}
	}
	return recoveryDecisionFromVerdict(verdict)
}

func recoveryDecisionFromVerdict(verdict VerificationVerdict) recoveryDecision {
	if verdict.Class == VerificationFatal && verdict.ReasonCode == "execution.partial" {
		return recoveryDecision{Class: "partial", ReasonCode: verdict.ReasonCode}
	}
	return recoveryDecision{Class: verdict.Class, ReasonCode: verdict.ReasonCode}
}

// runtimeFailureOutcome is the only translation from a terminal runtime
// failure into the public Run outcome. It deliberately uses the stable reason
// code supplied by the caller instead of inspecting an arbitrary error string.
// The original error remains available to server logs and Trace nodes.
func runtimeFailureOutcome(reasonCode string, err error) runtimeOutcome {
	if errors.Is(err, context.Canceled) {
		return runtimeOutcome{Status: RuntimeOutcomeCancelled, ReasonCode: "run.canceled"}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return runtimeOutcome{Status: RuntimeOutcomeFailed, ReasonCode: "run.timeout"}
	}
	switch reasonCode {
	case "authorization.denied", "agent.context_capability_denied":
		return runtimeOutcome{Status: RuntimeOutcomeBlocked, ReasonCode: "authorization.denied"}
	case "input.required":
		return runtimeOutcome{Status: RuntimeOutcomeNeedsInput, ReasonCode: "input.required"}
	case "execution.partial":
		return runtimeOutcome{Status: RuntimeOutcomePartial, ReasonCode: "execution.partial"}
	default:
		return runtimeOutcome{Status: RuntimeOutcomeFailed, ReasonCode: reasonCode}
	}
}

func classifyRuntimeOutcome(report *agent.PlanExecutionReport, out *agentschema.ExecutionResult, err error) runtimeOutcome {
	verdict, verifyErr := (DeterministicExecutionVerifier{}).Verify(context.Background(), report, out, err)
	if verifyErr != nil {
		return runtimeOutcome{Status: RuntimeOutcomeFailed, ReasonCode: "verification.contract_invalid"}
	}
	return runtimeOutcomeFromVerdict(verdict)
}

func runtimeOutcomeFromVerdict(verdict VerificationVerdict) runtimeOutcome {
	if verdict.ReasonCode == "run.canceled" {
		return runtimeOutcome{Status: RuntimeOutcomeCancelled, ReasonCode: verdict.ReasonCode}
	}
	if verdict.ReasonCode == "run.timeout" {
		return runtimeOutcome{Status: RuntimeOutcomeFailed, ReasonCode: verdict.ReasonCode}
	}
	switch verdict.Class {
	case VerificationPass:
		return runtimeOutcome{Status: RuntimeOutcomeCompleted, ReasonCode: verdict.ReasonCode}
	case VerificationNeedsInput:
		return runtimeOutcome{Status: RuntimeOutcomeNeedsInput, ReasonCode: verdict.ReasonCode}
	case VerificationBlocked:
		return runtimeOutcome{Status: RuntimeOutcomeBlocked, ReasonCode: verdict.ReasonCode}
	case VerificationRetryable, VerificationReplaceable:
		return runtimeOutcome{Status: RuntimeOutcomePartial, ReasonCode: verdict.ReasonCode}
	case VerificationFatal:
		if verdict.ReasonCode == "execution.partial" || verdict.ReasonCode == "budget.exhausted" {
			return runtimeOutcome{Status: RuntimeOutcomePartial, ReasonCode: verdict.ReasonCode}
		}
		return runtimeOutcome{Status: RuntimeOutcomeFailed, ReasonCode: verdict.ReasonCode}
	default:
		return runtimeOutcome{Status: RuntimeOutcomeFailed, ReasonCode: "verification.contract_invalid"}
	}
}

func withRuntimeOutcome(metadata map[string]any, outcome runtimeOutcome) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["outcome"] = outcome.Status
	metadata["reason_code"] = outcome.ReasonCode
	return metadata
}

func userActionForOutcome(outcome runtimeOutcome) string {
	switch outcome.Status {
	case RuntimeOutcomeNeedsInput:
		return "provide_required_input"
	case RuntimeOutcomeBlocked:
		if outcome.ReasonCode == "budget.exhausted" {
			return "none"
		}
		return "resolve_authorization_or_approval"
	case RuntimeOutcomePartial:
		return "review_partial_result"
	default:
		return "none"
	}
}
