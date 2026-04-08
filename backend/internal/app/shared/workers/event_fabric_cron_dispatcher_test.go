package workers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	cronscheduler "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/scheduler"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
)

func TestCronDispatcherWorker_EnqueueDueTask(t *testing.T) {
	now := time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)
	taskRepo := newMemoryScheduledTaskStore()
	runRepo := newMemoryScheduledTaskRunStore()
	driver := &cronDispatcherTaskDriverStub{}

	dueAt := now.Add(-time.Minute)
	task := &eventfabricmodel.ScheduledTask{
		TenantUUID:    "tenant-a",
		JobKey:        "job.due.1",
		Name:          "Due Task",
		Status:        eventfabricmodel.ScheduledTaskStatusEnabled,
		CronExpr:      "*/5 * * * *",
		Timezone:      "UTC",
		MisfirePolicy: eventfabricmodel.ScheduledTaskMisfireSkip,
		NextRunAt:     &dueAt,
		MaxRetry:      3,
	}
	task.BeforeCreate(nil)
	taskRepo.save(task)

	worker := NewEventFabricCronDispatcherWorker(EventFabricCronDispatcherWorkerOptions{
		TaskRepository:    taskRepo,
		TaskRunRepository: runRepo,
		TaskDriver:        driver,
		Scheduler:         cronscheduler.NewService(),
		Interval:          time.Second,
		BatchSize:         10,
		Clock:             func() time.Time { return now },
	})
	if worker == nil {
		t.Fatalf("worker should not be nil")
	}

	worker.TriggerNow(context.Background())

	if len(driver.enqueued) != 1 {
		t.Fatalf("expected 1 enqueued message, got %d", len(driver.enqueued))
	}
	if driver.enqueued[0].TenantKey != "tenant-a" {
		t.Fatalf("unexpected tenant key: %s", driver.enqueued[0].TenantKey)
	}

	savedTask := taskRepo.mustGet(task.UUID)
	if savedTask.LastRunAt == nil {
		t.Fatalf("last_run_at should be updated")
	}
	if savedTask.NextRunAt == nil || !savedTask.NextRunAt.After(now) {
		t.Fatalf("next_run_at should be greater than now")
	}

	runs := runRepo.listByTask(task.UUID)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run record, got %d", len(runs))
	}
	if runs[0].Status != eventfabricmodel.ScheduledTaskRunStatusSucceeded {
		t.Fatalf("run should be succeeded, got %s", runs[0].Status)
	}
}

func TestCronDispatcherWorker_EnqueueFailureWillRetry(t *testing.T) {
	now := time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)
	taskRepo := newMemoryScheduledTaskStore()
	runRepo := newMemoryScheduledTaskRunStore()
	driver := &cronDispatcherTaskDriverStub{enqueueErr: errors.New("enqueue failed")}

	dueAt := now.Add(-time.Minute)
	task := &eventfabricmodel.ScheduledTask{
		TenantUUID:      "tenant-a",
		JobKey:          "job.retry.1",
		Name:            "Retry Task",
		Status:          eventfabricmodel.ScheduledTaskStatusEnabled,
		CronExpr:        "*/5 * * * *",
		Timezone:        "UTC",
		MisfirePolicy:   eventfabricmodel.ScheduledTaskMisfireFireNow,
		NextRunAt:       &dueAt,
		MaxRetry:        2,
		RetryBackoffSec: 7,
	}
	task.BeforeCreate(nil)
	taskRepo.save(task)

	worker := NewEventFabricCronDispatcherWorker(EventFabricCronDispatcherWorkerOptions{
		TaskRepository:    taskRepo,
		TaskRunRepository: runRepo,
		TaskDriver:        driver,
		Scheduler:         cronscheduler.NewService(),
		Clock:             func() time.Time { return now },
	})
	worker.TriggerNow(context.Background())

	if len(driver.retries) != 1 {
		t.Fatalf("expected 1 retry request, got %d", len(driver.retries))
	}
	if !driver.retries[0].RetryAt.Equal(now.Add(7 * time.Second)) {
		t.Fatalf("unexpected retryAt: %s", driver.retries[0].RetryAt.Format(time.RFC3339))
	}

	runs := runRepo.listByTask(task.UUID)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run record, got %d", len(runs))
	}
	if runs[0].FailureCategory != "retry" || !runs[0].FailureRetryable {
		t.Fatalf("run should be marked as retry failure, category=%s retryable=%v", runs[0].FailureCategory, runs[0].FailureRetryable)
	}
}

