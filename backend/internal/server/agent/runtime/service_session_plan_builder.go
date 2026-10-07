package runtime

import (
	"context"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// InvokePlanningInput is loaded from the authorized admission anchor and its
// immutable message/history. It is never taken from the Stream payload.
type InvokePlanningInput struct {
	Context      context.Context
	Message      string
	Config       *dto.ChatConfig
	ExplicitFlow string
}

// InvokePlanningInputLoader reads the trusted business input for one Run.
type InvokePlanningInputLoader interface {
	Load(context.Context, agent_run.TaskRef) (InvokePlanningInput, error)
}

// ServiceSessionPlanBuilder turns one admitted service invocation into a
// persisted full plan and a scheduling DAG without executing any plan task.
type ServiceSessionPlanBuilder struct {
	Loader  InvokePlanningInputLoader
	Engine  *Engine
	Objects agent_run.ReportObjectStore
	Pool    TaskPoolResolver
	Budget  *agent_run.PlanBudget
}

var _ agent_run.PlanBuilder = (*ServiceSessionPlanBuilder)(nil)

func (b *ServiceSessionPlanBuilder) Build(ctx context.Context, ref agent_run.TaskRef, key string) (agent_run.PlanningResult, error) {
	if b == nil || b.Loader == nil || b.Engine == nil || b.Objects == nil || b.Pool == nil ||
		!validPlanArtifactRef(ref) || ref.Revision != 0 || ref.TaskID != agent_run.PlanningTaskID || ref.Attempt != 1 ||
		key != "agent:"+ref.RunID+":plan:1" {
		return agent_run.PlanningResult{}, agent_run.ErrInvalid
	}
	input, err := b.Loader.Load(ctx, ref)
	if err != nil {
		return agent_run.PlanningResult{ReasonCode: planningFailureReason(err, "planner.input_failed")}, err
	}
	if input.Context == nil || strings.TrimSpace(input.Message) == "" {
		return agent_run.PlanningResult{ReasonCode: "planner.input_invalid"}, agent_run.ErrInvalid
	}
	planCtx, cancel := context.WithCancel(input.Context)
	stop := context.AfterFunc(ctx, cancel)
	defer func() { stop(); cancel() }()
	plan, err := b.Engine.BuildInvokePlan(planCtx, input.Message, input.Config, input.ExplicitFlow)
	if err != nil {
		return agent_run.PlanningResult{ReasonCode: planningFailureReason(err, "planner.build_failed")}, err
	}
	schedule, err := TranslateInvokePlan(plan, b.Pool)
	if err != nil {
		return agent_run.PlanningResult{ReasonCode: planningFailureReason(err, "planner.plan_invalid")}, err
	}
	if b.Budget != nil {
		if err := applyDurablePlanBudget(&schedule, plan, *b.Budget); err != nil {
			return agent_run.PlanningResult{ReasonCode: "budget.exhausted"}, err
		}
	}
	resultRef, evidenceRef, err := SaveExecutionPlanArtifact(ctx, b.Objects, ref, plan)
	if err != nil {
		return agent_run.PlanningResult{ReasonCode: "store.unavailable"}, err
	}
	return agent_run.PlanningResult{Plan: schedule, ResultRef: resultRef, EvidenceRef: evidenceRef}, nil
}
