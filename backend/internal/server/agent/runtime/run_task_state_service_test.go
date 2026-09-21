package runtime

import (
	"context"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestBuildResumePlanSkipsCompletedDependencies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.Exec("CREATE TABLE public.agent_run_task_states (id integer primary key, created_at datetime, updated_at datetime, deleted_at datetime, uuid text, env text, tenant_uuid text, run_uuid text, snapshot_uuid text, plan_revision_uuid text, task_id text, status text, artifact_ref text)").Error)
	s := NewRunTaskStateService(db)
	run, rev := uuid.New(), uuid.New()
	tenant := uuid.NewString()
	require.NoError(t, s.repo.Upsert(context.Background(), &dbmodel.AgentRunTaskState{Env: "test", TenantUUID: tenant, RunUUID: run, SnapshotUUID: uuid.New(), PlanRevisionUUID: rev, TaskID: "prepare", Status: "completed"}))
	out, err := s.BuildResumePlan(context.Background(), "test", tenant, run, rev, flowschema.ExecutionPlan{PlanID: "p", Tasks: []flowschema.PlanTask{{TaskID: "prepare"}, {TaskID: "write", DependsOn: []string{"prepare"}}}})
	require.NoError(t, err)
	require.Len(t, out.Tasks, 1)
	require.Equal(t, "write", out.Tasks[0].TaskID)
	require.Empty(t, out.Tasks[0].DependsOn)
}
