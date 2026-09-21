package runtime

import (
	"context"
	"testing"
	"time"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRetryPlanUsesOnlyDeclaredClosedTasks(t *testing.T) {
	tenantUUID := uuid.NewString()
	prepareUUID := uuid.New()
	retryUUID := uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{{ResourceUUID: prepareUUID, Kind: ResourceKindCapability, DisplayName: "prepare", TenantUUID: tenantUUID, CapabilityUUID: prepareUUID, CapabilityID: "cap.prepare", DiscoveryGranted: true, InvocationGranted: true, RuntimeContract: CapabilityRuntimeContract{RetryMaxAttempts: 1}}, {ResourceUUID: retryUUID, Kind: ResourceKindCapability, DisplayName: "retry", TenantUUID: tenantUUID, CapabilityUUID: retryUUID, CapabilityID: "cap.retry", DiscoveryGranted: true, InvocationGranted: true, RuntimeContract: CapabilityRuntimeContract{RetryMaxAttempts: 1}}}, time.Now())
	require.NoError(t, err)
	current := flowschema.ExecutionPlan{PlanID: "plan_0", Tasks: []flowschema.PlanTask{{TaskID: "prepare", NodeKind: "tooling", Params: map[string]interface{}{"capability_uuid": prepareUUID.String()}}, {TaskID: "retry", NodeKind: "tooling", Params: map[string]interface{}{"capability_uuid": retryUUID.String()}, DependsOn: []string{"prepare"}}, {TaskID: "unrelated"}}}
	next, err := retryPlanFromVerdict(snapshot, current, VerificationVerdict{Class: VerificationRetryable, TaskRefs: []string{"prepare", "retry"}})
	require.NoError(t, err)
	require.Equal(t, "plan_0_retry", next.PlanID)
	require.Equal(t, []string{"prepare", "retry"}, taskRefs(next))
}

func TestRetryPlanRejectsUndeclaredDependencyOrTask(t *testing.T) {
	tenantUUID := uuid.NewString()
	capabilityUUID := uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{{ResourceUUID: capabilityUUID, Kind: ResourceKindCapability, DisplayName: "retry", TenantUUID: tenantUUID, CapabilityUUID: capabilityUUID, CapabilityID: "cap.retry", DiscoveryGranted: true, InvocationGranted: true, RuntimeContract: CapabilityRuntimeContract{RetryMaxAttempts: 1}}}, time.Now())
	require.NoError(t, err)
	current := flowschema.ExecutionPlan{PlanID: "plan_0", Tasks: []flowschema.PlanTask{{TaskID: "prepare", NodeKind: "tooling", Params: map[string]interface{}{"capability_uuid": capabilityUUID.String()}}, {TaskID: "retry", NodeKind: "tooling", Params: map[string]interface{}{"capability_uuid": capabilityUUID.String()}, DependsOn: []string{"prepare"}}}}
	_, err = retryPlanFromVerdict(snapshot, current, VerificationVerdict{Class: VerificationRetryable, TaskRefs: []string{"retry"}})
	require.Error(t, err)
	_, err = retryPlanFromVerdict(snapshot, current, VerificationVerdict{Class: VerificationRetryable, TaskRefs: []string{"outside"}})
	require.Error(t, err)
}

func TestReplacementPlanUsesOnlyFrozenInvocableCapability(t *testing.T) {
	tenantUUID := uuid.NewString()
	originalCapabilityUUID := uuid.New()
	replacementCapabilityUUID := uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{{ResourceUUID: originalCapabilityUUID, Kind: ResourceKindCapability, DisplayName: "original", TenantUUID: tenantUUID, CapabilityUUID: originalCapabilityUUID, CapabilityID: "cap.original", DiscoveryGranted: true, InvocationGranted: true, RuntimeContract: CapabilityRuntimeContract{AlternativeCapabilityUUIDs: []uuid.UUID{replacementCapabilityUUID}}}, {ResourceUUID: replacementCapabilityUUID, Kind: ResourceKindCapability, DisplayName: "replacement", TenantUUID: tenantUUID, CapabilityUUID: replacementCapabilityUUID, CapabilityID: "cap.replacement", DiscoveryGranted: true, InvocationGranted: true}}, time.Now())
	require.NoError(t, err)
	current := flowschema.ExecutionPlan{PlanID: "plan_0", Tasks: []flowschema.PlanTask{{TaskID: "invoke", NodeKind: "tooling", NodeRef: "cap.original", Params: map[string]interface{}{"capability_uuid": originalCapabilityUUID.String(), "capability_id": "cap.original"}}}}
	next, err := replacementPlanFromVerdict(snapshot, current, VerificationVerdict{Class: VerificationReplaceable, TaskRefs: []string{"invoke"}, ReplacementTaskRef: "invoke", ReplacementCapabilityUUID: replacementCapabilityUUID})
	require.NoError(t, err)
	require.Equal(t, "plan_0_replacement", next.PlanID)
	require.Equal(t, replacementCapabilityUUID.String(), next.Tasks[0].Params["capability_uuid"])
	require.Equal(t, "cap.replacement", next.Tasks[0].Params["capability_id"])
}

