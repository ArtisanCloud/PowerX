package runtime

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestObserveSnapshotResourceRequiresBudgetAndService(t *testing.T) {
	_, err := ObserveSnapshotResource(context.Background(), nil, "test", uuid.New(), "plan")
	require.Error(t, err)
	budget := &RuntimeBudget{MaxObservations: 1}
	_, err = ObserveSnapshotResource(context.Background(), budget, "test", uuid.New(), "plan")
	require.Error(t, err)
	require.Equal(t, 1, budget.Observations)
}

func TestRestoreObservationsRestoresTypedReplanDirective(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.Exec(`CREATE TABLE public.agent_run_observations (
		id integer primary key, uuid text, created_at datetime, updated_at datetime, deleted_at datetime,
		env text, tenant_uuid text, run_uuid text, snapshot_uuid text, resource_uuid text,
		purpose text, summary text, artifact_ref text, directive json, digest text
	)`).Error)
	runUUID, resourceUUID, capabilityUUID := uuid.New(), uuid.New(), uuid.New()
	directive, err := json.Marshal(ReplanDirective{Action: "append_authorized_capability", CapabilityUUID: capabilityUUID, TaskID: "follow_up", DependsOn: []string{"observe"}})
	require.NoError(t, err)
	require.NoError(t, db.Exec("INSERT INTO public.agent_run_observations (uuid, env, tenant_uuid, run_uuid, snapshot_uuid, resource_uuid, purpose, directive, digest) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", uuid.NewString(), "test", "tenant", runUUID.String(), uuid.NewString(), resourceUUID.String(), "plan", string(directive), "digest").Error)

	observations, err := NewObservationService(db).RestoreObservations(context.Background(), "test", "tenant", runUUID)
	require.NoError(t, err)
	require.Len(t, observations, 1)
	require.NotNil(t, observations[0].ReplanDirective)
	require.Equal(t, capabilityUUID, observations[0].ReplanDirective.CapabilityUUID)
	require.Equal(t, []string{"observe"}, observations[0].ReplanDirective.DependsOn)
}
