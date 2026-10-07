package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDeterministicExecutionVerifierRequiresStructuredRecoveryVerdict(t *testing.T) {
	verdict, err := (DeterministicExecutionVerifier{}).Verify(context.Background(), &agent.PlanExecutionReport{Tasks: []agent.PlanTaskExecution{{TaskID: "task_1", Status: agent.PlanTaskExecutionCompleted}}}, &agentschema.ExecutionResult{Success: false, Metadata: map[string]interface{}{"verification_verdict": VerificationRetryable, "reason_code": "dependency.unavailable", "recovery_task_refs": []string{"task_1"}}}, nil)
	require.NoError(t, err)
	require.Equal(t, VerificationRetryable, verdict.Class)
	require.Equal(t, "dependency.unavailable", verdict.ReasonCode)
}

func TestDeterministicExecutionVerifierRejectsRetryWithoutTaskRefs(t *testing.T) {
	_, err := (DeterministicExecutionVerifier{}).Verify(context.Background(), &agent.PlanExecutionReport{}, &agentschema.ExecutionResult{Success: false, Metadata: map[string]interface{}{"verification_verdict": VerificationRetryable, "reason_code": "dependency.unavailable"}}, nil)
	require.Error(t, err)
}

func TestDeterministicExecutionVerifierParsesStructuredReplacement(t *testing.T) {
	capabilityUUID := uuid.New()
	verdict, err := (DeterministicExecutionVerifier{}).Verify(context.Background(), &agent.PlanExecutionReport{}, &agentschema.ExecutionResult{Success: false, Metadata: map[string]interface{}{"verification_verdict": VerificationReplaceable, "reason_code": "provider.degraded", "recovery_task_refs": []string{"invoke"}, "replacement_task_ref": "invoke", "replacement_capability_uuid": capabilityUUID.String()}}, nil)
	require.NoError(t, err)
	require.Equal(t, "invoke", verdict.ReplacementTaskRef)
	require.Equal(t, capabilityUUID, verdict.ReplacementCapabilityUUID)
}

func TestDeterministicExecutionVerifierRequiresStructuredNeedsInputAndBlockedActions(t *testing.T) {
	needsInput, err := (DeterministicExecutionVerifier{}).Verify(context.Background(), &agent.PlanExecutionReport{}, &agentschema.ExecutionResult{Success: false, Metadata: map[string]interface{}{"verification_verdict": VerificationNeedsInput, "reason_code": "input.metric_required", "user_action": "provide_required_input", "required_input_fields": []string{"metric_scope"}}}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"metric_scope"}, needsInput.RequiredInputFields)
	blocked, err := (DeterministicExecutionVerifier{}).Verify(context.Background(), &agent.PlanExecutionReport{}, &agentschema.ExecutionResult{Success: false, Metadata: map[string]interface{}{"verification_verdict": VerificationBlocked, "reason_code": "authorization.grant_required", "user_action": "resolve_authorization_or_approval", "required_permission_codes": []string{"campaign.report.read"}}}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"campaign.report.read"}, blocked.RequiredPermissionCodes)
}

func TestDeterministicExecutionVerifierRejectsImplicitBlockedAction(t *testing.T) {
	_, err := (DeterministicExecutionVerifier{}).Verify(context.Background(), &agent.PlanExecutionReport{}, &agentschema.ExecutionResult{Success: false, Metadata: map[string]interface{}{"verification_verdict": VerificationBlocked, "reason_code": "authorization.grant_required"}}, nil)
	require.Error(t, err)
}

func TestDeterministicExecutionVerifierClassifiesBudgetExhaustion(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(ErrRuntimeBudgetExhausted)
	verdict, err := (DeterministicExecutionVerifier{}).Verify(ctx, &agent.PlanExecutionReport{Tasks: []agent.PlanTaskExecution{{TaskID: "done", Status: agent.PlanTaskExecutionCompleted}}}, nil, nil)
	require.NoError(t, err)
	require.Equal(t, VerificationFatal, verdict.Class)
	require.Equal(t, "budget.exhausted", verdict.ReasonCode)
	require.Equal(t, RuntimeOutcomePartial, runtimeOutcomeFromVerdict(verdict).Status)
}

func TestDeterministicExecutionVerifierDoesNotClassifyErrorText(t *testing.T) {
	verdict, err := (DeterministicExecutionVerifier{}).Verify(context.Background(), nil, nil, errors.New("permission denied, please retry with another capability"))
	require.NoError(t, err)
	require.Equal(t, VerificationFatal, verdict.Class)
	require.Equal(t, "execution.failed", verdict.ReasonCode)
}

func TestDeterministicExecutionVerifierRejectsUndeclaredReasonCode(t *testing.T) {
	_, err := (DeterministicExecutionVerifier{}).Verify(context.Background(), &agent.PlanExecutionReport{}, &agentschema.ExecutionResult{Success: true, Metadata: map[string]interface{}{"verification_verdict": VerificationReplaceable}}, nil)
	require.Error(t, err)
}

