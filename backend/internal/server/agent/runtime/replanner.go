package runtime

import (
	"context"
	"fmt"
	"strings"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/google/uuid"
)

// PlanRevisionRequest contains only frozen authorization state and redacted
// observations. Replanners must not fetch new resources or grants.
type PlanRevisionRequest struct {
	Snapshot     *ResourceSnapshot
	Observations []ResourceObservation
	CurrentPlan  flowschema.ExecutionPlan
}

type PlanRevisionPlanner interface {
	Replan(context.Context, PlanRevisionRequest) (string, flowschema.ExecutionPlan, error)
}

type planRevisionPlannerContextKey struct{}

func ContextWithPlanRevisionPlanner(ctx context.Context, planner PlanRevisionPlanner) context.Context {
	if ctx == nil || planner == nil {
		return ctx
	}
	return context.WithValue(ctx, planRevisionPlannerContextKey{}, planner)
}

func PlanRevisionPlannerFromContext(ctx context.Context) (PlanRevisionPlanner, bool) {
	if ctx == nil {
		return nil, false
	}
	planner, ok := ctx.Value(planRevisionPlannerContextKey{}).(PlanRevisionPlanner)
	return planner, ok && planner != nil
}

// ContinuationReplanner is the first bounded planner. It creates a new plan
// revision after observations but can only retain tasks that were already in
// the current plan; it cannot invent a new capability or expand authorization.
type ContinuationReplanner struct{}

func (ContinuationReplanner) Replan(_ context.Context, in PlanRevisionRequest) (string, flowschema.ExecutionPlan, error) {
	if in.Snapshot == nil || in.Snapshot.SnapshotUUID == uuid.Nil || len(in.Observations) == 0 || strings.TrimSpace(in.CurrentPlan.PlanID) == "" || len(in.CurrentPlan.Tasks) == 0 {
		return "", flowschema.ExecutionPlan{}, fmt.Errorf("plan revision request is incomplete")
	}
	for _, task := range in.CurrentPlan.Tasks {
		if strings.TrimSpace(task.TaskID) == "" {
			return "", flowschema.ExecutionPlan{}, fmt.Errorf("plan revision task_id is required")
		}
	}
	next := cloneExecutionPlan(in.CurrentPlan)
	next.PlanID = in.CurrentPlan.PlanID + "_continuation"
	return "observation.continuation", next, nil
}

// SemanticReplanner permits a reader to add one explicitly declared next
// capability. It is still bounded by the frozen snapshot and refuses all
// free-form directives, ungranted capabilities, duplicate task IDs, and
// dependencies outside the existing plan.
type SemanticReplanner struct{}

func (SemanticReplanner) Replan(ctx context.Context, in PlanRevisionRequest) (string, flowschema.ExecutionPlan, error) {
	reason, next, err := (ContinuationReplanner{}).Replan(ctx, in)
	if err != nil {
		return "", flowschema.ExecutionPlan{}, err
	}
	var directive *ReplanDirective
	for _, observation := range in.Observations {
		if observation.ReplanDirective == nil {
			continue
		}
		if directive != nil {
			return "", flowschema.ExecutionPlan{}, fmt.Errorf("multiple replan directives are not supported")
		}
		directive = observation.ReplanDirective
	}
	if directive == nil {
		return reason, next, nil
	}
	if directive.CapabilityUUID == uuid.Nil || strings.TrimSpace(directive.TaskID) == "" {
		return "", flowschema.ExecutionPlan{}, fmt.Errorf("replan directive is invalid")
	}
	resource, err := in.Snapshot.RequireInvocable(directive.CapabilityUUID)
	if err != nil || resource.Kind != ResourceKindCapability || resource.CapabilityUUID != directive.CapabilityUUID || strings.TrimSpace(resource.CapabilityID) == "" {
		return "", flowschema.ExecutionPlan{}, fmt.Errorf("replan directive capability is not invocable in snapshot")
	}
	existing := make(map[string]struct{}, len(next.Tasks))
	for _, task := range next.Tasks {
		existing[task.TaskID] = struct{}{}
	}
	switch directive.Action {
	case "append_authorized_capability":
		if _, found := existing[directive.TaskID]; found {
			return "", flowschema.ExecutionPlan{}, fmt.Errorf("replan directive task_id already exists")
		}
		for _, dependency := range directive.DependsOn {
			if _, found := existing[dependency]; !found {
				return "", flowschema.ExecutionPlan{}, fmt.Errorf("replan directive dependency is outside plan")
			}
		}
		params := clonePlanParams(directive.Params)
		if params == nil {
			params = make(map[string]interface{}, 1)
		}
		params["capability_uuid"] = directive.CapabilityUUID.String()
		next.Tasks = append(next.Tasks, flowschema.PlanTask{TaskID: directive.TaskID, NodeKind: "tooling", NodeRef: resource.CapabilityID, SourceScope: "agent", Params: params, DependsOn: append([]string(nil), directive.DependsOn...)})
	case "replace_authorized_capability":
		if len(directive.DependsOn) != 0 || len(directive.Params) != 0 {
			return "", flowschema.ExecutionPlan{}, fmt.Errorf("replacement replan directive cannot alter task dependencies or parameters")
		}
		replaced := false
		for index := range next.Tasks {
			task := &next.Tasks[index]
			if task.TaskID != directive.TaskID {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(task.NodeKind), "tooling") || task.Params == nil {
				return "", flowschema.ExecutionPlan{}, fmt.Errorf("replacement replan task must be tooling with parameters")
			}
			task.NodeRef = resource.CapabilityID
			task.Params["capability_uuid"] = directive.CapabilityUUID.String()
			task.Params["capability_id"] = resource.CapabilityID
			replaced = true
			break
		}
		if !replaced {
			return "", flowschema.ExecutionPlan{}, fmt.Errorf("replacement replan task is outside current plan")
		}
	default:
		return "", flowschema.ExecutionPlan{}, fmt.Errorf("replan directive action is not supported")
	}
	next.PlanID = in.CurrentPlan.PlanID + "_semantic_continuation"
	return "observation.semantic_continuation", next, nil
}

func cloneExecutionPlan(in flowschema.ExecutionPlan) flowschema.ExecutionPlan {
	out := flowschema.ExecutionPlan{PlanID: in.PlanID, Tasks: make([]flowschema.PlanTask, 0, len(in.Tasks))}
	for _, task := range in.Tasks {
		copyTask := task
		copyTask.DependsOn = append([]string(nil), task.DependsOn...)
		copyTask.Params = clonePlanParams(task.Params)
		copyTask.ParamRefs = make(map[string]string, len(task.ParamRefs))
		for key, value := range task.ParamRefs {
			copyTask.ParamRefs[key] = value
		}
		out.Tasks = append(out.Tasks, copyTask)
	}
	return out
}
