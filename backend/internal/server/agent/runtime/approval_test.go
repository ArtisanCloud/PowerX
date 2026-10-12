package runtime

import (
	"context"
	"testing"
	"time"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestApprovalGateRequiresTrustedEvidenceForApprovedCapability(t *testing.T) {
	tenantUUID := uuid.NewString()
	capabilityUUID := uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{{
		ResourceUUID: capabilityUUID, Kind: ResourceKindCapability, TenantUUID: tenantUUID,
		CapabilityUUID: capabilityUUID, CapabilityID: "com.test.high_risk", DisplayName: "high risk",
		DiscoveryGranted: true, InvocationGranted: true,
		RuntimeContract: CapabilityRuntimeContract{HumanApprovalRequired: true, SideEffectEvidenceSchema: "approval/v1"},
	}}, time.Now())
	require.NoError(t, err)
	task := flowschema.PlanTask{NodeKind: "tooling", Params: map[string]interface{}{"capability_uuid": capabilityUUID.String()}}
	parsed, err := taskCapabilityUUID(task)
	require.NoError(t, err)
	err = requireTaskApproval(context.Background(), snapshot, parsed)
	var approvalErr *ApprovalRequiredError
	require.ErrorAs(t, err, &approvalErr)

	ctx, err := ContextWithCapabilityApprovals(context.Background(), []CapabilityApproval{{ApprovalUUID: uuid.New(), CapabilityUUID: capabilityUUID, EvidenceRef: "approval://evidence/1"}})
	require.NoError(t, err)
	require.NoError(t, requireTaskApproval(ctx, snapshot, parsed))
}

func TestApprovalGateRejectsUnstructuredToolingCapability(t *testing.T) {
	_, err := taskCapabilityUUID(flowschema.PlanTask{NodeKind: "tooling", Params: map[string]interface{}{"capability_uuid": "not-a-uuid"}})
	require.Error(t, err)
}

func TestVerifierClassifiesApprovalAsNeedsInput(t *testing.T) {
	capabilityUUID := uuid.New()
	verdict, err := (DeterministicExecutionVerifier{}).Verify(context.Background(), nil, nil, &ApprovalRequiredError{CapabilityUUID: capabilityUUID})
	require.NoError(t, err)
	require.Equal(t, VerificationNeedsInput, verdict.Class)
	require.Equal(t, "approval.required", verdict.ReasonCode)
	require.Equal(t, []string{"approval_evidence"}, verdict.RequiredInputFields)
	require.Equal(t, []uuid.UUID{capabilityUUID}, verdict.RequiredApprovalCapabilityUUIDs)
}