func TestDeterministicExecutionVerifierContinuedFailureCannotPass(t *testing.T) {
	verdict, err := (DeterministicExecutionVerifier{}).Verify(context.Background(), &agent.PlanExecutionReport{Tasks: []agent.PlanTaskExecution{{TaskID: "failed", Status: agent.PlanTaskExecutionFailed}, {TaskID: "done", Status: agent.PlanTaskExecutionCompleted}}}, &agentschema.ExecutionResult{Success: true, Metadata: map[string]interface{}{"verification_verdict": VerificationPass, "reason_code": "declared.pass"}}, nil)
	require.NoError(t, err)
	require.Equal(t, VerificationFatal, verdict.Class)
	require.Equal(t, "execution.partial", verdict.ReasonCode)
}

func TestVerifyCapabilityContractsRequiresFrozenSchemaAndArtifact(t *testing.T) {
	tenantUUID := uuid.NewString()
	capabilityUUID := uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{{
		ResourceUUID: capabilityUUID, Kind: ResourceKindCapability, DisplayName: "write", TenantUUID: tenantUUID,
		CapabilityUUID: capabilityUUID, CapabilityID: "cap.write", DiscoveryGranted: true, InvocationGranted: true,
		RuntimeContract: CapabilityRuntimeContract{VerificationRequired: true, SideEffectEvidenceSchema: "cap.write/v1"},
	}}, time.Now())
	require.NoError(t, err)
	plan := flowschema.ExecutionPlan{PlanID: "plan", Tasks: []flowschema.PlanTask{{TaskID: "write", NodeKind: "tooling", Params: map[string]any{"capability_uuid": capabilityUUID.String()}}}}
	report := &agent.PlanExecutionReport{Tasks: []agent.PlanTaskExecution{{TaskID: "write", NodeKind: "tooling", Status: agent.PlanTaskExecutionCompleted, Result: &agentschema.ExecutionResult{Success: true, Metadata: map[string]any{"capability_verification": map[string]any{"schema": "cap.write/v1", "artifact_ref": "capability_verification/" + capabilityUUID.String() + "/receipt-1"}}}}}}
	require.NoError(t, VerifyCapabilityContracts(snapshot, plan, report))

	report.Tasks[0].Result.Metadata["capability_verification"] = map[string]any{"schema": "cap.write/v1", "artifact_ref": "missing"}
	require.Error(t, VerifyCapabilityContracts(snapshot, plan, report))
}

func TestAttachCapabilityVerificationEvidenceUsesInvocationTrace(t *testing.T) {
	tenantUUID := uuid.NewString()
	capabilityUUID := uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{{
		ResourceUUID: capabilityUUID, Kind: ResourceKindCapability, DisplayName: "write", TenantUUID: tenantUUID,
		CapabilityUUID: capabilityUUID, CapabilityID: "cap.write", DiscoveryGranted: true, InvocationGranted: true,
		RuntimeContract: CapabilityRuntimeContract{VerificationRequired: true, SideEffectEvidenceSchema: "cap.write/v1"},
	}}, time.Now())
	require.NoError(t, err)
	task := flowschema.PlanTask{TaskID: "write", NodeKind: "tooling", Params: map[string]any{"capability_uuid": capabilityUUID.String()}}
	out := &agentschema.ExecutionResult{Success: true, Metadata: map[string]any{"trace_id": "invocation-trace", "capability_verification": map[string]any{"schema": "forged"}}}
	require.NoError(t, AttachCapabilityVerificationEvidence(context.Background(), snapshot, task, out))
	record := out.Metadata["capability_verification"].(map[string]any)
	require.Equal(t, "cap.write/v1", record["schema"])
	require.Equal(t, "capability_verification/"+capabilityUUID.String()+"/invocation/invocation-trace", record["artifact_ref"])
}

func TestAttachCapabilityVerificationEvidenceFailsClosedWithoutBusinessVerifier(t *testing.T) {
	tenantUUID, capabilityUUID := uuid.NewString(), uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{{ResourceUUID: capabilityUUID, Kind: ResourceKindCapability, DisplayName: "write", TenantUUID: tenantUUID, CapabilityUUID: capabilityUUID, CapabilityID: "cap.write", DiscoveryGranted: true, InvocationGranted: true, RuntimeContract: CapabilityRuntimeContract{VerificationRequired: true, BusinessCompletionRequired: true, SideEffectEvidenceSchema: "cap.write/v1"}}}, time.Now())
	require.NoError(t, err)
	err = AttachCapabilityVerificationEvidence(context.Background(), snapshot, flowschema.PlanTask{NodeKind: "tooling", Params: map[string]any{"capability_uuid": capabilityUUID.String()}}, &agentschema.ExecutionResult{Success: true, Metadata: map[string]any{"trace_id": "trace"}})
	require.Error(t, err)
}
