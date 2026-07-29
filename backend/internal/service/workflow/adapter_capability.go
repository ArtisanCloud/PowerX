package workflow

import (
	"context"
	"errors"
)

var ErrCapabilityInvokerUnavailable = errors.New("workflow.capability_invoker_unavailable")

type CapabilityInvokeRequest struct {
	TenantUUID        string
	CapabilityID      string
	PreferredProtocol string
	NodeRef           string
	TraceID           string
	Config            map[string]any
	Input             map[string]any
}

type CapabilityInvokeResponse struct {
	Output map[string]any
}

type CapabilityInvoker interface {
	InvokeCapability(ctx context.Context, req CapabilityInvokeRequest) (CapabilityInvokeResponse, error)
}

type CapabilityAdapter struct {
	invoker CapabilityInvoker
}

func NewCapabilityAdapter(invoker CapabilityInvoker) *CapabilityAdapter {
	return &CapabilityAdapter{invoker: invoker}
}

func (a *CapabilityAdapter) Spec() NodeAdapterSpec {
	return NodeAdapterSpec{
		NodeKind:     "capability.invoke",
		DisplayName:  "workflow.node.capability.invoke",
		Category:     "capability",
		InputSchema:  requiredObjectSchema("capability_id", "preferred_protocol", "input_path", "output_path"),
		OutputSchema: objectSchema(),
	}
}

func (a *CapabilityAdapter) Validate(step StepDefinition) error {
	if err := requireConfigString(step, "capability_id"); err != nil {
		return err
	}
	if err := requireConfigString(step, "preferred_protocol"); err != nil {
		return err
	}
	if err := requireConfigString(step, "input_path"); err != nil {
		return err
	}
	return requireConfigString(step, "output_path")
}

func (a *CapabilityAdapter) Execute(ctx context.Context, exec NodeExecutionContext) (NodeResult, error) {
	if a == nil || a.invoker == nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: ErrCapabilityInvokerUnavailable.Error()}, ErrCapabilityInvokerUnavailable
	}
	resp, err := a.invoker.InvokeCapability(ctx, CapabilityInvokeRequest{
		TenantUUID:        exec.TenantUUID,
		CapabilityID:      configString(exec.Step.Config, "capability_id"),
		PreferredProtocol: configString(exec.Step.Config, "preferred_protocol"),
		NodeRef:           exec.Step.NodeRef,
		TraceID:           exec.TraceID,
		Config:            cloneMap(exec.Step.Config),
		Input:             cloneMap(exec.Payload),
	})
	if err != nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: "workflow.capability_invoke_failed", ErrorMessage: err.Error()}, err
	}
	return NodeResult{Status: NodeResultStatusSucceeded, Output: cloneMap(resp.Output)}, nil
}
