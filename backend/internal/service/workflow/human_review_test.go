package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
)

type humanReviewStore struct {
	task *modelworkflow.HumanReviewTask
}

func (s *humanReviewStore) CreateTask(_ context.Context, task *modelworkflow.HumanReviewTask) (*modelworkflow.HumanReviewTask, error) {
	task.PowerUUIDModel.UUID = uuid.New()
	s.task = task
	return task, nil
}

func (s *humanReviewStore) GetByUUID(context.Context, string, uuid.UUID) (*modelworkflow.HumanReviewTask, error) {
	if s.task == nil {
		return nil, errors.New("not found")
	}
	return s.task, nil
}

func (s *humanReviewStore) ListTasks(context.Context, workflowrepo.HumanReviewTaskListFilter) ([]modelworkflow.HumanReviewTask, int64, error) {
	if s.task == nil {
		return nil, 0, nil
	}
	return []modelworkflow.HumanReviewTask{*s.task}, 1, nil
}

func (s *humanReviewStore) UpdateDecision(_ context.Context, _ string, _ uuid.UUID, status string, updates map[string]interface{}) error {
	if s.task == nil {
		return errors.New("not found")
	}
	s.task.Status = status
	if v, ok := updates["reviewer_user_uuid"].(uuid.UUID); ok {
		s.task.ReviewerUserUUID = v
	}
	if v, ok := updates["decision"].(string); ok {
		s.task.Decision = v
	}
	if v, ok := updates["comment"].(string); ok {
		s.task.Comment = v
	}
	return nil
}

func TestHumanReviewAdapterCreatesWaitingTask(t *testing.T) {
	store := &humanReviewStore{}
	adapter := NewHumanReviewAdapter(store)
	instanceUUID := uuid.New()
	result, err := adapter.Execute(context.Background(), NodeExecutionContext{
		TenantUUID: "tenant-a",
		Instance: &modelworkflow.WorkflowInstance{
			PowerUUIDModel: coremodel.PowerUUIDModel{UUID: instanceUUID},
		},
		Step: StepDefinition{
			ID: "review",
			Config: map[string]any{
				"review_type":         "knowledge_publish",
				"approver_policy":     map[string]any{"roles": []any{"knowledge_reviewer"}},
				"review_payload_path": "$.draft",
				"approved_route":      "publish",
				"rejected_route":      "revise",
			},
		},
		Payload: map[string]any{"draft": "demo"},
	})
	if err != nil {
		t.Fatalf("execute human adapter: %v", err)
	}
	if result.Status != NodeResultStatusWaiting || result.ReviewTaskUUID == uuid.Nil || store.task == nil {
		t.Fatalf("unexpected result=%#v task=%#v", result, store.task)
	}
}

func TestActHumanReviewTaskApproveCompletesStepAndQueuesApprovedRoute(t *testing.T) {
	instanceUUID := uuid.New()
	definitionUUID := uuid.New()
	taskUUID := uuid.New()
	steps := []StepDefinition{
		{ID: "capture", Type: "system", NodeKind: "input.capture", NextStepIDs: []string{"review"}},
		{
			ID:        "review",
			Type:      "human_approval",
			NodeKind:  "human.review",
			DependsOn: []string{"capture"},
			Config: map[string]any{
				"review_type":         "knowledge_publish",
				"approver_policy":     map[string]any{"roles": []any{"knowledge_reviewer"}},
				"review_payload_path": "$.draft",
				"approved_route":      "publish",
				"rejected_route":      "revise",
			},
		},
		{ID: "publish", Type: "system", NodeKind: "knowledge.publish", DependsOn: []string{"review"}, NextStepIDs: []string{"end"}},
		{ID: "end", Type: "system", NodeKind: "workflow.end", DependsOn: []string{"publish"}},
	}
	registry := NewNodeAdapterRegistry()
	if err := registry.Register(testNodeAdapter{spec: NodeAdapterSpec{NodeKind: "knowledge.publish"}}); err != nil {
		t.Fatalf("register adapter: %v", err)
	}
	if err := registry.Register(NewWorkflowEndAdapter()); err != nil {
		t.Fatalf("register end adapter: %v", err)
	}
	stepStore := &runnerStepStore{nextID: 1, records: []modelworkflow.WorkflowStepRecord{{
		PowerModel:     coremodel.PowerModel{ID: 1},
		InstanceUUID:   instanceUUID,
		StepID:         "review",
		Type:           "human_approval",
		NodeKind:       "human.review",
		State:          "waiting",
		SubjectType:    "human",
		AwaitingHuman:  true,
		ScheduledAt:    time.Unix(1000, 0).UTC(),
		LastTransition: time.Unix(1000, 0).UTC(),
	}}}
	reviewStore := &humanReviewStore{task: &modelworkflow.HumanReviewTask{
		PowerUUIDModel:       coremodel.PowerUUIDModel{UUID: taskUUID},
		TenantUUID:           "tenant-a",
		WorkflowInstanceUUID: instanceUUID,
		StepID:               "review",
		ReviewType:           "knowledge_publish",
		Status:               "pending",
	}}
	instanceStore := &runnerInstanceStore{instance: &modelworkflow.WorkflowInstance{
		PowerUUIDModel:    coremodel.PowerUUIDModel{UUID: instanceUUID},
		TenantUUID:        "tenant-a",
		DefinitionUUID:    definitionUUID,
		DefinitionVersion: 1,
		State:             "waiting",
	}}
	service := &Service{
		definitions: runnerDefinitionStore{definition: testDefinition(definitionUUID, steps)},
		instances:   instanceStore,
		steps:       stepStore,
		reviews:     reviewStore,
		adapters:    registry,
		now: func() time.Time {
			return time.Unix(2000, 0).UTC()
		},
	}

	updated, err := service.ActHumanReviewTask(context.Background(), HumanReviewActionInput{
		TenantUUID:     "tenant-a",
		ReviewTaskUUID: taskUUID,
		Action:         "approve",
		ReviewerUUID:   uuid.New(),
		Payload:        map[string]any{"approved": true},
	})
	if err != nil {
		t.Fatalf("act review task: %v", err)
	}
	if updated.Status != "approved" {
		t.Fatalf("expected approved task, got %s", updated.Status)
	}
	records, err := stepStore.ListByInstance(context.Background(), instanceUUID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected approved route queued, got records=%#v", records)
	}
	if records[0].State != "completed" || records[1].StepID != "publish" || records[1].State != "completed" || records[2].StepID != "end" || records[2].State != "completed" {
		t.Fatalf("unexpected records: %#v", records)
	}
	if instanceStore.instance.State != "succeeded" {
		t.Fatalf("expected instance succeeded, got %s", instanceStore.instance.State)
	}
}
