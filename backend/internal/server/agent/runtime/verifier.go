package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
)

const (
	VerificationPass        = "pass"
	VerificationRetryable   = "retryable"
	VerificationReplaceable = "replaceable"
	VerificationNeedsInput  = "needs_input"
	VerificationBlocked     = "blocked"
	VerificationFatal       = "fatal"
)

// VerificationVerdict is the sole recovery input for the runtime loop. A
// non-pass verdict must carry a stable reason code; errors are never parsed.
type VerificationVerdict struct {
	Class                           string      `json:"class"`
	ReasonCode                      string      `json:"reason_code"`
	TaskRefs                        []string    `json:"task_refs,omitempty"`
	UserAction                      string      `json:"user_action,omitempty"`
	RequiredInputFields             []string    `json:"required_input_fields,omitempty"`
	RequiredPermissionCodes         []string    `json:"required_permission_codes,omitempty"`
	RequiredApprovalCapabilityUUIDs []uuid.UUID `json:"required_approval_capability_uuids,omitempty"`
	ApprovalRequestUUID             uuid.UUID   `json:"approval_request_uuid,omitempty"`
	ReplacementTaskRef              string      `json:"replacement_task_ref,omitempty"`
	ReplacementCapabilityUUID       uuid.UUID   `json:"replacement_capability_uuid,omitempty"`
}

type ExecutionVerifier interface {
	Verify(context.Context, *agent.PlanExecutionReport, *agentschema.ExecutionResult, error) (VerificationVerdict, error)
}

// DeterministicExecutionVerifier recognizes only declared result metadata and
// runtime state. Contract-specific verifiers can be registered later without
// granting the model authority to classify arbitrary text.
type DeterministicExecutionVerifier struct{}

// AttachCapabilityVerificationEvidence derives verification evidence at the
// Runtime boundary after a successful tooling invocation. It deliberately
// overwrites any executor/model supplied value and anchors the artifact to the
// actual capability invocation trace rather than its textual response.
func AttachCapabilityVerificationEvidence(ctx context.Context, snapshot *ResourceSnapshot, task flowschema.PlanTask, out *agentschema.ExecutionResult) error {
	if snapshot == nil || out == nil || !out.Success || !strings.EqualFold(strings.TrimSpace(task.NodeKind), "tooling") {
		return nil
	}
	capabilityUUID, err := taskCapabilityUUID(task)
	if err != nil {
		return err
	}
	capability, err := snapshot.RequireInvocable(capabilityUUID)
	if err != nil {
		return err
	}
	contract := capability.RuntimeContract
	if !contract.VerificationRequired {
		return nil
	}
	if strings.TrimSpace(contract.SideEffectEvidenceSchema) == "" {
		return fmt.Errorf("verified capability contract is missing side_effect_evidence_schema")
	}
	if out.Metadata == nil {
		return fmt.Errorf("tooling result metadata is required for verification evidence")
	}
	traceID := strings.TrimSpace(anyToString(out.Metadata["trace_id"]))
	if traceID == "" {
		return fmt.Errorf("tooling invocation trace_id is required for verification evidence")
	}
	out.Metadata["capability_verification"] = map[string]any{
		"schema":       contract.SideEffectEvidenceSchema,
		"artifact_ref": "capability_verification/" + capabilityUUID.String() + "/invocation/" + traceID,
	}
	if contract.BusinessCompletionRequired {
		registry, ok := CompletionVerifierRegistryFromContext(ctx)
		if !ok {
			return fmt.Errorf("completion verifier registry is required")
		}
		artifactRef, err := registry.Verify(ctx, capability, task, out)
		if err != nil || strings.TrimSpace(artifactRef) == "" {
			return fmt.Errorf("verify capability business completion: %w", err)
		}
		out.Metadata["capability_business_completion"] = map[string]any{"artifact_ref": artifactRef}
	}
	return nil
}

