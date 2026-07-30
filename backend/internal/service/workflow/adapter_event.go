package workflow

import (
	"context"
	"errors"
)

var ErrWorkflowEventPublisherUnavailable = errors.New("workflow.event_publisher_unavailable")

type WorkflowEventPublishRequest struct {
	TenantUUID string
	Topic      string
	Config     map[string]any
	Payload    map[string]any
}

type WorkflowEventPublisher interface {
	PublishWorkflowEvent(ctx context.Context, req WorkflowEventPublishRequest) error
}

type EventAdapter struct {
	publisher WorkflowEventPublisher
}

func NewEventAdapter(publisher WorkflowEventPublisher) *EventAdapter {
	return &EventAdapter{publisher: publisher}
}

func (a *EventAdapter) Spec() NodeAdapterSpec {
	return NodeAdapterSpec{
		NodeKind:     "event.emit",
		DisplayName:  "workflow.node.event.emit",
		Category:     "event",
		InputSchema:  requiredObjectSchema("topic", "payload_path", "event_schema_ref"),
		OutputSchema: objectSchema(),
	}
}

func (a *EventAdapter) Validate(step StepDefinition) error {
	if err := requireConfigString(step, "topic"); err != nil {
		return err
	}
	if err := requireConfigString(step, "payload_path"); err != nil {
		return err
	}
	return requireConfigString(step, "event_schema_ref")
}

func (a *EventAdapter) Execute(ctx context.Context, exec NodeExecutionContext) (NodeResult, error) {
	if a == nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: ErrWorkflowEventPublisherUnavailable.Error()}, ErrWorkflowEventPublisherUnavailable
	}
	if workflowInputBool(exec, "request", "payload", "dry_run") {
		return NodeResult{Status: NodeResultStatusSucceeded, Output: map[string]any{
			"topic":     configString(exec.Step.Config, "topic"),
			"dry_run":   true,
			"simulated": true,
		}}, nil
	}
	if a.publisher == nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: ErrWorkflowEventPublisherUnavailable.Error()}, ErrWorkflowEventPublisherUnavailable
	}
	if err := a.publisher.PublishWorkflowEvent(ctx, WorkflowEventPublishRequest{
		TenantUUID: exec.TenantUUID,
		Topic:      configString(exec.Step.Config, "topic"),
		Config:     cloneMap(exec.Step.Config),
		Payload:    cloneMap(exec.Payload),
	}); err != nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: "workflow.event_publish_failed", ErrorMessage: err.Error()}, err
	}
	return NodeResult{Status: NodeResultStatusSucceeded, Output: map[string]any{"published": true}}, nil
}
