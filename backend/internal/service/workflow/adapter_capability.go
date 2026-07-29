package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	if a == nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: ErrCapabilityInvokerUnavailable.Error()}, ErrCapabilityInvokerUnavailable
	}
	capabilityID, err := resolveRuntimeConfigString(exec, "capability_id")
	if err != nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: "workflow.capability_config_invalid", ErrorMessage: err.Error()}, err
	}
	preferredProtocol, err := resolveRuntimeConfigString(exec, "preferred_protocol")
	if err != nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: "workflow.capability_config_invalid", ErrorMessage: err.Error()}, err
	}
	if workflowInputBool(exec, "request", "payload", "dry_run") {
		return NodeResult{
			Status: NodeResultStatusSucceeded,
			Output: map[string]any{
				"capability_id":      capabilityID,
				"preferred_protocol": preferredProtocol,
				"dry_run":            true,
				"simulated":          true,
			},
		}, nil
	}
	if a.invoker == nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: ErrCapabilityInvokerUnavailable.Error()}, ErrCapabilityInvokerUnavailable
	}
	resp, err := a.invoker.InvokeCapability(ctx, CapabilityInvokeRequest{
		TenantUUID:        exec.TenantUUID,
		CapabilityID:      capabilityID,
		PreferredProtocol: preferredProtocol,
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

func resolveRuntimeConfigString(exec NodeExecutionContext, key string) (string, error) {
	value := configString(exec.Step.Config, key)
	if value == "" {
		return "", fmt.Errorf("workflow.node_config_required: %s.%s", exec.Step.ID, key)
	}
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return value, nil
	}
	inputKey := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}"))
	if inputKey == "" {
		return "", fmt.Errorf("workflow.node_config_placeholder_invalid: %s.%s", exec.Step.ID, key)
	}
	resolved := workflowInputString(exec, inputKey)
	if resolved == "" {
		return "", fmt.Errorf("workflow.node_config_placeholder_missing: %s.%s=%s", exec.Step.ID, key, value)
	}
	return resolved, nil
}

func workflowInputString(exec NodeExecutionContext, path ...string) string {
	value := workflowInputValue(exec, path...)
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func workflowInputBool(exec NodeExecutionContext, path ...string) bool {
	value := workflowInputValue(exec, path...)
	typed, ok := value.(bool)
	return ok && typed
}

func workflowInputValue(exec NodeExecutionContext, path ...string) any {
	if exec.Instance == nil || len(exec.Instance.InputContext) == 0 || len(path) == 0 {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal(exec.Instance.InputContext, &data); err != nil {
		return nil
	}
	var current any = data
	for _, key := range path {
		node, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = node[strings.TrimSpace(key)]
	}
	return current
}
