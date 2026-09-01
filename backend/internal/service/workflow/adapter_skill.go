package workflow

import (
	"context"
	"errors"
)

var ErrSkillInvokerUnavailable = errors.New("workflow.skill_invoker_unavailable")

type SkillInvokeRequest struct {
	TenantUUID string
	SkillID    string
	NodeRef    string
	AgentUUID  string
	TraceID    string
	Config     map[string]any
	Input      map[string]any
}

type SkillInvokeResponse struct {
	Output map[string]any
}

type SkillInvoker interface {
	InvokeSkill(ctx context.Context, req SkillInvokeRequest) (SkillInvokeResponse, error)
}

type SkillAdapter struct {
	invoker SkillInvoker
}

func NewSkillAdapter(invoker SkillInvoker) *SkillAdapter {
	return &SkillAdapter{invoker: invoker}
}

func (a *SkillAdapter) Spec() NodeAdapterSpec {
	return NodeAdapterSpec{
		NodeKind:     "skill.invoke",
		DisplayName:  "workflow.node.skill.invoke",
		Category:     "skill",
		InputSchema:  requiredObjectSchema("skill_id", "entrypoint", "input_path", "output_path"),
		OutputSchema: objectSchema(),
	}
}

func (a *SkillAdapter) Validate(step StepDefinition) error {
	if err := requireConfigString(step, "skill_id"); err != nil {
		return err
	}
	if err := requireConfigString(step, "entrypoint"); err != nil {
		return err
	}
	if err := requireConfigString(step, "input_path"); err != nil {
		return err
	}
	return requireConfigString(step, "output_path")
}

func (a *SkillAdapter) Execute(ctx context.Context, exec NodeExecutionContext) (NodeResult, error) {
	if a == nil || a.invoker == nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: ErrSkillInvokerUnavailable.Error()}, ErrSkillInvokerUnavailable
	}
	resp, err := a.invoker.InvokeSkill(ctx, SkillInvokeRequest{
		TenantUUID: exec.TenantUUID,
		SkillID:    configString(exec.Step.Config, "skill_id"),
		NodeRef:    exec.Step.NodeRef,
		AgentUUID:  exec.AgentUUID.String(),
		TraceID:    exec.TraceID,
		Config:     cloneMap(exec.Step.Config),
		Input:      cloneMap(exec.Payload),
	})
	if err != nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: "workflow.skill_invoke_failed", ErrorMessage: err.Error()}, err
	}
	return NodeResult{Status: NodeResultStatusSucceeded, Output: cloneMap(resp.Output)}, nil
}
