package runtime

import (
	"context"
	"testing"
	"time"

	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fixedTaskReader struct {
	run   agent_run.Snapshot
	plan  agent_run.Plan
	tasks map[string]agent_run.TaskSnapshot
}

func (r *fixedTaskReader) Get(context.Context, string, string, string) (agent_run.Snapshot, error) {
	return r.run, nil
}
func (r *fixedTaskReader) GetPlan(context.Context, agent_run.Snapshot, uint64) (agent_run.Plan, error) {
	return r.plan, nil
}
func (r *fixedTaskReader) GetTask(_ context.Context, _ agent_run.Snapshot, revision uint64, id string) (agent_run.TaskSnapshot, error) {
	if revision == 0 {
		return r.tasks["plan"], nil
	}
	return r.tasks[id], nil
}

type taskInvokerFunc func(context.Context, flowschema.ExecutionPlan, string,
	map[string]*aschema.ExecutionResult, InvokePlanningInput, string) (*aschema.ExecutionResult, error)

func (f taskInvokerFunc) InvokeTask(ctx context.Context, plan flowschema.ExecutionPlan, id string,
	prior map[string]*aschema.ExecutionResult, input InvokePlanningInput, key string) (*aschema.ExecutionResult, error) {
	return f(ctx, plan, id, prior, input, key)
}

func TestServiceSessionTaskExecutorReplaysVerifiedPriorResults(t *testing.T) {
	ref := agent_run.TaskRef{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		Revision: 1, TaskID: "final", Attempt: 1}
	planRef := ref
	planRef.Revision, planRef.TaskID = 0, agent_run.PlanningTaskID
	full := &flowschema.ExecutionPlan{PlanID: "persisted", Tasks: []flowschema.PlanTask{
		{TaskID: "source", NodeKind: "skill", NodeRef: "skill.source", Stage: 1},
		{TaskID: "final", NodeKind: "skill", NodeRef: "skill.final", Stage: 2, DependsOn: []string{"source"}},
	}}
	objects := &planMemoryObjects{items: map[string][]byte{}}
	planKey, planEvidence, err := SaveExecutionPlanArtifact(context.Background(), objects, planRef, full)
	require.NoError(t, err)
	upstreamRef := ref
	upstreamRef.TaskID = "source"
	upstreamKey, upstreamEvidence, err := SaveTaskResultArtifact(context.Background(), objects, upstreamRef,
		&aschema.ExecutionResult{Success: true, Data: flowschema.Result{"result": map[string]any{"text": "source"}}})
	require.NoError(t, err)
	schedule, err := TranslateInvokePlan(full, func(flowschema.PlanTask) (string, error) { return "skill", nil })
	require.NoError(t, err)
	reader := &fixedTaskReader{run: agent_run.Snapshot{TenantUUID: ref.TenantUUID, Env: ref.Env, RunID: ref.RunID,
		Status: "running", PlanRevision: 1, DeadlineAt: time.Now().Add(time.Hour)}, plan: schedule,
		tasks: map[string]agent_run.TaskSnapshot{
			"plan":   {TaskID: "plan", Revision: 0, Status: "completed", Attempt: 1, ResultRef: planKey, EvidenceRef: planEvidence},
			"source": {TaskID: "source", Revision: 1, Status: "completed", Attempt: 1, ResultRef: upstreamKey, EvidenceRef: upstreamEvidence},
			"final":  {TaskID: "final", Revision: 1, Status: "running", Attempt: 1},
		}}
	calls := 0
	executor := &ServiceSessionTaskExecutor{Runs: reader, Objects: objects,
		Loader: planningInputLoaderFunc(func(ctx context.Context, got agent_run.TaskRef) (InvokePlanningInput, error) {
			require.Equal(t, planRef, got)
			return InvokePlanningInput{Context: ctx, Message: "review"}, nil
		}),
		Invoker: taskInvokerFunc(func(_ context.Context, got flowschema.ExecutionPlan, id string,
			prior map[string]*aschema.ExecutionResult, _ InvokePlanningInput, key string) (*aschema.ExecutionResult, error) {
			calls++
			require.Equal(t, *full, got)
			require.Equal(t, "final", id)
			require.Equal(t, "source", prior["source"].Data["result"].(map[string]any)["text"])
			require.Equal(t, "agent:"+ref.RunID+":1:final", key)
			return &aschema.ExecutionResult{Success: true, Data: flowschema.Result{"content": "done"}}, nil
		}),
	}
	result, err := executor.Execute(context.Background(), ref, "agent:"+ref.RunID+":1:final")
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	read, err := LoadTaskResultArtifact(context.Background(), objects, ref, result.ResultRef, result.EvidenceRef)
	require.NoError(t, err)
	require.Equal(t, "done", read.Data["content"])

	require.NoError(t, applyDurablePlanBudget(&reader.plan, full, agent_run.PlanBudget{MaxSteps: 2, MaxCapabilityCalls: 1}))
	_, err = executor.Execute(context.Background(), ref, "agent:"+ref.RunID+":1:final")
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	reader.plan.Budget.MaxSteps = 1
	_, err = executor.Execute(context.Background(), ref, "agent:"+ref.RunID+":1:final")
	require.Error(t, err)
	require.Equal(t, 2, calls)
	reader.plan.Budget.MaxSteps = 2

	objects.items[upstreamKey][len(objects.items[upstreamKey])-2] ^= 1
	_, err = executor.Execute(context.Background(), ref, "agent:"+ref.RunID+":1:final")
	require.Error(t, err)
	require.Equal(t, 2, calls)
}
