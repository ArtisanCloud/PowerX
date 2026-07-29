package workflow

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
)

type runnerDefinitionStore struct {
	definition *modelworkflow.WorkflowDefinition
}

func (s runnerDefinitionStore) CreateDefinition(context.Context, *modelworkflow.WorkflowDefinition) (*modelworkflow.WorkflowDefinition, error) {
	return nil, errors.New("not used")
}

func (s runnerDefinitionStore) NextVersion(context.Context, string, string) (int32, error) {
	return 0, errors.New("not used")
}

func (s runnerDefinitionStore) GetByUUID(context.Context, string, uuid.UUID, *int32) (*modelworkflow.WorkflowDefinition, error) {
	return s.definition, nil
}

func (s runnerDefinitionStore) GetLatestPublished(context.Context, string, uuid.UUID) (*modelworkflow.WorkflowDefinition, error) {
	return s.definition, nil
}

func (s runnerDefinitionStore) ListByTenant(context.Context, string, []string, string, int, int) ([]modelworkflow.WorkflowDefinition, int64, error) {
	return nil, 0, errors.New("not used")
}

func (s runnerDefinitionStore) UpdateStatus(context.Context, string, uuid.UUID, int32, string, map[string]interface{}) error {
	return errors.New("not used")
}

type runnerInstanceStore struct {
	instance *modelworkflow.WorkflowInstance
}

func (s *runnerInstanceStore) CreateInstance(context.Context, *modelworkflow.WorkflowInstance) (*modelworkflow.WorkflowInstance, error) {
	return nil, errors.New("not used")
}

func (s *runnerInstanceStore) GetByUUID(context.Context, string, uuid.UUID) (*modelworkflow.WorkflowInstance, error) {
	return s.instance, nil
}

func (s *runnerInstanceStore) GetByUUIDAnyTenant(context.Context, uuid.UUID) (*modelworkflow.WorkflowInstance, error) {
	return s.instance, nil
}

func (s *runnerInstanceStore) ListInstances(context.Context, workflowrepo.InstanceListFilter) ([]modelworkflow.WorkflowInstance, int64, error) {
	return nil, 0, errors.New("not used")
}

func (s *runnerInstanceStore) UpdateState(_ context.Context, _ string, _ uuid.UUID, nextState string, updates map[string]interface{}) error {
	if nextState != "" {
		s.instance.State = nextState
	}
	if v, ok := updates["current_step_id"].(string); ok {
		s.instance.CurrentStepID = v
	}
	if v, ok := updates["last_error"].(string); ok {
		s.instance.LastError = v
	}
	return nil
}

type runnerStepStore struct {
	mu      sync.Mutex
	records []modelworkflow.WorkflowStepRecord
	nextID  uint64
}

func (s *runnerStepStore) AppendRecord(_ context.Context, record *modelworkflow.WorkflowStepRecord) (*modelworkflow.WorkflowStepRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	record.ID = s.nextID
	s.records = append(s.records, *record)
	return record, nil
}

func (s *runnerStepStore) GetByID(_ context.Context, id uint64) (*modelworkflow.WorkflowStepRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].ID == id {
			return &s.records[i], nil
		}
	}
	return nil, errors.New("not found")
}

func (s *runnerStepStore) ListByInstance(_ context.Context, instanceUUID uuid.UUID) ([]modelworkflow.WorkflowStepRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]modelworkflow.WorkflowStepRecord, 0, len(s.records))
	for _, record := range s.records {
		if record.InstanceUUID == instanceUUID {
			out = append(out, record)
		}
	}
	return out, nil
}

func (s *runnerStepStore) FindLatestByStep(_ context.Context, instanceUUID uuid.UUID, stepID string) (*modelworkflow.WorkflowStepRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.records) - 1; i >= 0; i-- {
		if s.records[i].InstanceUUID == instanceUUID && s.records[i].StepID == stepID {
			return &s.records[i], nil
		}
	}
	return nil, errors.New("not found")
}

func (s *runnerStepStore) UpdateState(_ context.Context, id uint64, nextState string, updates map[string]interface{}) error {
	_, err := s.UpdateStateForAttempt(context.Background(), id, 0, nextState, updates)
	return err
}

func (s *runnerStepStore) LeaseQueuedSteps(_ context.Context, limit int, leaseOwner string, leaseUntil time.Time) ([]modelworkflow.WorkflowStepRecord, error) {
	return s.leaseQueuedSteps(uuid.Nil, limit, leaseOwner, leaseUntil)
}

func (s *runnerStepStore) LeaseQueuedStepsByInstance(_ context.Context, instanceUUID uuid.UUID, limit int, leaseOwner string, leaseUntil time.Time) ([]modelworkflow.WorkflowStepRecord, error) {
	return s.leaseQueuedSteps(instanceUUID, limit, leaseOwner, leaseUntil)
}

func (s *runnerStepStore) leaseQueuedSteps(instanceUUID uuid.UUID, limit int, leaseOwner string, leaseUntil time.Time) ([]modelworkflow.WorkflowStepRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 10
	}
	out := make([]modelworkflow.WorkflowStepRecord, 0, limit)
	for i := range s.records {
		if len(out) >= limit {
			break
		}
		if s.records[i].State != "queued" {
			continue
		}
		if instanceUUID != uuid.Nil && s.records[i].InstanceUUID != instanceUUID {
			continue
		}
		s.records[i].State = "in_progress"
		s.records[i].LeaseOwner = leaseOwner
		s.records[i].LeaseUntil = &leaseUntil
		out = append(out, s.records[i])
	}
	return out, nil
}

