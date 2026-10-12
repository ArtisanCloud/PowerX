package runtime

import (
	"context"
	"testing"
	"time"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestContinuationReplannerCreatesIndependentContinuation(t *testing.T) {
	snapshot, err := NewResourceSnapshot(uuid.NewString(), uuid.New(), nil, time.Now())
	require.NoError(t, err)
	input := flowschema.ExecutionPlan{
		PlanID: "plan_0",
		Tasks: []flowschema.PlanTask{{
			TaskID:    "follow_up",
			Params:    map[string]interface{}{"nested": map[string]any{"value": "original"}},
			ParamRefs: map[string]string{"metric": "{{task.read.output.metric}}"},
		}},
	}
	reason, next, err := (ContinuationReplanner{}).Replan(context.Background(), PlanRevisionRequest{Snapshot: snapshot, Observations: []ResourceObservation{{ObservationUUID: uuid.New(), ResourceUUID: uuid.New(), Summary: "redacted"}}, CurrentPlan: input})
	require.NoError(t, err)
	require.Equal(t, "observation.continuation", reason)
	require.Equal(t, "plan_0_continuation", next.PlanID)
	next.Tasks[0].Params["nested"].(map[string]any)["value"] = "mutated"
	next.Tasks[0].ParamRefs["metric"] = "changed"
	require.Equal(t, "original", input.Tasks[0].Params["nested"].(map[string]any)["value"])
	require.Equal(t, "{{task.read.output.metric}}", input.Tasks[0].ParamRefs["metric"])
}

func TestContinuationReplannerRejectsMissingObservation(t *testing.T) {
	snapshot, err := NewResourceSnapshot(uuid.NewString(), uuid.New(), nil, time.Now())
	require.NoError(t, err)
	_, _, err = (ContinuationReplanner{}).Replan(context.Background(), PlanRevisionRequest{Snapshot: snapshot, CurrentPlan: flowschema.ExecutionPlan{PlanID: "plan_0", Tasks: []flowschema.PlanTask{{TaskID: "follow_up"}}}})
	require.Error(t, err)
}

func TestSemanticReplannerAppendsOnlySnapshotGrantedDirective(t *testing.T) {
	tenantUUID := uuid.NewString()
	capabilityUUID := uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{{ResourceUUID: capabilityUUID, Kind: ResourceKindCapability, DisplayName: "follow up", TenantUUID: tenantUUID, CapabilityUUID: capabilityUUID, CapabilityID: "com.example.follow_up", DiscoveryGranted: true, InvocationGranted: true}}, time.Now())
	require.NoError(t, err)
	reason, next, err := (SemanticReplanner{}).Replan(context.Background(), PlanRevisionRequest{Snapshot: snapshot, CurrentPlan: flowschema.ExecutionPlan{PlanID: "plan_0", Tasks: []flowschema.PlanTask{{TaskID: "observe"}}}, Observations: []ResourceObservation{{ObservationUUID: uuid.New(), ResourceUUID: uuid.New(), ReplanDirective: &ReplanDirective{Action: "append_authorized_capability", CapabilityUUID: capabilityUUID, TaskID: "follow_up", DependsOn: []string{"observe"}}}}})
	require.NoError(t, err)
	require.Equal(t, "observation.semantic_continuation", reason)
	require.Equal(t, "plan_0_semantic_continuation", next.PlanID)
	require.Equal(t, capabilityUUID.String(), next.Tasks[1].Params["capability_uuid"])
	_, _, err = (SemanticReplanner{}).Replan(context.Background(), PlanRevisionRequest{Snapshot: snapshot, CurrentPlan: flowschema.ExecutionPlan{PlanID: "plan_0", Tasks: []flowschema.PlanTask{{TaskID: "observe"}}}, Observations: []ResourceObservation{{ObservationUUID: uuid.New(), ReplanDirective: &ReplanDirective{Action: "append_authorized_capability", CapabilityUUID: uuid.New(), TaskID: "outside"}}}})
	require.Error(t, err)
}

func TestSemanticReplannerReplacesOnlyExistingToolingTask(t *testing.T) {
	tenantUUID := uuid.NewString()
	primaryUUID := uuid.New()
	replacementUUID := uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{
		{ResourceUUID: primaryUUID, Kind: ResourceKindCapability, DisplayName: "primary", TenantUUID: tenantUUID, CapabilityUUID: primaryUUID, CapabilityID: "cap.primary", DiscoveryGranted: true, InvocationGranted: true},
		{ResourceUUID: replacementUUID, Kind: ResourceKindCapability, DisplayName: "replacement", TenantUUID: tenantUUID, CapabilityUUID: replacementUUID, CapabilityID: "cap.replacement", DiscoveryGranted: true, InvocationGranted: true},
	}, time.Now())
	require.NoError(t, err)
	plan := flowschema.ExecutionPlan{PlanID: "plan_0", Tasks: []flowschema.PlanTask{{TaskID: "call", NodeKind: "tooling", NodeRef: "cap.primary", Params: map[string]any{"capability_uuid": primaryUUID.String()}}}}
	_, next, err := (SemanticReplanner{}).Replan(context.Background(), PlanRevisionRequest{Snapshot: snapshot, CurrentPlan: plan, Observations: []ResourceObservation{{ObservationUUID: uuid.New(), ResourceUUID: primaryUUID, ReplanDirective: &ReplanDirective{Action: "replace_authorized_capability", CapabilityUUID: replacementUUID, TaskID: "call"}}}})
	require.NoError(t, err)
	require.Len(t, next.Tasks, 1)
	require.Equal(t, "cap.replacement", next.Tasks[0].NodeRef)
	require.Equal(t, replacementUUID.String(), next.Tasks[0].Params["capability_uuid"])

	_, _, err = (SemanticReplanner{}).Replan(context.Background(), PlanRevisionRequest{Snapshot: snapshot, CurrentPlan: plan, Observations: []ResourceObservation{{ObservationUUID: uuid.New(), ResourceUUID: primaryUUID, ReplanDirective: &ReplanDirective{Action: "replace_authorized_capability", CapabilityUUID: replacementUUID, TaskID: "missing"}}}})
	require.Error(t, err)
}
