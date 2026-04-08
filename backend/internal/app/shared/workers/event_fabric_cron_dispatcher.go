package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	eventdomain "github.com/ArtisanCloud/PowerX/internal/event_bus"
	cronscheduler "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/scheduler"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
)

const (
	defaultCronDispatcherSubscriberID = eventdomain.SubscriberEventFabricCronDispatch
	defaultCronDispatcherTopic        = "event_fabric.cron.dispatch"
	defaultCronDispatcherMaxInterval  = 30 * time.Second
)

type scheduledTaskStore interface {
	ListDueTasks(ctx context.Context, now time.Time, tenantUUID string, limit int) ([]*eventfabricmodel.ScheduledTask, error)
	UpdateFields(ctx context.Context, taskUUID uuid.UUID, fields map[string]interface{}) error
}

type scheduledTaskRunStore interface {
	Create(ctx context.Context, run *eventfabricmodel.ScheduledTaskRun) (*eventfabricmodel.ScheduledTaskRun, error)
	UpdateFields(ctx context.Context, runUUID uuid.UUID, fields map[string]interface{}) error
}

type CronTaskDispatchPayload struct {
	TaskUUID    string `json:"task_uuid"`
	TenantUUID  string `json:"tenant_uuid"`
	JobKey      string `json:"job_key"`
	TaskName    string `json:"task_name"`
	TriggerType string `json:"trigger_type"`
	ScheduledAt string `json:"scheduled_at"`
}

type EventFabricCronDispatcherWorkerOptions struct {
	TaskRepository    scheduledTaskStore
	TaskRunRepository scheduledTaskRunStore
	TaskDriver        event_bus.TaskDriver
	Scheduler         *cronscheduler.Service
	SubscriberID      string
	Topic             string
	Interval          time.Duration
	MaxInterval       time.Duration
	BatchSize         int
	Logger            *pxlog.Logger
	Clock             func() time.Time
}

type EventFabricCronDispatcherWorker struct {
	taskRepo     scheduledTaskStore
	taskRunRepo  scheduledTaskRunStore
	taskDriver   event_bus.TaskDriver
	scheduler    *cronscheduler.Service
	subscriberID string
	topic        string
	interval     time.Duration
	maxInterval  time.Duration
	batchSize    int
	logger       *pxlog.Logger
	clock        func() time.Time
	paused       atomic.Bool
}

type cronDispatchCycleResult struct {
	dueCount int
	hadError bool
}

func NewEventFabricCronDispatcherWorker(opts EventFabricCronDispatcherWorkerOptions) *EventFabricCronDispatcherWorker {
	if opts.TaskRepository == nil || opts.TaskRunRepository == nil || opts.TaskDriver == nil || opts.Scheduler == nil {
		return nil
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	maxInterval := opts.MaxInterval
	if maxInterval <= 0 {
		maxInterval = defaultCronDispatcherMaxInterval
	}
	if maxInterval < interval {
		maxInterval = interval
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	subscriberID := strings.TrimSpace(opts.SubscriberID)
	if subscriberID == "" {
		subscriberID = defaultCronDispatcherSubscriberID
	}
	topic := strings.TrimSpace(opts.Topic)
	if topic == "" {
		topic = defaultCronDispatcherTopic
	}
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &EventFabricCronDispatcherWorker{
		taskRepo:     opts.TaskRepository,
		taskRunRepo:  opts.TaskRunRepository,
		taskDriver:   opts.TaskDriver,
		scheduler:    opts.Scheduler,
		subscriberID: subscriberID,
		topic:        topic,
		interval:     interval,
		maxInterval:  maxInterval,
		batchSize:    batchSize,
		logger:       logger,
		clock:        clock,
	}
}

func (w *EventFabricCronDispatcherWorker) Run(ctx context.Context) {
	if w == nil || w.taskRepo == nil || w.taskRunRepo == nil || w.taskDriver == nil || w.scheduler == nil {
		return
	}
	currentInterval := w.interval
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if !w.paused.Load() {
				result := w.flush(ctx)
				if result.dueCount > 0 {
					currentInterval = w.interval
				} else {
					currentInterval = w.nextBackoffInterval(currentInterval)
				}
			} else {
				currentInterval = w.nextBackoffInterval(currentInterval)
			}
			timer.Reset(currentInterval)
		}
	}
}

