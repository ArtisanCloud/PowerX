package workflowunit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/workflow"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeGrantValidator struct {
	err     error
	grantID string
	version int64
}

func (f *fakeGrantValidator) ValidateAgentGrant(ctx context.Context, tenantID uint64, agentID uuid.UUID, capability string) (workflow.GrantValidationResult, error) {
	if f.err != nil {
		return workflow.GrantValidationResult{}, f.err
	}
	return workflow.GrantValidationResult{
		GrantID:        f.grantID,
		GrantVersion:   f.version,
		CapabilityName: capability,
	}, nil
}

type stubStepStore struct {
	records map[uint64]*modelworkflow.WorkflowStepRecord
}

func newStubStepStore() *stubStepStore {
	return &stubStepStore{records: make(map[uint64]*modelworkflow.WorkflowStepRecord)}
}

func (s *stubStepStore) AppendRecord(ctx context.Context, record *modelworkflow.WorkflowStepRecord) (*modelworkflow.WorkflowStepRecord, error) {
	if record == nil {
		return nil, errors.New("record nil")
	}
	nextID := uint64(len(s.records) + 1)
	record.ID = nextID
	copied := *record
	s.records[nextID] = &copied
	return &copied, nil
}

func (s *stubStepStore) GetByID(ctx context.Context, id uint64) (*modelworkflow.WorkflowStepRecord, error) {
	rec, ok := s.records[id]
	if !ok {
		return nil, errors.New("record not found")
	}
	copied := *rec
	return &copied, nil
}

func (s *stubStepStore) ListByInstance(ctx context.Context, instanceUUID uuid.UUID) ([]modelworkflow.WorkflowStepRecord, error) {
	items := make([]modelworkflow.WorkflowStepRecord, 0)
	for _, rec := range s.records {
		if rec.InstanceUUID == instanceUUID {
			items = append(items, *rec)
		}
	}
	return items, nil
}

func (s *stubStepStore) FindLatestByStep(ctx context.Context, instanceUUID uuid.UUID, stepID string) (*modelworkflow.WorkflowStepRecord, error) {
	var latest *modelworkflow.WorkflowStepRecord
	for _, rec := range s.records {
		if rec.InstanceUUID == instanceUUID && rec.StepID == stepID {
			if latest == nil || rec.ID > latest.ID {
				copied := *rec
				latest = &copied
			}
		}
	}
	if latest == nil {
		return nil, errors.New("not found")
	}
	return latest, nil
}

func (s *stubStepStore) UpdateState(ctx context.Context, id uint64, nextState string, updates map[string]interface{}) error {
	rec, ok := s.records[id]
	if !ok {
		return errors.New("record not found")
	}
	if nextState != "" {
		rec.State = nextState
	}
	if v, ok := updates["failure_reason"].(string); ok {
		rec.FailureReason = v
	}
	if v, ok := updates["attempt"].(int); ok {
		rec.Attempt = int32(v)
	}
	if v, ok := updates["attempt"].(int32); ok {
		rec.Attempt = v
	}
	if v, ok := updates["subject_uuid"].(uuid.UUID); ok {
		rec.SubjectUUID = v
	}
	if v, ok := updates["tool_grant_id"].(string); ok {
		rec.ToolGrantID = v
	}
	if v, ok := updates["tool_grant_version"].(int64); ok {
		rec.ToolGrantVer = v
	}
	if v, ok := updates["started_at"].(time.Time); ok {
		rec.StartedAt = &v
	}
	return nil
}

type stubAssignmentStore struct {
	nextID      uint64
	assignments map[uint64]*modelworkflow.AgentAssignment
}

func newStubAssignmentStore() *stubAssignmentStore {
	return &stubAssignmentStore{assignments: make(map[uint64]*modelworkflow.AgentAssignment)}
}

func (s *stubAssignmentStore) CreateAssignment(ctx context.Context, assignment *modelworkflow.AgentAssignment) (*modelworkflow.AgentAssignment, error) {
	if assignment == nil {
		return nil, errors.New("assignment nil")
	}
	s.nextID++
	assignment.ID = s.nextID
	copied := *assignment
	s.assignments[s.nextID] = &copied
	return &copied, nil
}

