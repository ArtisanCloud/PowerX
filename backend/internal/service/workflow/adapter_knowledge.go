package workflow

import (
	"context"
	"errors"
)

var ErrKnowledgeOperatorUnavailable = errors.New("workflow.knowledge_operator_unavailable")

type KnowledgeStageRequest struct {
	TenantUUID         string
	KnowledgeSpaceUUID string
	DraftSchemaRef     string
	Config             map[string]any
	Input              map[string]any
}

type KnowledgePublishRequest struct {
	TenantUUID         string
	KnowledgeSpaceUUID string
	PublishPolicy      string
	Config             map[string]any
	Input              map[string]any
}

type KnowledgeOperationResponse struct {
	Output map[string]any
}

type KnowledgeOperator interface {
	StageKnowledge(ctx context.Context, req KnowledgeStageRequest) (KnowledgeOperationResponse, error)
	PublishKnowledge(ctx context.Context, req KnowledgePublishRequest) (KnowledgeOperationResponse, error)
}

type KnowledgeStageAdapter struct {
	operator KnowledgeOperator
}

func NewKnowledgeStageAdapter(operator KnowledgeOperator) *KnowledgeStageAdapter {
	return &KnowledgeStageAdapter{operator: operator}
}

func (a *KnowledgeStageAdapter) Spec() NodeAdapterSpec {
	return NodeAdapterSpec{
		NodeKind:     "knowledge.stage",
		DisplayName:  "workflow.node.knowledge.stage",
		Category:     "knowledge",
		InputSchema:  requiredObjectSchema("knowledge_space_uuid", "draft_schema_ref", "input_path", "output_path"),
		OutputSchema: objectSchema(),
	}
}

func (a *KnowledgeStageAdapter) Validate(step StepDefinition) error {
	if err := requireConfigString(step, "knowledge_space_uuid"); err != nil {
		return err
	}
	if err := requireConfigString(step, "draft_schema_ref"); err != nil {
		return err
	}
	if err := requireConfigString(step, "input_path"); err != nil {
		return err
	}
	return requireConfigString(step, "output_path")
}

func (a *KnowledgeStageAdapter) Execute(ctx context.Context, exec NodeExecutionContext) (NodeResult, error) {
	if a == nil || a.operator == nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: ErrKnowledgeOperatorUnavailable.Error()}, ErrKnowledgeOperatorUnavailable
	}
	resp, err := a.operator.StageKnowledge(ctx, KnowledgeStageRequest{
		TenantUUID:         exec.TenantUUID,
		KnowledgeSpaceUUID: configString(exec.Step.Config, "knowledge_space_uuid"),
		DraftSchemaRef:     configString(exec.Step.Config, "draft_schema_ref"),
		Config:             cloneMap(exec.Step.Config),
		Input:              cloneMap(exec.Payload),
	})
	if err != nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: "workflow.knowledge_stage_failed", ErrorMessage: err.Error()}, err
	}
	return NodeResult{Status: NodeResultStatusSucceeded, Output: cloneMap(resp.Output)}, nil
}

type KnowledgePublishAdapter struct {
	operator KnowledgeOperator
}

func NewKnowledgePublishAdapter(operator KnowledgeOperator) *KnowledgePublishAdapter {
	return &KnowledgePublishAdapter{operator: operator}
}

func (a *KnowledgePublishAdapter) Spec() NodeAdapterSpec {
	return NodeAdapterSpec{
		NodeKind:     "knowledge.publish",
		DisplayName:  "workflow.node.knowledge.publish",
		Category:     "knowledge",
		InputSchema:  requiredObjectSchema("knowledge_space_uuid", "draft_refs_path", "review_result_path", "publish_policy"),
		OutputSchema: objectSchema(),
	}
}

func (a *KnowledgePublishAdapter) Validate(step StepDefinition) error {
	if err := requireConfigString(step, "knowledge_space_uuid"); err != nil {
		return err
	}
	if err := requireConfigString(step, "draft_refs_path"); err != nil {
		return err
	}
	if err := requireConfigString(step, "review_result_path"); err != nil {
		return err
	}
	return requireConfigString(step, "publish_policy")
}

func (a *KnowledgePublishAdapter) Execute(ctx context.Context, exec NodeExecutionContext) (NodeResult, error) {
	if a == nil || a.operator == nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: ErrKnowledgeOperatorUnavailable.Error()}, ErrKnowledgeOperatorUnavailable
	}
	resp, err := a.operator.PublishKnowledge(ctx, KnowledgePublishRequest{
		TenantUUID:         exec.TenantUUID,
		KnowledgeSpaceUUID: configString(exec.Step.Config, "knowledge_space_uuid"),
		PublishPolicy:      configString(exec.Step.Config, "publish_policy"),
		Config:             cloneMap(exec.Step.Config),
		Input:              cloneMap(exec.Payload),
	})
	if err != nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: "workflow.knowledge_publish_failed", ErrorMessage: err.Error()}, err
	}
	return NodeResult{Status: NodeResultStatusSucceeded, Output: cloneMap(resp.Output)}, nil
}