func (w *EventFabricCronDispatcherWorker) Pause() {
	if w == nil {
		return
	}
	w.paused.Store(true)
}

func (w *EventFabricCronDispatcherWorker) Resume() {
	if w == nil {
		return
	}
	w.paused.Store(false)
}

func (w *EventFabricCronDispatcherWorker) IsPaused() bool {
	if w == nil {
		return false
	}
	return w.paused.Load()
}

func (w *EventFabricCronDispatcherWorker) TriggerNow(ctx context.Context) {
	if w == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.flush(ctx)
}

func (w *EventFabricCronDispatcherWorker) Interval() time.Duration {
	if w == nil {
		return 0
	}
	return w.interval
}

func (w *EventFabricCronDispatcherWorker) BatchSize() int {
	if w == nil {
		return 0
	}
	return w.batchSize
}

func (w *EventFabricCronDispatcherWorker) SubscriberID() string {
	if w == nil {
		return ""
	}
	return w.subscriberID
}

func (w *EventFabricCronDispatcherWorker) Topic() string {
	if w == nil {
		return ""
	}
	return w.topic
}

func (w *EventFabricCronDispatcherWorker) flush(ctx context.Context) cronDispatchCycleResult {
	now := w.clock().UTC()
	tasks, err := w.taskRepo.ListDueTasks(ctx, now, "", w.batchSize)
	if err != nil {
		w.logger.WarnF(ctx, "[event_fabric.cron.dispatch] list due tasks failed: %v", err)
		return cronDispatchCycleResult{hadError: true}
	}
	if len(tasks) == 0 {
		return cronDispatchCycleResult{}
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		w.dispatchTask(ctx, now, task)
	}
	return cronDispatchCycleResult{dueCount: len(tasks)}
}

func (w *EventFabricCronDispatcherWorker) nextBackoffInterval(current time.Duration) time.Duration {
	if w == nil {
		return 0
	}
	if current < w.interval {
		current = w.interval
	}
	next := current * 2
	if next < w.interval {
		next = w.interval
	}
	if w.maxInterval > 0 && next > w.maxInterval {
		next = w.maxInterval
	}
	return next
}

func (w *EventFabricCronDispatcherWorker) dispatchTask(ctx context.Context, now time.Time, task *eventfabricmodel.ScheduledTask) {
	tenantKey := strings.TrimSpace(task.TenantUUID)
	if tenantKey == "" {
		tenantKey = "global"
	}
	run := &eventfabricmodel.ScheduledTaskRun{
		TenantUUID:        tenantKey,
		ScheduledTaskUUID: task.UUID,
		TriggerType:       eventfabricmodel.ScheduledTaskRunTriggerSchedule,
		Status:            eventfabricmodel.ScheduledTaskRunStatusRunning,
		Attempt:           1,
		StartedAt:         ptrTime(now),
		DispatchedAt:      ptrTime(now),
		TraceID:           task.TraceID,
	}
	createdRun, err := w.taskRunRepo.Create(ctx, run)
	if err != nil {
		w.logger.WarnF(ctx, "[event_fabric.cron.dispatch] create run failed task=%s err=%v", task.UUID.String(), err)
		return
	}

	payload, err := json.Marshal(CronTaskDispatchPayload{
		TaskUUID:    task.UUID.String(),
		TenantUUID:  tenantKey,
		JobKey:      task.JobKey,
		TaskName:    task.Name,
		TriggerType: eventfabricmodel.ScheduledTaskRunTriggerSchedule,
		ScheduledAt: now.Format(time.RFC3339),
	})
	if err != nil {
		_ = w.failRun(ctx, createdRun.UUID, err.Error(), "system", false, now)
		_ = w.taskRepo.UpdateFields(ctx, task.UUID, map[string]interface{}{"last_error": err.Error()})
		return
	}

	msgID := fmt.Sprintf("event_fabric.cron.%s.%d", task.UUID.String(), now.UnixMilli())
	taskMessage := event_bus.TaskMessage{
		ID:           msgID,
		TenantKey:    tenantKey,
		SubscriberID: w.subscriberID,
		Topic:        w.topic,
		Payload:      payload,
		TraceID:      task.TraceID,
		VisibleAt:    now,
		Metadata: map[string]string{
			"task_uuid": task.UUID.String(),
			"job_key":   task.JobKey,
			"source":    "event_fabric.cron",
		},
	}
	enqueueErr := w.taskDriver.Enqueue(ctx, taskMessage)
	if enqueueErr != nil {
		w.handleEnqueueFailure(ctx, now, task, createdRun, taskMessage, enqueueErr)
		return
	}

	next, nextErr := w.scheduler.ComputeNextRun(cronscheduler.ComputeNextRunInput{
		CronExpr:      task.CronExpr,
		Timezone:      task.Timezone,
		MisfirePolicy: task.MisfirePolicy,
		Now:           now,
		LastRunAt:     ptrTime(now),
		PrevNextRunAt: task.NextRunAt,
	})
	if nextErr != nil {
		_ = w.failRun(ctx, createdRun.UUID, nextErr.Error(), "system", false, now)
		_ = w.taskRepo.UpdateFields(ctx, task.UUID, map[string]interface{}{"last_error": nextErr.Error()})
		w.logger.WarnF(ctx, "[event_fabric.cron.dispatch] compute next run failed task=%s err=%v", task.UUID.String(), nextErr)
		return
	}

	updates := map[string]interface{}{
		"last_run_at": now,
		"last_error":  "",
	}
	if next != nil && next.NextRunAt != nil {
		updates["next_run_at"] = *next.NextRunAt
	}
	if err := w.taskRepo.UpdateFields(ctx, task.UUID, updates); err != nil {
		w.logger.WarnF(ctx, "[event_fabric.cron.dispatch] update task state failed task=%s err=%v", task.UUID.String(), err)
	}
	if err := w.taskRunRepo.UpdateFields(ctx, createdRun.UUID, map[string]interface{}{
		"status":      eventfabricmodel.ScheduledTaskRunStatusSucceeded,
		"finished_at": now.UTC(),
	}); err != nil {
		w.logger.WarnF(ctx, "[event_fabric.cron.dispatch] update run status failed task=%s err=%v", task.UUID.String(), err)
	}
}