func (s *stubAssignmentStore) GetLatestByStep(ctx context.Context, stepRecordID uint64) (*modelworkflow.AgentAssignment, error) {
	var latest *modelworkflow.AgentAssignment
	for _, item := range s.assignments {
		if item.StepRecordID == stepRecordID {
			if latest == nil || item.ID > latest.ID {
				copied := *item
				latest = &copied
			}
		}
	}
	if latest == nil {
		return nil, errors.New("assignment not found")
	}
	return latest, nil
}

func (s *stubAssignmentStore) FindOpenAssignments(ctx context.Context, agentUUID uuid.UUID, statuses []string, limit int) ([]modelworkflow.AgentAssignment, error) {
	return nil, nil
}

func (s *stubAssignmentStore) UpdateStatus(ctx context.Context, id uint64, status string, updates map[string]interface{}) error {
	item, ok := s.assignments[id]
	if !ok {
		return errors.New("assignment not found")
	}
	item.Status = status
	if v, ok := updates["completed_at"].(time.Time); ok {
		item.CompletedAt = &v
	}
	return nil
}

func (s *stubAssignmentStore) FindTimedOutAssignments(ctx context.Context, tenantID uint64, before time.Time, limit int) ([]modelworkflow.AgentAssignment, error) {
	results := make([]modelworkflow.AgentAssignment, 0, len(s.assignments))
	for _, item := range s.assignments {
		results = append(results, *item)
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results, nil
}

func TestAssignmentTrackerValidatesGrantAndTimeout(t *testing.T) {
	ctx := context.Background()
	stepStore := newStubStepStore()
	assignmentStore := newStubAssignmentStore()

	instanceUUID := uuid.New()
	stepRecord := &modelworkflow.WorkflowStepRecord{
		PowerModel:     coremodel.PowerModel{ID: 1},
		InstanceUUID:   instanceUUID,
		StepID:         "agent_step",
		Type:           "agent",
		State:          "queued",
		SubjectType:    "agent",
		ScheduledAt:    time.Now(),
		LastTransition: time.Now(),
	}
	stepStore.records[1] = stepRecord

	validator := &fakeGrantValidator{err: errors.New("grant revoked")}
	tracker := workflow.NewAssignmentTracker(workflow.AssignmentTrackerOptions{
		AssignmentStore: assignmentStore,
		StepStore:       stepStore,
		GrantValidator:  validator,
		Clock:           time.Now,
		AckTimeout:      time.Minute,
	})

	dispatchInput := workflow.AssignmentDispatchInput{
		TenantID:     3003,
		InstanceUUID: instanceUUID,
		StepRecordID: stepRecord.ID,
		StepID:       stepRecord.StepID,
		AgentUUID:    uuid.New(),
		Capability:   "demo.capability",
	}

	_, err := tracker.Dispatch(ctx, dispatchInput)
	require.Error(t, err)

	validator.err = nil
	validator.grantID = "grant-777"
	validator.version = 5

	assignment, err := tracker.Dispatch(ctx, dispatchInput)
	require.NoError(t, err)
	require.Equal(t, "dispatched", assignment.Status)
	require.Len(t, assignmentStore.assignments, 1)

	updatedStep, err := stepStore.GetByID(ctx, stepRecord.ID)
	require.NoError(t, err)
	require.Equal(t, "in_progress", updatedStep.State)
	require.Equal(t, int32(1), updatedStep.Attempt)
	require.Equal(t, validator.grantID, updatedStep.ToolGrantID)
	require.Equal(t, validator.version, updatedStep.ToolGrantVer)

	deadline := time.Now().Add(-time.Minute)
	assignment.AckDeadline = &deadline
	if stored := assignmentStore.assignments[assignment.ID]; stored != nil {
		stored.AckDeadline = &deadline
	}

	result, err := tracker.ProcessTimeouts(ctx, workflow.TimeoutProcessOptions{
		TenantID: 3003,
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, result.TimedOutAssignments, 1)

	latest, err := stepStore.GetByID(ctx, stepRecord.ID)
	require.NoError(t, err)
	require.Equal(t, "queued", latest.State)
	require.Equal(t, int32(2), latest.Attempt)
}
