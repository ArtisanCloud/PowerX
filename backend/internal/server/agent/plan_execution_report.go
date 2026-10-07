package agent

import (
	"sort"
	"sync"

	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

const (
	PlanTaskExecutionCompleted = "completed"
	PlanTaskExecutionFailed    = "failed"
)

// PlanTaskExecution records the terminal result of one submitted plan task.
// It is deliberately independent of the final response envelope: a final
// response cannot hide a continued task failure.
type PlanTaskExecution struct {
	TaskID        string                   `json:"task_id"`
	NodeKind      string                   `json:"node_kind"`
	NodeRef       string                   `json:"node_ref"`
	Stage         int                      `json:"stage"`
	FailurePolicy string                   `json:"failure_policy"`
	Status        string                   `json:"status"`
	Attempts      int                      `json:"attempts"`
	Result        *aschema.ExecutionResult `json:"result,omitempty"`
	Error         string                   `json:"error,omitempty"`
}

// PlanExecutionReport is the authoritative execution aggregate for a single
// immutable execution plan. FinalResult is retained only for response-envelope
// extraction; callers must use Tasks to determine the business outcome.
type PlanExecutionReport struct {
	PlanID      string              `json:"plan_id"`
	Tasks       []PlanTaskExecution `json:"tasks"`
	FinalResult *aschema.ExecutionResult

	mu sync.Mutex
}

func newPlanExecutionReport(planID string) *PlanExecutionReport {
	return &PlanExecutionReport{PlanID: planID}
}

func (r *PlanExecutionReport) record(task flowschema.PlanTask, status string, attempts int, out *aschema.ExecutionResult, err error) {
	if r == nil {
		return
	}
	record := PlanTaskExecution{
		TaskID:        task.TaskID,
		NodeKind:      planTaskKind(task),
		NodeRef:       planTaskRef(task),
		Stage:         task.Stage,
		FailurePolicy: planTaskFailurePolicy(task),
		Status:        status,
		Attempts:      attempts,
		Result:        out,
	}
	if err != nil {
		record.Error = err.Error()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Tasks = append(r.Tasks, record)
}

func (r *PlanExecutionReport) finalize(final *aschema.ExecutionResult) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.FinalResult = final
	sort.SliceStable(r.Tasks, func(i, j int) bool {
		if r.Tasks[i].Stage != r.Tasks[j].Stage {
			return r.Tasks[i].Stage < r.Tasks[j].Stage
		}
		return r.Tasks[i].TaskID < r.Tasks[j].TaskID
	})
}

func (r *PlanExecutionReport) HasFailures() bool {
	if r == nil {
		return false
	}
	for _, task := range r.Tasks {
		if task.Status == PlanTaskExecutionFailed {
			return true
		}
	}
	return false
}

func (r *PlanExecutionReport) HasCompletedTasks() bool {
	if r == nil {
		return false
	}
	for _, task := range r.Tasks {
		if task.Status == PlanTaskExecutionCompleted {
			return true
		}
	}
	return false
}
