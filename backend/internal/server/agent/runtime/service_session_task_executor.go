package runtime

import (
	"context"
	"fmt"
	"reflect"
	"time"

	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

// TaskRunReader exposes only the RunStore facts needed to execute one task.
type TaskRunReader interface {
	Get(context.Context, string, string, string) (agent_run.Snapshot, error)
	GetPlan(context.Context, agent_run.Snapshot, uint64) (agent_run.Plan, error)
	GetTask(context.Context, agent_run.Snapshot, uint64, string) (agent_run.TaskSnapshot, error)
}

// DurableTaskInvoker runs only inside the Worker's persistent execution guard.
// Uncertain execution is quarantined instead of automatically replayed.
type DurableTaskInvoker interface {
	InvokeTask(context.Context, flowschema.ExecutionPlan, string,
		map[string]*aschema.ExecutionResult, InvokePlanningInput, string) (*aschema.ExecutionResult, error)
}

// ServiceSessionTaskExecutor reconstructs one task from the immutable plan,
// completed upstream receipts and the authorized admission input.
type ServiceSessionTaskExecutor struct {
	Runs    TaskRunReader
	Objects agent_run.ReportObjectStore
	Loader  InvokePlanningInputLoader
	Invoker DurableTaskInvoker
}

var _ agent_run.TaskExecutor = (*ServiceSessionTaskExecutor)(nil)

func (e *ServiceSessionTaskExecutor) Execute(ctx context.Context, ref agent_run.TaskRef, key string) (agent_run.WorkResult, error) {
	if e == nil || e.Runs == nil || e.Objects == nil || e.Loader == nil || e.Invoker == nil ||
		!validResultRef(ref) || key != fmt.Sprintf("agent:%s:%d:%s", ref.RunID, ref.Revision, ref.TaskID) {
		return agent_run.WorkResult{}, agent_run.ErrInvalid
	}
	run, err := e.Runs.Get(ctx, ref.TenantUUID, ref.Env, ref.RunID)
	if err != nil {
		return agent_run.WorkResult{}, err
	}
	if run.TenantUUID != ref.TenantUUID || run.Env != ref.Env || run.RunID != ref.RunID ||
		run.Status != "running" || run.PlanRevision != ref.Revision || !time.Now().Before(run.DeadlineAt) {
		return agent_run.WorkResult{}, agent_run.ErrConflict
	}
	current, err := e.Runs.GetTask(ctx, run, ref.Revision, ref.TaskID)
	if err != nil {
		return agent_run.WorkResult{}, err
	}
	if current.TaskID != ref.TaskID || current.Revision != ref.Revision ||
		current.Status != "running" || current.Attempt != ref.Attempt {
		return agent_run.WorkResult{}, agent_run.ErrConflict
	}
	planning, err := e.Runs.GetTask(ctx, run, 0, agent_run.PlanningTaskID)
	if err != nil {
		return agent_run.WorkResult{}, err
	}
	if planning.TaskID != agent_run.PlanningTaskID || planning.Revision != 0 || planning.Attempt != 1 ||
		planning.Status != "completed" || planning.ResultRef == "" || planning.EvidenceRef == "" {
		return agent_run.WorkResult{}, agent_run.ErrConflict
	}
	planRef := agent_run.TaskRef{TenantUUID: ref.TenantUUID, Env: ref.Env, RunID: ref.RunID,
		Revision: 0, TaskID: agent_run.PlanningTaskID, Attempt: 1}
	full, err := LoadExecutionPlanArtifact(ctx, e.Objects, planRef, planning.ResultRef, planning.EvidenceRef)
	if err != nil {
		return agent_run.WorkResult{}, err
	}
	schedule, err := e.Runs.GetPlan(ctx, run, ref.Revision)
	if err != nil {
		return agent_run.WorkResult{}, err
	}
	pools := make(map[string]string, len(schedule.Tasks))
	for _, task := range schedule.Tasks {
		pools[task.TaskID] = task.PoolID
	}
	rebuilt, err := TranslateInvokePlan(full, func(task flowschema.PlanTask) (string, error) {
		return pools[task.TaskID], nil
	})
	if err == nil && schedule.Budget != nil {
		err = applyDurablePlanBudget(&rebuilt, full, *schedule.Budget)
	}
	if err != nil || !reflect.DeepEqual(schedule, rebuilt) || schedule.Revision != ref.Revision {
		return agent_run.WorkResult{}, agent_run.ErrInvalid
	}
	var selected *flowschema.PlanTask
	for i := range full.Tasks {
		if full.Tasks[i].TaskID == ref.TaskID {
			selected = &full.Tasks[i]
			break
		}
	}
	if selected == nil {
		return agent_run.WorkResult{}, agent_run.ErrInvalid
	}
	prior := make(map[string]*aschema.ExecutionResult)
	for _, task := range full.Tasks {
		needed := task.Stage < selected.Stage
		for _, dependency := range selected.DependsOn {
			needed = needed || dependency == task.TaskID
		}
		if !needed {
			continue
		}
		state, err := e.Runs.GetTask(ctx, run, ref.Revision, task.TaskID)
		if err != nil {
			return agent_run.WorkResult{}, err
		}
		if state.TaskID != task.TaskID || state.Revision != ref.Revision || state.Status != "completed" ||
			state.ResultRef == "" || state.EvidenceRef == "" || state.Attempt == 0 {
			return agent_run.WorkResult{}, agent_run.ErrConflict
		}
		upstreamRef := agent_run.TaskRef{TenantUUID: ref.TenantUUID, Env: ref.Env, RunID: ref.RunID,
			Revision: ref.Revision, TaskID: task.TaskID, Attempt: state.Attempt}
		prior[task.TaskID], err = LoadTaskResultArtifact(ctx, e.Objects, upstreamRef, state.ResultRef, state.EvidenceRef)
		if err != nil {
			return agent_run.WorkResult{}, err
		}
	}
	input, err := e.Loader.Load(ctx, planRef)
	if err != nil {
		return agent_run.WorkResult{}, err
	}
	if input.Context == nil {
		return agent_run.WorkResult{}, agent_run.ErrInvalid
	}
	result, err := e.Invoker.InvokeTask(input.Context, *full, ref.TaskID, prior, input, key)
	if err != nil {
		return agent_run.WorkResult{ReasonCode: modelFailureReason(err)}, err
	}
	resultRef, evidenceRef, err := SaveTaskResultArtifact(ctx, e.Objects, ref, result)
	if err != nil {
		return agent_run.WorkResult{}, err
	}
	return agent_run.WorkResult{ResultRef: resultRef, EvidenceRef: evidenceRef}, nil
}
