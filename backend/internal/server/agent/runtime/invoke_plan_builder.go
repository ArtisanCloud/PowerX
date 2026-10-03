package runtime

import (
	"context"
	"fmt"
	"strings"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// BuildInvokePlan resolves the same intent/team inputs as RunPlanInvoke but
// stops before Dispatch or ExecutePlan. The returned plan must be persisted
// before a Worker can execute any task.
func (e *Engine) BuildInvokePlan(ctx context.Context, msg string, reqCfg *dto.ChatConfig, explicitFlow string) (*flowschema.ExecutionPlan, error) {
	if e == nil || strings.TrimSpace(msg) == "" {
		return nil, fmt.Errorf("agent planner requires a message")
	}
	if flowID := strings.TrimSpace(explicitFlow); flowID != "" {
		return &flowschema.ExecutionPlan{PlanID: "invoke_explicit", Tasks: []flowschema.PlanTask{{
			TaskID: "default", FlowID: flowID, NodeKind: "workflow", Stage: 1,
		}}}, nil
	}
	if e.mgr == nil {
		return nil, fmt.Errorf("agent planner manager is unavailable")
	}
	ctx = context.WithValue(ctx, "team_user_message", strings.TrimSpace(msg))
	teamPlan, handled, err := builtInTeamPlanFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if handled {
		if teamPlan == nil || len(teamPlan.Tasks) == 0 {
			return nil, fmt.Errorf("team planner produced no tasks")
		}
		return e.applyRuntimeParamState(ctx, teamPlan), nil
	}
	tasks, err := e.detectTasks(ctx, msg, reqCfg)
	if err != nil {
		return nil, err
	}
	raw := e.mgr.BuildPlan(tasks)
	plan, ok := NormalizeExecPlan(raw)
	if ok && plan != nil && len(plan.Tasks) > 0 {
		return e.applyRuntimeParamState(ctx, plan), nil
	}
	_, fallbackFlowID, err := e.mgr.GetDefaultRoute()
	if err != nil {
		return nil, fmt.Errorf("agent planner has no executable route: %w", err)
	}
	if strings.TrimSpace(fallbackFlowID) == "" {
		return nil, fmt.Errorf("agent planner has no executable route")
	}
	return &flowschema.ExecutionPlan{PlanID: "invoke_default", Tasks: []flowschema.PlanTask{{
		TaskID: "default", FlowID: fallbackFlowID, NodeKind: "workflow", Stage: 1,
	}}}, nil
}
