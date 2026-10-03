package runtime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

// TaskPoolResolver maps an executable node to a configured physical or
// non-model capacity pool. An unresolved pool fails planning closed.
type TaskPoolResolver func(flowschema.PlanTask) (string, error)

// TranslateInvokePlan preserves explicit task dependencies and the legacy
// stage barrier. Tasks in one stage remain independent unless a dependency
// is explicitly declared; each later stage waits for every prior-stage task.
// The full ExecutionPlan must be saved separately for task execution.
func TranslateInvokePlan(source *flowschema.ExecutionPlan, pool TaskPoolResolver) (agent_run.Plan, error) {
	if source == nil || len(source.Tasks) == 0 || pool == nil {
		return agent_run.Plan{}, agent_run.ErrInvalid
	}
	stageIDs := make(map[int][]string)
	stages := make([]int, 0)
	seen := make(map[string]bool, len(source.Tasks))
	for _, task := range source.Tasks {
		id := strings.TrimSpace(task.TaskID)
		if id == "" || id != task.TaskID || task.Stage < 1 || seen[id] {
			return agent_run.Plan{}, agent_run.ErrInvalid
		}
		seen[id] = true
		if _, exists := stageIDs[task.Stage]; !exists {
			stages = append(stages, task.Stage)
		}
		stageIDs[task.Stage] = append(stageIDs[task.Stage], id)
	}
	sort.Ints(stages)
	previous := make(map[int][]string, len(stages))
	for i, stage := range stages {
		if i > 0 {
			previous[stage] = stageIDs[stages[i-1]]
		}
	}
	plan := agent_run.Plan{Revision: 1, Tasks: make([]agent_run.TaskDefinition, 0, len(source.Tasks))}
	for _, task := range source.Tasks {
		poolID, err := pool(task)
		if err != nil || strings.TrimSpace(poolID) == "" || poolID != strings.TrimSpace(poolID) {
			return agent_run.Plan{}, fmt.Errorf("%w: task %s has no capacity pool", agent_run.ErrInvalid, task.TaskID)
		}
		deps := make([]string, 0, len(task.DependsOn)+len(previous[task.Stage]))
		added := make(map[string]bool)
		for _, dep := range append(append([]string(nil), task.DependsOn...), previous[task.Stage]...) {
			if dep == task.TaskID || !seen[dep] || strings.TrimSpace(dep) != dep {
				return agent_run.Plan{}, agent_run.ErrInvalid
			}
			if !added[dep] {
				deps = append(deps, dep)
				added[dep] = true
			}
		}
		plan.Tasks = append(plan.Tasks, agent_run.TaskDefinition{TaskID: task.TaskID, DependsOn: deps, PoolID: poolID})
	}
	if !agent_run.ValidPlan(plan) {
		return agent_run.Plan{}, agent_run.ErrInvalid
	}
	return plan, nil
}