func (w *EventFabricCronDispatcherWorker) handleEnqueueFailure(
	ctx context.Context,
	now time.Time,
	task *eventfabricmodel.ScheduledTask,
	run *eventfabricmodel.ScheduledTaskRun,
	message event_bus.TaskMessage,
	enqueueErr error,
) {
	retryBackoff := time.Duration(task.RetryBackoffSec) * time.Second
	if retryBackoff <= 0 {
		retryBackoff = 30 * time.Second
	}
	if task.MaxRetry > 0 {
		retryErr := w.taskDriver.Retry(ctx, event_bus.RetryRequest{
			Message: message,
			RetryAt: now.Add(retryBackoff),
			Reason:  enqueueErr.Error(),
		})
		if retryErr == nil {
			_ = w.failRun(ctx, run.UUID, enqueueErr.Error(), "retry", true, now)
			_ = w.taskRepo.UpdateFields(ctx, task.UUID, map[string]interface{}{"last_error": enqueueErr.Error()})
			w.logger.WarnF(ctx, "[event_fabric.cron.dispatch] enqueue failed, moved to retry task=%s err=%v", task.UUID.String(), enqueueErr)
			return
		}
		enqueueErr = fmt.Errorf("%w; retry failed: %v", enqueueErr, retryErr)
	}

	dlqErr := fmt.Sprintf("dlq: %s", enqueueErr.Error())
	_ = w.failRun(ctx, run.UUID, dlqErr, "dlq", false, now)
	_ = w.taskRepo.UpdateFields(ctx, task.UUID, map[string]interface{}{"last_error": dlqErr})
	w.logger.WarnF(ctx, "[event_fabric.cron.dispatch] enqueue failed, moved to dlq task=%s err=%v", task.UUID.String(), enqueueErr)
}

func (w *EventFabricCronDispatcherWorker) failRun(ctx context.Context, runUUID uuid.UUID, message, category string, retryable bool, finishedAt time.Time) error {
	updates := map[string]interface{}{
		"status":            eventfabricmodel.ScheduledTaskRunStatusFailed,
		"finished_at":       finishedAt.UTC(),
		"error_message":     message,
		"failure_category":  category,
		"failure_retryable": retryable,
	}
	return w.taskRunRepo.UpdateFields(ctx, runUUID, updates)
}

func ptrTime(v time.Time) *time.Time {
	t := v.UTC()
	return &t
}
