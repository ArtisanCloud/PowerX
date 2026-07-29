package workflow

import (
	"context"
	"errors"
)

type InputCaptureAdapter struct{}

func NewInputCaptureAdapter() *InputCaptureAdapter {
	return &InputCaptureAdapter{}
}

func (a *InputCaptureAdapter) Spec() NodeAdapterSpec {
	return NodeAdapterSpec{
		NodeKind:     "input.capture",
		DisplayName:  "workflow.node.input.capture",
		Category:     "input",
		InputSchema:  requiredObjectSchema("input_schema_ref", "source_policy", "artifact_output_path"),
		OutputSchema: objectSchema(),
	}
}

func (a *InputCaptureAdapter) Validate(step StepDefinition) error {
	if err := requireConfigString(step, "input_schema_ref"); err != nil {
		return err
	}
	if err := requireConfigValue(step, "source_policy"); err != nil {
		return err
	}
	return requireConfigString(step, "artifact_output_path")
}

func (a *InputCaptureAdapter) Execute(ctx context.Context, exec NodeExecutionContext) (NodeResult, error) {
	return NodeResult{Status: NodeResultStatusSucceeded, Output: cloneMap(exec.Payload)}, nil
}

type DecisionGatewayAdapter struct{}

func NewDecisionGatewayAdapter() *DecisionGatewayAdapter {
	return &DecisionGatewayAdapter{}
}

func (a *DecisionGatewayAdapter) Spec() NodeAdapterSpec {
	return NodeAdapterSpec{
		NodeKind:     "decision.gateway",
		DisplayName:  "workflow.node.decision.gateway",
		Category:     "decision",
		InputSchema:  requiredObjectSchema("routes", "default_route", "condition_source_path"),
		OutputSchema: objectSchema(),
	}
}

func (a *DecisionGatewayAdapter) Validate(step StepDefinition) error {
	if err := requireConfigValue(step, "routes"); err != nil {
		return err
	}
	if err := requireConfigString(step, "default_route"); err != nil {
		return err
	}
	return requireConfigString(step, "condition_source_path")
}

func (a *DecisionGatewayAdapter) Execute(ctx context.Context, exec NodeExecutionContext) (NodeResult, error) {
	decision := configString(exec.Step.Config, "default_route")
	if value, ok := exec.Payload["decision"].(string); ok && value != "" {
		decision = value
	}
	if decision == "" {
		err := errors.New("workflow.decision_required")
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: err.Error()}, err
	}
	return NodeResult{Status: NodeResultStatusSucceeded, Decision: decision, Output: map[string]any{"decision": decision}}, nil
}

type ParallelAdapter struct {
	nodeKind string
}

func NewParallelFanoutAdapter() *ParallelAdapter {
	return &ParallelAdapter{nodeKind: "parallel.fanout"}
}

func NewParallelJoinAdapter() *ParallelAdapter {
	return &ParallelAdapter{nodeKind: "parallel.join"}
}

func (a *ParallelAdapter) Spec() NodeAdapterSpec {
	return NodeAdapterSpec{
		NodeKind:     a.nodeKind,
		DisplayName:  "workflow.node." + a.nodeKind,
		Category:     "parallel",
		InputSchema:  objectSchema(),
		OutputSchema: objectSchema(),
	}
}

func (a *ParallelAdapter) Validate(step StepDefinition) error {
	return nil
}

func (a *ParallelAdapter) Execute(ctx context.Context, exec NodeExecutionContext) (NodeResult, error) {
	return NodeResult{Status: NodeResultStatusSucceeded, Output: cloneMap(exec.Payload)}, nil
}

type CompensationRollbackAdapter struct{}

func NewCompensationRollbackAdapter() *CompensationRollbackAdapter {
	return &CompensationRollbackAdapter{}
}

func (a *CompensationRollbackAdapter) Spec() NodeAdapterSpec {
	return NodeAdapterSpec{
		NodeKind:     "compensation.rollback",
		DisplayName:  "workflow.node.compensation.rollback",
		Category:     "compensation",
		InputSchema:  requiredObjectSchema("target_step_id", "rollback_policy", "manual_approval_required"),
		OutputSchema: objectSchema(),
	}
}

func (a *CompensationRollbackAdapter) Validate(step StepDefinition) error {
	if err := requireConfigString(step, "target_step_id"); err != nil {
		return err
	}
	if err := requireConfigValue(step, "rollback_policy"); err != nil {
		return err
	}
	return requireConfigValue(step, "manual_approval_required")
}

func (a *CompensationRollbackAdapter) Execute(ctx context.Context, exec NodeExecutionContext) (NodeResult, error) {
	return NodeResult{Status: NodeResultStatusCompensating, Output: cloneMap(exec.Payload)}, nil
}
