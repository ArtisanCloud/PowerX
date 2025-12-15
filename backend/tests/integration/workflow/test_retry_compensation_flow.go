package workflowintegration

import (
	"context"
	"testing"
	"time"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	eventbus "github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/tests/workflow/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/ArtisanCloud/PowerX/internal/service/workflow"
)

const retryCompTenantUUID = "workflow-retry-comp"

func TestRetryCompensationFlow(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	mockQueue := newMemoryQueue()
	clock := &controlledClock{now: time.Now()}
	scheduler := workflow.NewScheduler(mockQueue)
	scheduler.WithClock(clock.Now)

	env.OverrideService(workflow.ServiceOptions{
		Scheduler: scheduler,
	})

	definition, err := env.Service.CreateDefinition(ctx, workflow.CreateDefinitionInput{
		TenantUUID: retryCompTenantUUID,
		Name:       "retry-compensation-demo",
		CreatedBy:  uuid.New(),
		DefaultRetryPolicy: map[string]any{
			"max_attempts":        2,
			"initial_interval_ms": 1000,
			"backoff_multiplier":  2.0,
			"max_interval_ms":     5000,
		},
		Steps: []workflow.StepDefinition{
			{
				ID:          "start",
				Type:        "system",
				NextStepIDs: []string{"agent_step"},
			},
			{
				ID:            "agent_step",
				Type:          "agent",
				NextStepIDs:   []string{"finalize"},
				Compensatable: true,
				Config: map[string]any{
					"capability": "demo.capability",
					"agent_id":   uuid.New().String(),
				},
			},
			{
				ID:   "finalize",
				Type: "system",
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, definition)

	_, err = env.Service.PublishDefinition(ctx, workflow.PublishDefinitionInput{
		TenantUUID:     retryCompTenantUUID,
		DefinitionUUID: definition.UUID,
		PublishedBy:    uuid.New(),
	})
	require.NoError(t, err)

	instance, err := env.Service.StartInstance(ctx, workflow.StartInstanceInput{
		TenantUUID:     retryCompTenantUUID,
		DefinitionUUID: definition.UUID,
		Input:          map[string]any{"ref": "retry-flow"},
	})
	require.NoError(t, err)

	var stepRecord modelworkflow.WorkflowStepRecord
	require.NoError(t, env.DB.
		Where("instance_uuid = ? AND step_id = ?", instance.UUID, "agent_step").
		First(&stepRecord).Error)

	resultOne, err := env.Service.HandleStepFailure(ctx, workflow.StepFailureInput{
		TenantUUID:   retryCompTenantUUID,
		InstanceUUID: instance.UUID,
		StepRecordID: stepRecord.ID,
		StepID:       "agent_step",
		Reason:       "agent unreachable",
	})
	require.NoError(t, err)
	require.True(t, resultOne.RetryScheduled)
	require.False(t, resultOne.CompensationTriggered)
	require.Len(t, mockQueue.items, 1)
	require.Equal(t, 2, mockQueue.items[0].Attempt)
	require.Equal(t, "agent_step", mockQueue.items[0].Metadata["step_id"])

	retryRecord := &modelworkflow.WorkflowStepRecord{
		InstanceUUID:   instance.UUID,
		StepID:         "agent_step",
		Type:           "agent",
		State:          "in_progress",
		Attempt:        2,
		SubjectType:    "agent",
		ScheduledAt:    clock.Now(),
		LastTransition: clock.Now(),
	}
	require.NoError(t, env.DB.Create(retryRecord).Error)

	resultTwo, err := env.Service.HandleStepFailure(ctx, workflow.StepFailureInput{
		TenantUUID:   retryCompTenantUUID,
		InstanceUUID: instance.UUID,
		StepRecordID: retryRecord.ID,
		StepID:       "agent_step",
		Reason:       "agent still failing",
	})
	require.NoError(t, err)
	require.False(t, resultTwo.RetryScheduled)
	require.True(t, resultTwo.CompensationTriggered)

	var compRecord modelworkflow.WorkflowStepCompensation
	require.NoError(t, env.DB.
		Where("step_record_id = ?", retryRecord.ID).
		First(&compRecord).Error)

	updatedInstance, _, err := env.Service.GetInstance(ctx, instance.TenantUUID, instance.UUID, false)
	require.NoError(t, err)
	require.Equal(t, "compensating", updatedInstance.State)
}

type memoryQueue struct {
	items []eventbus.RetryItem
}

func newMemoryQueue() *memoryQueue {
	return &memoryQueue{items: make([]eventbus.RetryItem, 0)}
}

func (m *memoryQueue) ScheduleRetry(ctx context.Context, item eventbus.RetryItem) error {
	m.items = append(m.items, item)
	return nil
}

func (m *memoryQueue) PopDueRetries(ctx context.Context, tenantKey string, now time.Time, limit int) ([]eventbus.RetryItem, error) {
	due := make([]eventbus.RetryItem, 0)
	remaining := make([]eventbus.RetryItem, 0)
	for _, it := range m.items {
		if !it.ExecuteAt.After(now) && len(due) < limit {
			due = append(due, it)
		} else {
			remaining = append(remaining, it)
		}
	}
	m.items = remaining
	return due, nil
}

func (m *memoryQueue) RemoveRetry(ctx context.Context, item eventbus.RetryItem) error {
	next := make([]eventbus.RetryItem, 0, len(m.items))
	for _, it := range m.items {
		if it.EnvelopeUUID != item.EnvelopeUUID {
			next = append(next, it)
		}
	}
	m.items = next
	return nil
}

func (m *memoryQueue) AcquireLease(ctx context.Context, lease eventbus.DeliveryLease) (bool, error) {
	return true, nil
}

func (m *memoryQueue) ReleaseLease(ctx context.Context, lease eventbus.DeliveryLease) error {
	return nil
}

type controlledClock struct {
	now time.Time
}

func (c *controlledClock) Now() time.Time {
	return c.now
}

func (c *controlledClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}