func TestReplacementPlanRejectsCapabilityOutsideSnapshot(t *testing.T) {
	tenantUUID := uuid.NewString()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), nil, time.Now())
	require.NoError(t, err)
	current := flowschema.ExecutionPlan{PlanID: "plan_0", Tasks: []flowschema.PlanTask{{TaskID: "invoke", NodeKind: "tooling", Params: map[string]interface{}{"capability_uuid": uuid.NewString()}}}}
	_, err = replacementPlanFromVerdict(snapshot, current, VerificationVerdict{Class: VerificationReplaceable, TaskRefs: []string{"invoke"}, ReplacementTaskRef: "invoke", ReplacementCapabilityUUID: uuid.New()})
	require.Error(t, err)
}

func TestSemanticReplacementPlanPersistsTypedObservation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.Exec(`CREATE TABLE public.agent_run_observations (
		id integer primary key, uuid text, created_at datetime, updated_at datetime, deleted_at datetime,
		env text, tenant_uuid text, run_uuid text, snapshot_uuid text, resource_uuid text,
		purpose text, summary text, artifact_ref text, directive json, digest text
	)`).Error)
	tenantUUID := uuid.NewString()
	primaryUUID, replacementUUID := uuid.New(), uuid.New()
	snapshot, err := NewResourceSnapshot(tenantUUID, uuid.New(), []ResourceDescriptor{
		{ResourceUUID: primaryUUID, Kind: ResourceKindCapability, DisplayName: "primary", TenantUUID: tenantUUID, CapabilityUUID: primaryUUID, CapabilityID: "cap.primary", DiscoveryGranted: true, ReadGranted: true, InvocationGranted: true, RuntimeContract: CapabilityRuntimeContract{AlternativeCapabilityUUIDs: []uuid.UUID{replacementUUID}}},
		{ResourceUUID: replacementUUID, Kind: ResourceKindCapability, DisplayName: "replacement", TenantUUID: tenantUUID, CapabilityUUID: replacementUUID, CapabilityID: "cap.replacement", DiscoveryGranted: true, ReadGranted: true, InvocationGranted: true},
	}, time.Now())
	require.NoError(t, err)
	plan := flowschema.ExecutionPlan{PlanID: "plan_0", Tasks: []flowschema.PlanTask{{TaskID: "invoke", NodeKind: "tooling", NodeRef: "cap.primary", Params: map[string]any{"capability_uuid": primaryUUID.String()}}}}
	controller, err := NewPlanController(snapshot, RuntimeBudget{MaxObservations: 1, MaxPlanRevisions: 1}, plan)
	require.NoError(t, err)
	observationSvc := NewObservationService(db)
	require.NoError(t, observationSvc.RegisterReader(ResourceKindCapability, CapabilityMetadataReader{}))
	ctx := ContextWithObservationService(ContextWithResourceSnapshot(context.Background(), snapshot), observationSvc)
	ctx = context.WithValue(ctx, "runtime_run_uuid", uuid.NewString())
	reason, next, observation, err := semanticReplacementPlanFromVerdict(ctx, controller, SemanticReplanner{}, snapshot, plan, VerificationVerdict{Class: VerificationReplaceable, TaskRefs: []string{"invoke"}, ReplacementTaskRef: "invoke", ReplacementCapabilityUUID: replacementUUID}, "test")
	require.NoError(t, err)
	require.Equal(t, "observation.semantic_continuation", reason)
	require.NotNil(t, observation.ReplanDirective)
	require.Equal(t, replacementUUID.String(), next.Tasks[0].Params["capability_uuid"])
	require.Len(t, controller.Observations(), 1)
}