// VerifyCapabilityContracts makes a frozen Capability contract authoritative
// for completed tooling tasks. A successful executor response is insufficient
// when the contract requires verification: it must carry a structured evidence
// object whose schema and artifact namespace identify the invoked capability.
func VerifyCapabilityContracts(snapshot *ResourceSnapshot, plan flowschema.ExecutionPlan, report *agent.PlanExecutionReport) error {
	if snapshot == nil || report == nil {
		return nil
	}
	byTaskID := make(map[string]flowschema.PlanTask, len(plan.Tasks))
	for _, task := range plan.Tasks {
		byTaskID[task.TaskID] = task
	}
	for _, execution := range report.Tasks {
		if execution.Status != agent.PlanTaskExecutionCompleted || !strings.EqualFold(strings.TrimSpace(execution.NodeKind), "tooling") {
			continue
		}
		task, found := byTaskID[execution.TaskID]
		if !found || task.Params == nil {
			return fmt.Errorf("completed tooling task is not present in submitted plan")
		}
		capabilityUUID, err := uuid.Parse(strings.TrimSpace(anyToString(task.Params["capability_uuid"])))
		if err != nil || capabilityUUID == uuid.Nil {
			return fmt.Errorf("completed tooling task capability_uuid is invalid")
		}
		capability, err := snapshot.RequireInvocable(capabilityUUID)
		if err != nil {
			return fmt.Errorf("completed tooling task capability is outside frozen snapshot: %w", err)
		}
		contract := capability.RuntimeContract
		if !contract.VerificationRequired {
			continue
		}
		if strings.TrimSpace(contract.SideEffectEvidenceSchema) == "" {
			return fmt.Errorf("verified capability contract is missing side_effect_evidence_schema")
		}
		if execution.Result == nil || execution.Result.Metadata == nil {
			return fmt.Errorf("capability verification evidence is required")
		}
		record, ok := execution.Result.Metadata["capability_verification"].(map[string]any)
		if !ok {
			return fmt.Errorf("capability verification evidence must be a structured object")
		}
		schema := strings.TrimSpace(anyToString(record["schema"]))
		artifactRef := strings.TrimSpace(anyToString(record["artifact_ref"]))
		if schema != contract.SideEffectEvidenceSchema {
			return fmt.Errorf("capability verification evidence schema does not match frozen contract")
		}
		prefix := "capability_verification/" + capabilityUUID.String() + "/"
		if !strings.HasPrefix(artifactRef, prefix) || strings.TrimPrefix(artifactRef, prefix) == "" {
			return fmt.Errorf("capability verification evidence artifact_ref is invalid")
		}
	}
	return nil
}

func (DeterministicExecutionVerifier) Verify(ctx context.Context, report *agent.PlanExecutionReport, out *agentschema.ExecutionResult, runErr error) (VerificationVerdict, error) {
	var approvalErr *ApprovalRequiredError
	if errors.As(runErr, &approvalErr) && approvalErr.CapabilityUUID != uuid.Nil {
		return VerificationVerdict{
			Class:                           VerificationNeedsInput,
			ReasonCode:                      "approval.required",
			TaskRefs:                        incompleteTaskRefs(report),
			UserAction:                      "provide_required_input",
			RequiredInputFields:             []string{"approval_evidence"},
			RequiredApprovalCapabilityUUIDs: []uuid.UUID{approvalErr.CapabilityUUID},
			ApprovalRequestUUID:             approvalErr.ApprovalUUID,
		}, nil
	}
	if errors.Is(context.Cause(ctx), ErrRuntimeBudgetExhausted) || errors.Is(runErr, ErrRuntimeBudgetExhausted) {
		if report != nil && report.HasCompletedTasks() {
			return VerificationVerdict{Class: VerificationFatal, ReasonCode: "budget.exhausted", TaskRefs: failedTaskRefs(report)}, nil
		}
		return VerificationVerdict{Class: VerificationBlocked, ReasonCode: "budget.exhausted", TaskRefs: incompleteTaskRefs(report)}, nil
	}
	if modelReason := modelFailureReason(runErr); modelReason != "" {
		return VerificationVerdict{Class: VerificationFatal, ReasonCode: modelReason, TaskRefs: failedTaskRefs(report)}, nil
	}
	if errors.Is(runErr, context.Canceled) {
		return VerificationVerdict{Class: VerificationFatal, ReasonCode: "run.canceled", TaskRefs: failedTaskRefs(report)}, nil
	}
	if errors.Is(runErr, context.DeadlineExceeded) {
		return VerificationVerdict{Class: VerificationFatal, ReasonCode: "run.timeout", TaskRefs: failedTaskRefs(report)}, nil
	}
	if isAwaitingParamsResult(out) {
		return VerificationVerdict{Class: VerificationNeedsInput, ReasonCode: "input.required", TaskRefs: incompleteTaskRefs(report)}, nil
	}
	if verdict, ok, err := declaredVerificationVerdict(out, report); err != nil || ok {
		if err == nil && verdict.Class == VerificationPass && report != nil && report.HasFailures() {
			return VerificationVerdict{Class: VerificationFatal, ReasonCode: "execution.partial", TaskRefs: failedTaskRefs(report)}, nil
		}
		return verdict, err
	}
	if report != nil && report.HasFailures() {
		if report.HasCompletedTasks() {
			return VerificationVerdict{Class: VerificationFatal, ReasonCode: "execution.partial", TaskRefs: failedTaskRefs(report)}, nil
		}
		return VerificationVerdict{Class: VerificationFatal, ReasonCode: "execution.all_tasks_failed", TaskRefs: failedTaskRefs(report)}, nil
	}
	if runErr != nil {
		return VerificationVerdict{Class: VerificationFatal, ReasonCode: "execution.failed", TaskRefs: failedTaskRefs(report)}, nil
	}
	if report == nil || out == nil || !out.Success {
		return VerificationVerdict{Class: VerificationFatal, ReasonCode: "execution.no_verified_result", TaskRefs: incompleteTaskRefs(report)}, nil
	}
	return VerificationVerdict{Class: VerificationPass, ReasonCode: "execution.verified_pending"}, nil
}

