package workflow

import (
	"context"
	"errors"
	"testing"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
)

type testSkillInvoker struct {
	req SkillInvokeRequest
}

func (i *testSkillInvoker) InvokeSkill(_ context.Context, req SkillInvokeRequest) (SkillInvokeResponse, error) {
	i.req = req
	return SkillInvokeResponse{Output: map[string]any{"skill": req.SkillID}}, nil
}

type testCapabilityInvoker struct {
	calls int
	req   CapabilityInvokeRequest
}

func (i *testCapabilityInvoker) InvokeCapability(_ context.Context, req CapabilityInvokeRequest) (CapabilityInvokeResponse, error) {
	i.calls++
	i.req = req
	return CapabilityInvokeResponse{Output: map[string]any{"capability": true}}, nil
}

type testMetadataClassifier struct{}

func (c testMetadataClassifier) ClassifyMetadata(context.Context, MetadataClassifyRequest) (MetadataClassifyResponse, error) {
	return MetadataClassifyResponse{Output: map[string]any{"metadata": true}}, nil
}

type testKnowledgeOperator struct{}

func (o testKnowledgeOperator) StageKnowledge(context.Context, KnowledgeStageRequest) (KnowledgeOperationResponse, error) {
	return KnowledgeOperationResponse{Output: map[string]any{"staged": true}}, nil
}

func (o testKnowledgeOperator) PublishKnowledge(context.Context, KnowledgePublishRequest) (KnowledgeOperationResponse, error) {
	return KnowledgeOperationResponse{Output: map[string]any{"published": true}}, nil
}

type testWorkflowEventPublisher struct {
	topic string
}

func (p *testWorkflowEventPublisher) PublishWorkflowEvent(_ context.Context, req WorkflowEventPublishRequest) error {
	p.topic = req.Topic
	return nil
}

func TestSkillAdapterValidatesAndInvokes(t *testing.T) {
	adapter := NewSkillAdapter(nil)
	if err := adapter.Validate(StepDefinition{ID: "extract", Config: map[string]any{"skill_id": "s1"}}); err == nil {
		t.Fatal("expected missing config validation error")
	}
	if _, err := adapter.Execute(context.Background(), NodeExecutionContext{}); !errors.Is(err, ErrSkillInvokerUnavailable) {
		t.Fatalf("expected ErrSkillInvokerUnavailable, got %v", err)
	}

	invoker := &testSkillInvoker{}
	adapter = NewSkillAdapter(invoker)
	result, err := adapter.Execute(context.Background(), NodeExecutionContext{
		TenantUUID: "tenant-a",
		Step: StepDefinition{NodeRef: "skill.ref", Config: map[string]any{
			"skill_id":    "knowledge.extract",
			"input_path":  "$.in",
			"output_path": "$.out",
		}},
		Payload: map[string]any{"source": "demo"},
	})
	if err != nil {
		t.Fatalf("execute skill: %v", err)
	}
	if result.Status != NodeResultStatusSucceeded || invoker.req.SkillID != "knowledge.extract" {
		t.Fatalf("unexpected skill result=%#v req=%#v", result, invoker.req)
	}
}

func TestAdaptersValidateRequiredConfig(t *testing.T) {
	cases := []struct {
		name    string
		adapter NodeAdapter
		step    StepDefinition
	}{
		{
			name:    "capability",
			adapter: NewCapabilityAdapter(&testCapabilityInvoker{}),
			step: StepDefinition{ID: "cap", Config: map[string]any{
				"capability_id":      "com.corex.demo",
				"preferred_protocol": "rest",
				"input_path":         "$.in",
				"output_path":        "$.out",
			}},
		},
		{
			name:    "metadata",
			adapter: NewMetadataAdapter(testMetadataClassifier{}),
			step: StepDefinition{ID: "meta", Config: map[string]any{
				"taxonomy_namespace":      "corex.customer",
				"tag_namespace":           "corex.customer",
				"dictionary_namespace":    "corex.customer.level",
				"resource_type_namespace": "corex.customer",
				"input_path":              "$.in",
				"output_path":             "$.out",
			}},
		},
		{
			name:    "knowledge.stage",
			adapter: NewKnowledgeStageAdapter(testKnowledgeOperator{}),
			step: StepDefinition{ID: "stage", Config: map[string]any{
				"knowledge_space_uuid": "2e1ab018-f7e6-4b1d-aaac-4c3d0d21cb71",
				"draft_schema_ref":     "knowledge.draft.v1",
				"input_path":           "$.in",
				"output_path":          "$.out",
			}},
		},
		{
			name:    "knowledge.publish",
			adapter: NewKnowledgePublishAdapter(testKnowledgeOperator{}),
			step: StepDefinition{ID: "publish", Config: map[string]any{
				"knowledge_space_uuid": "2e1ab018-f7e6-4b1d-aaac-4c3d0d21cb71",
				"draft_refs_path":      "$.drafts",
				"review_result_path":   "$.review",
				"publish_policy":       "review_required",
			}},
		},
		{
			name:    "event",
			adapter: NewEventAdapter(&testWorkflowEventPublisher{}),
			step: StepDefinition{ID: "event", Config: map[string]any{
				"topic":            "workflow.knowledge.published",
				"payload_path":     "$.payload",
				"event_schema_ref": "workflow.knowledge.published.v1",
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.adapter.Validate(tc.step); err != nil {
				t.Fatalf("validate: %v", err)
			}
			result, err := tc.adapter.Execute(context.Background(), NodeExecutionContext{TenantUUID: "tenant-a", Step: tc.step})
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if result.Status != NodeResultStatusSucceeded {
				t.Fatalf("expected succeeded, got %#v", result)
			}
		})
	}
}

func TestCapabilityAdapterDryRunSkipsInvokerAndResolvesInputPlaceholder(t *testing.T) {
	invoker := &testCapabilityInvoker{}
	adapter := NewCapabilityAdapter(invoker)
	result, err := adapter.Execute(context.Background(), NodeExecutionContext{
		Instance: &modelworkflow.WorkflowInstance{
			InputContext: toJSONOrEmpty(map[string]any{
				"capability_id": "com.corex.llm.models.list",
				"request": map[string]any{
					"payload": map[string]any{"dry_run": true},
				},
			}),
		},
		Step: StepDefinition{ID: "invoke", Config: map[string]any{
			"capability_id":      "${capability_id}",
			"preferred_protocol": "rest",
			"input_path":         "$.artifacts.request",
			"output_path":        "$.vars.capability_result",
		}},
	})
	if err != nil {
		t.Fatalf("execute dry-run capability: %v", err)
	}
	if result.Status != NodeResultStatusSucceeded || invoker.calls != 0 {
		t.Fatalf("unexpected dry-run result=%#v calls=%d", result, invoker.calls)
	}
	if result.Output["capability_id"] != "com.corex.llm.models.list" || result.Output["dry_run"] != true {
		t.Fatalf("unexpected dry-run output=%#v", result.Output)
	}
}