func (s *runnerStepStore) UpdateStateForAttempt(_ context.Context, id uint64, attempt int32, nextState string, updates map[string]interface{}) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].ID != id || s.records[i].Attempt != attempt {
			continue
		}
		if nextState != "" {
			s.records[i].State = nextState
		}
		if v, ok := updates["payload_out"].(datatypes.JSON); ok {
			s.records[i].PayloadOut = v
		}
		if v, ok := updates["error_code"].(string); ok {
			s.records[i].ErrorCode = v
		}
		if v, ok := updates["error_message"].(string); ok {
			s.records[i].ErrorMessage = v
		}
		if v, ok := updates["failure_reason"].(string); ok {
			s.records[i].FailureReason = v
		}
		if v, ok := updates["awaiting_human"].(bool); ok {
			s.records[i].AwaitingHuman = v
		}
		return true, nil
	}
	return false, nil
}

type runnerAdapter struct {
	result NodeResult
}

func (a runnerAdapter) Spec() NodeAdapterSpec {
	return NodeAdapterSpec{NodeKind: "system.invoke"}
}

func (a runnerAdapter) Validate(StepDefinition) error {
	return nil
}

func (a runnerAdapter) Execute(context.Context, NodeExecutionContext) (NodeResult, error) {
	return a.result, nil
}

func TestRunnerCompletesStepAndEnqueuesNextStep(t *testing.T) {
	instanceUUID := uuid.New()
	definitionUUID := uuid.New()
	steps := []StepDefinition{
		{ID: "capture", Type: "system", NodeKind: "system.invoke", NextStepIDs: []string{"publish"}},
		{ID: "publish", Type: "system", NodeKind: "system.invoke", DependsOn: []string{"capture"}},
	}
	stepStore := &runnerStepStore{nextID: 1, records: []modelworkflow.WorkflowStepRecord{{
		InstanceUUID: instanceUUID,
		StepID:       "capture",
		Type:         "system",
		NodeKind:     "system.invoke",
		State:        "queued",
		SubjectType:  "system",
	}}}
	runner := newTestRunner(t, definitionUUID, instanceUUID, steps, stepStore, NodeResult{Status: NodeResultStatusSucceeded, Output: map[string]any{"ok": true}})

	result, err := runner.RunDueSteps(context.Background())
	if err != nil {
		t.Fatalf("run due steps: %v", err)
	}
	if result.Completed != 1 || result.Leased != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	records, err := stepStore.ListByInstance(context.Background(), instanceUUID)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected next step to be queued, got %d records", len(records))
	}
	if records[0].State != "completed" || records[1].StepID != "publish" || records[1].State != "queued" {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestRunnerConvergesInstanceToSucceededWhenNoActiveStepsRemain(t *testing.T) {
	instanceUUID := uuid.New()
	definitionUUID := uuid.New()
	stepStore := &runnerStepStore{nextID: 1, records: []modelworkflow.WorkflowStepRecord{{
		InstanceUUID: instanceUUID,
		StepID:       "only",
		Type:         "system",
		NodeKind:     "system.invoke",
		State:        "queued",
		SubjectType:  "system",
	}}}
	instanceStore := &runnerInstanceStore{instance: &modelworkflow.WorkflowInstance{
		PowerUUIDModel:    coremodel.PowerUUIDModel{UUID: instanceUUID},
		TenantUUID:        "tenant-a",
		DefinitionUUID:    definitionUUID,
		DefinitionVersion: 1,
		State:             "running",
	}}
	runner := newTestRunnerWithStores(t, runnerDefinitionStore{definition: testDefinition(definitionUUID, []StepDefinition{
		{ID: "only", Type: "system", NodeKind: "system.invoke"},
	})}, instanceStore, stepStore, NodeResult{Status: NodeResultStatusSucceeded})

	if _, err := runner.RunDueSteps(context.Background()); err != nil {
		t.Fatalf("run due steps: %v", err)
	}
	if instanceStore.instance.State != "succeeded" {
		t.Fatalf("expected instance succeeded, got %s", instanceStore.instance.State)
	}
}

func newTestRunner(t *testing.T, definitionUUID uuid.UUID, instanceUUID uuid.UUID, steps []StepDefinition, stepStore *runnerStepStore, result NodeResult) *Runner {
	t.Helper()
	instanceStore := &runnerInstanceStore{instance: &modelworkflow.WorkflowInstance{
		PowerUUIDModel:    coremodel.PowerUUIDModel{UUID: instanceUUID},
		TenantUUID:        "tenant-a",
		DefinitionUUID:    definitionUUID,
		DefinitionVersion: 1,
		State:             "running",
	}}
	defStore := runnerDefinitionStore{definition: testDefinition(definitionUUID, steps)}
	return newTestRunnerWithStores(t, defStore, instanceStore, stepStore, result)
}

func newTestRunnerWithStores(t *testing.T, defStore runnerDefinitionStore, instanceStore *runnerInstanceStore, stepStore *runnerStepStore, result NodeResult) *Runner {
	t.Helper()
	registry := NewNodeAdapterRegistry()
	if err := registry.Register(runnerAdapter{result: result}); err != nil {
		t.Fatalf("register adapter: %v", err)
	}
	runner, err := NewRunner(RunnerOptions{
		DefinitionStore: defStore,
		InstanceStore:   instanceStore,
		StepStore:       stepStore,
		AdapterRegistry: registry,
		LeaseOwner:      "test-runner",
		Clock: func() time.Time {
			return time.Unix(1000, 0).UTC()
		},
	})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	return runner
}

func testDefinition(definitionUUID uuid.UUID, steps []StepDefinition) *modelworkflow.WorkflowDefinition {
	return &modelworkflow.WorkflowDefinition{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: definitionUUID},
		TenantUUID:     "tenant-a",
		Version:        1,
		Status:         "published",
		StepGraph:      toJSONOrEmpty(steps),
	}
}
