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

func TestValidateSemanticReplayFromPersistedRevision(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.Exec(`CREATE TABLE public.agent_plan_revisions (
 id integer primary key, uuid text, created_at datetime, updated_at datetime, deleted_at datetime,
 env text, tenant_uuid text, run_uuid text, snapshot_uuid text, parent_revision_uuid text, reason_code text,
 plan json, trigger_observation_uuids json, superseded_task_refs json, new_task_refs json,
 plan_revisions_consumed integer, observations_consumed integer
)`).Error)
	tenant, run, capability := uuid.NewString(), uuid.New(), uuid.New()
	snapshot, err := NewResourceSnapshot(tenant, uuid.New(), []ResourceDescriptor{{ResourceUUID: capability, Kind: ResourceKindCapability, DisplayName: "cap", TenantUUID: tenant, CapabilityUUID: capability, CapabilityID: "cap.follow", DiscoveryGranted: true, InvocationGranted: true}}, time.Now())
	require.NoError(t, err)
	svc := NewPlanRevisionService(db)
	parent := &PlanRevision{RevisionUUID: uuid.New(), SnapshotUUID: snapshot.SnapshotUUID, ReasonCode: "initial_plan", Plan: flowschema.ExecutionPlan{PlanID: "p0", Tasks: []flowschema.PlanTask{{TaskID: "observe"}}}}
	require.NoError(t, svc.Persist(context.Background(), "test", tenant, run, parent, RuntimeBudget{}, nil))
	observation := ResourceObservation{ObservationUUID: uuid.New(), ResourceUUID: capability, ReplanDirective: &ReplanDirective{Action: "append_authorized_capability", CapabilityUUID: capability, TaskID: "follow", DependsOn: []string{"observe"}}}
	_, next, err := (SemanticReplanner{}).Replan(context.Background(), PlanRevisionRequest{Snapshot: snapshot, CurrentPlan: parent.Plan, Observations: []ResourceObservation{observation}})
	require.NoError(t, err)
	child := &PlanRevision{RevisionUUID: uuid.New(), ParentRevisionUUID: &parent.RevisionUUID, SnapshotUUID: snapshot.SnapshotUUID, ReasonCode: "observation.semantic_continuation", Plan: next}
	require.NoError(t, svc.Persist(context.Background(), "test", tenant, run, child, RuntimeBudget{}, []ResourceObservation{observation}))
	restored, err := svc.Restore(context.Background(), "test", tenant, run, snapshot.SnapshotUUID, child.RevisionUUID)
	require.NoError(t, err)
	require.NoError(t, svc.ValidateSemanticReplay(context.Background(), "test", tenant, run, snapshot, restored, []ResourceObservation{observation}))
}