func TestCronDispatcherWorker_EnqueueFailureWillDLQWhenRetryFails(t *testing.T) {
	now := time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)
	taskRepo := newMemoryScheduledTaskStore()
	runRepo := newMemoryScheduledTaskRunStore()
	driver := &cronDispatcherTaskDriverStub{
		enqueueErr: errors.New("enqueue failed"),
		retryErr:   errors.New("retry failed"),
	}

	dueAt := now.Add(-time.Minute)
	task := &eventfabricmodel.ScheduledTask{
		TenantUUID:      "tenant-a",
		JobKey:          "job.dlq.1",
		Name:            "DLQ Task",
		Status:          eventfabricmodel.ScheduledTaskStatusEnabled,
		CronExpr:        "*/5 * * * *",
		Timezone:        "UTC",
		MisfirePolicy:   eventfabricmodel.ScheduledTaskMisfireCatchUp,
		NextRunAt:       &dueAt,
		MaxRetry:        1,
		RetryBackoffSec: 3,
	}
	task.BeforeCreate(nil)
	taskRepo.save(task)

	worker := NewEventFabricCronDispatcherWorker(EventFabricCronDispatcherWorkerOptions{
		TaskRepository:    taskRepo,
		TaskRunRepository: runRepo,
		TaskDriver:        driver,
		Scheduler:         cronscheduler.NewService(),
		Clock:             func() time.Time { return now },
	})
	worker.TriggerNow(context.Background())

	runs := runRepo.listByTask(task.UUID)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run record, got %d", len(runs))
	}
	if runs[0].FailureCategory != "dlq" || runs[0].FailureRetryable {
		t.Fatalf("run should be marked as dlq failure, category=%s retryable=%v", runs[0].FailureCategory, runs[0].FailureRetryable)
	}

	savedTask := taskRepo.mustGet(task.UUID)
	if savedTask.LastError == "" {
		t.Fatalf("last_error should be updated")
	}
	if len(savedTask.LastError) < 4 || savedTask.LastError[:4] != "dlq:" {
		t.Fatalf("last_error should start with dlq:, got %s", savedTask.LastError)
	}
}

func TestCronDispatcherWorker_BackoffInterval(t *testing.T) {
	worker := NewEventFabricCronDispatcherWorker(EventFabricCronDispatcherWorkerOptions{
		TaskRepository:    newMemoryScheduledTaskStore(),
		TaskRunRepository: newMemoryScheduledTaskRunStore(),
		TaskDriver:        &cronDispatcherTaskDriverStub{},
		Scheduler:         cronscheduler.NewService(),
		Interval:          5 * time.Second,
		MaxInterval:       30 * time.Second,
	})
	if worker == nil {
		t.Fatalf("worker should not be nil")
	}

	if got := worker.nextBackoffInterval(5 * time.Second); got != 10*time.Second {
		t.Fatalf("expected 10s, got %s", got)
	}
	if got := worker.nextBackoffInterval(10 * time.Second); got != 20*time.Second {
		t.Fatalf("expected 20s, got %s", got)
	}
	if got := worker.nextBackoffInterval(20 * time.Second); got != 30*time.Second {
		t.Fatalf("expected 30s, got %s", got)
	}
	if got := worker.nextBackoffInterval(30 * time.Second); got != 30*time.Second {
		t.Fatalf("expected capped 30s, got %s", got)
	}
	if got := worker.nextBackoffInterval(2 * time.Second); got != 10*time.Second {
		t.Fatalf("expected normalized to 10s, got %s", got)
	}
}

type memoryScheduledTaskStore struct {
	mu    sync.Mutex
	tasks map[uuid.UUID]*eventfabricmodel.ScheduledTask
}

func newMemoryScheduledTaskStore() *memoryScheduledTaskStore {
	return &memoryScheduledTaskStore{tasks: map[uuid.UUID]*eventfabricmodel.ScheduledTask{}}
}

func (m *memoryScheduledTaskStore) save(task *eventfabricmodel.ScheduledTask) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copyTask := *task
	m.tasks[copyTask.UUID] = &copyTask
}

func (m *memoryScheduledTaskStore) mustGet(id uuid.UUID) *eventfabricmodel.ScheduledTask {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.tasks[id]
	if !ok {
		return nil
	}
	copyTask := *item
	return &copyTask
}