func declaredVerificationVerdict(out *agentschema.ExecutionResult, report *agent.PlanExecutionReport) (VerificationVerdict, bool, error) {
	if out == nil || out.Metadata == nil {
		return VerificationVerdict{}, false, nil
	}
	class := strings.TrimSpace(anyToString(out.Metadata["verification_verdict"]))
	if class == "" {
		return VerificationVerdict{}, false, nil
	}
	switch class {
	case VerificationPass, VerificationRetryable, VerificationReplaceable, VerificationNeedsInput, VerificationBlocked, VerificationFatal:
	default:
		return VerificationVerdict{}, true, fmt.Errorf("unsupported verification verdict: %s", class)
	}
	reasonCode := strings.TrimSpace(anyToString(out.Metadata["reason_code"]))
	if reasonCode == "" {
		return VerificationVerdict{}, true, fmt.Errorf("verification reason_code is required")
	}
	if class == VerificationPass && !out.Success {
		return VerificationVerdict{}, true, fmt.Errorf("successful verification verdict requires a successful execution result")
	}
	taskRefs := failedTaskRefs(report)
	if class == VerificationRetryable || class == VerificationReplaceable {
		taskRefs = normalizedTaskRefs(out.Metadata["recovery_task_refs"])
		if len(taskRefs) == 0 {
			return VerificationVerdict{}, true, fmt.Errorf("recovery_task_refs is required for %s verification verdict", class)
		}
	}
	verdict := VerificationVerdict{Class: class, ReasonCode: reasonCode, TaskRefs: taskRefs}
	if class == VerificationNeedsInput || class == VerificationBlocked {
		verdict.UserAction = strings.TrimSpace(anyToString(out.Metadata["user_action"]))
		expectedAction := "provide_required_input"
		if class == VerificationBlocked {
			expectedAction = "resolve_authorization_or_approval"
			verdict.RequiredPermissionCodes = normalizedTaskRefs(out.Metadata["required_permission_codes"])
			if len(verdict.RequiredPermissionCodes) == 0 {
				return VerificationVerdict{}, true, fmt.Errorf("required_permission_codes is required for blocked verification verdict")
			}
		} else {
			verdict.RequiredInputFields = normalizedTaskRefs(out.Metadata["required_input_fields"])
			if len(verdict.RequiredInputFields) == 0 {
				return VerificationVerdict{}, true, fmt.Errorf("required_input_fields is required for needs_input verification verdict")
			}
		}
		if verdict.UserAction != expectedAction {
			return VerificationVerdict{}, true, fmt.Errorf("user_action is invalid for %s verification verdict", class)
		}
	}
	if class == VerificationReplaceable {
		verdict.ReplacementTaskRef = strings.TrimSpace(anyToString(out.Metadata["replacement_task_ref"]))
		parsedUUID, parseErr := uuid.Parse(strings.TrimSpace(anyToString(out.Metadata["replacement_capability_uuid"])))
		if verdict.ReplacementTaskRef == "" || parseErr != nil {
			return VerificationVerdict{}, true, fmt.Errorf("replacement_task_ref and replacement_capability_uuid are required for replaceable verification verdict")
		}
		verdict.ReplacementCapabilityUUID = parsedUUID
		found := false
		for _, ref := range taskRefs {
			if ref == verdict.ReplacementTaskRef {
				found = true
				break
			}
		}
		if !found {
			return VerificationVerdict{}, true, fmt.Errorf("replacement_task_ref must be declared in recovery_task_refs")
		}
	}
	return verdict, true, nil
}

func normalizedTaskRefs(raw any) []string {
	values, ok := raw.([]string)
	if !ok {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	refs := make([]string, 0, len(values))
	for _, value := range values {
		ref := strings.TrimSpace(value)
		if ref == "" {
			return nil
		}
		if _, exists := seen[ref]; exists {
			return nil
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
}

func failedTaskRefs(report *agent.PlanExecutionReport) []string {
	if report == nil {
		return nil
	}
	refs := make([]string, 0)
	for _, task := range report.Tasks {
		if task.Status == agent.PlanTaskExecutionFailed && strings.TrimSpace(task.TaskID) != "" {
			refs = append(refs, task.TaskID)
		}
	}
	return refs
}

func incompleteTaskRefs(report *agent.PlanExecutionReport) []string {
	if report == nil {
		return nil
	}
	refs := make([]string, 0)
	for _, task := range report.Tasks {
		if task.Status != agent.PlanTaskExecutionCompleted && strings.TrimSpace(task.TaskID) != "" {
			refs = append(refs, task.TaskID)
		}
	}
	return refs
}
