package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type planningInputLoaderFunc func(context.Context, agent_run.TaskRef) (InvokePlanningInput, error)

func (f planningInputLoaderFunc) Load(ctx context.Context, ref agent_run.TaskRef) (InvokePlanningInput, error) {
	return f(ctx, ref)
}

type planMemoryObjects struct {
	mu      sync.Mutex
	items   map[string][]byte
	failGet bool
}

func (m *planMemoryObjects) Put(_ context.Context, key string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = append([]byte(nil), body...)
	return nil
}
func (m *planMemoryObjects) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failGet {
		return nil, errors.New("object_unavailable")
	}
	return append([]byte(nil), m.items[key]...), nil
}

func TestServiceSessionPlanBuilderWritesVerifiableArtifact(t *testing.T) {
	ref := agent_run.TaskRef{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		Revision: 0, TaskID: agent_run.PlanningTaskID, Attempt: 1}
	objects := &planMemoryObjects{items: map[string][]byte{}}
	loads := 0
	builder := &ServiceSessionPlanBuilder{
		Loader: planningInputLoaderFunc(func(ctx context.Context, got agent_run.TaskRef) (InvokePlanningInput, error) {
			loads++
			require.Equal(t, ref, got)
			return InvokePlanningInput{Context: ctx, Message: "review", ExplicitFlow: "flow.review"}, nil
		}),
		Engine: &Engine{}, Objects: objects,
		Pool: func(task flowschema.PlanTask) (string, error) {
			require.Equal(t, "flow.review", task.FlowID)
			return "workflow", nil
		},
	}
	result, err := builder.Build(context.Background(), ref, "agent:"+ref.RunID+":plan:1")
	require.NoError(t, err)
	require.Equal(t, 1, loads)
	require.Len(t, result.Plan.Tasks, 1)
	require.Equal(t, "workflow", result.Plan.Tasks[0].PoolID)
	full, err := LoadExecutionPlanArtifact(context.Background(), objects, ref, result.ResultRef, result.EvidenceRef)
	require.NoError(t, err)
	require.Equal(t, "flow.review", full.Tasks[0].FlowID)
	objects.items[result.ResultRef][len(objects.items[result.ResultRef])-2] ^= 1
	_, err = LoadExecutionPlanArtifact(context.Background(), objects, ref, result.ResultRef, result.EvidenceRef)
	require.Error(t, err)
}

func TestServiceSessionPlanBuilderFailsClosedOnMissingArtifact(t *testing.T) {
	ref := agent_run.TaskRef{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		TaskID: agent_run.PlanningTaskID, Attempt: 1}
	objects := &planMemoryObjects{items: map[string][]byte{}, failGet: true}
	builder := &ServiceSessionPlanBuilder{
		Loader: planningInputLoaderFunc(func(ctx context.Context, _ agent_run.TaskRef) (InvokePlanningInput, error) {
			return InvokePlanningInput{Context: ctx, Message: "review", ExplicitFlow: "flow.review"}, nil
		}),
		Engine: &Engine{}, Objects: objects,
		Pool: func(flowschema.PlanTask) (string, error) { return "workflow", nil },
	}
	_, err := builder.Build(context.Background(), ref, "agent:"+ref.RunID+":plan:1")
	require.Error(t, err)
	_, err = builder.Build(context.Background(), ref, "wrong-key")
	require.ErrorIs(t, err, agent_run.ErrInvalid)
}