func (m *memoryScheduledTaskStore) ListDueTasks(_ context.Context, now time.Time, tenantUUID string, limit int) ([]*eventfabricmodel.ScheduledTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*eventfabricmodel.ScheduledTask, 0)
	for _, item := range m.tasks {
		if item.Status != eventfabricmodel.ScheduledTaskStatusEnabled {
			continue
		}
		if tenantUUID != "" && item.TenantUUID != tenantUUID {
			continue
		}
		if item.NextRunAt == nil || item.NextRunAt.After(now) {
			continue
		}
		copyTask := *item
		out = append(out, &copyTask)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memoryScheduledTaskStore) UpdateFields(_ context.Context, taskUUID uuid.UUID, fields map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.tasks[taskUUID]
	if !ok {
		return nil
	}
	if v, ok := fields["last_run_at"].(time.Time); ok {
		t := v.UTC()
		item.LastRunAt = &t
	}
	if v, ok := fields["next_run_at"].(time.Time); ok {
		t := v.UTC()
		item.NextRunAt = &t
	}
	if v, ok := fields["last_error"].(string); ok {
		item.LastError = v
	}
	return nil
}

type memoryScheduledTaskRunStore struct {
	mu   sync.Mutex
	runs map[uuid.UUID]*eventfabricmodel.ScheduledTaskRun
}

func newMemoryScheduledTaskRunStore() *memoryScheduledTaskRunStore {
	return &memoryScheduledTaskRunStore{runs: map[uuid.UUID]*eventfabricmodel.ScheduledTaskRun{}}
}

func (m *memoryScheduledTaskRunStore) Create(_ context.Context, run *eventfabricmodel.ScheduledTaskRun) (*eventfabricmodel.ScheduledTaskRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copyRun := *run
	_ = copyRun.BeforeCreate(nil)
	m.runs[copyRun.UUID] = &copyRun
	return &copyRun, nil
}

func (m *memoryScheduledTaskRunStore) UpdateFields(_ context.Context, runUUID uuid.UUID, fields map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runUUID]
	if !ok {
		return nil
	}
	if v, ok := fields["status"].(string); ok {
		run.Status = v
	}
	if v, ok := fields["finished_at"].(time.Time); ok {
		t := v.UTC()
		run.FinishedAt = &t
	}
	if v, ok := fields["error_message"].(string); ok {
		run.ErrorMessage = v
	}
	if v, ok := fields["failure_category"].(string); ok {
		run.FailureCategory = v
	}
	if v, ok := fields["failure_retryable"].(bool); ok {
		run.FailureRetryable = v
	}
	return nil
}

func (m *memoryScheduledTaskRunStore) listByTask(taskUUID uuid.UUID) []*eventfabricmodel.ScheduledTaskRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*eventfabricmodel.ScheduledTaskRun, 0)
	for _, item := range m.runs {
		if item.ScheduledTaskUUID != taskUUID {
			continue
		}
		copyRun := *item
		out = append(out, &copyRun)
	}
	return out
}

type cronDispatcherTaskDriverStub struct {
	mu         sync.Mutex
	enqueueErr error
	retryErr   error
	enqueued   []event_bus.TaskMessage
	retries    []event_bus.RetryRequest
}

func (s *cronDispatcherTaskDriverStub) Type() event_bus.QueueDriverType {
	return event_bus.QueueDriverMemory
}

func (s *cronDispatcherTaskDriverStub) Capability() event_bus.QueueDriverCapability {
	return event_bus.QueueDriverCapability{}
}

func (s *cronDispatcherTaskDriverStub) Enqueue(_ context.Context, message event_bus.TaskMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enqueued = append(s.enqueued, message)
	return s.enqueueErr
}

func (s *cronDispatcherTaskDriverStub) Dequeue(context.Context, event_bus.DequeueRequest) ([]event_bus.TaskMessage, error) {
	return nil, nil
}

func (s *cronDispatcherTaskDriverStub) Ack(context.Context, event_bus.AckRequest) error {
	return nil
}

func (s *cronDispatcherTaskDriverStub) Nack(context.Context, event_bus.NackRequest) error {
	return nil
}

func (s *cronDispatcherTaskDriverStub) Retry(_ context.Context, request event_bus.RetryRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retries = append(s.retries, request)
	return s.retryErr
}
