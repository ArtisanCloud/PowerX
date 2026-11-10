package runtimescheduler

import (
	"context"
	"encoding/json"
	"errors"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type taskPublisherProbe struct {
	calls []TaskDispatch
	fail  bool
}

func (p *taskPublisherProbe) PublishScheduledTask(_ context.Context, run *models.SchedulerJobRun, d TaskDispatch) error {
	p.calls = append(p.calls, d)
	if p.fail {
		return errors.New("unavailable")
	}
	return nil
}
func TestTaskDispatchRetrySnapshotAndRunIdentity(t *testing.T) {
	now := time.Now().UTC()
	svc, ctx := newTestServiceWithClock(t, func() time.Time { return now }, nil)
	p := &taskPublisherProbe{fail: true}
	svc.taskPublisher = p
	payload := map[string]any{"business_action": "worker", "task_dispatch": map[string]any{"contract": TaskDispatchContract, "subscriber_id": "runtime.scheduler.jobs"}, "value": "original"}
	job, err := svc.CreateJob(ctx, JobSpec{OwnerType: models.OwnerTypePlugin, OwnerID: "com.powerx.plugins.ai-craft", Name: "worker", ScheduleType: "interval", ScheduleExpr: "1m", Payload: payload}, "test", "")
	require.NoError(t, err)
	first, err := svc.TriggerJob(ctx, job.UUID.String(), "test", "")
	require.NoError(t, err)
	require.Equal(t, models.RunStatusPendingDispatch, first.Run.Status)
	require.Equal(t, 1, first.Run.DispatchAttempts)
	// Editing the recurring job cannot change a persisted delivery retry.
	require.NoError(t, svc.db.Model(job).Update("payload_json", `{"business_action":"changed"}`).Error)
	p.fail = false
	now = now.Add(6 * time.Second)
	require.NoError(t, svc.dispatchPendingTasks(ctx, now, 10))
	require.Len(t, p.calls, 2)
	require.JSONEq(t, string(p.calls[0].Payload), string(p.calls[1].Payload))
	var got models.SchedulerJobRun
	require.NoError(t, svc.db.First(&got, "uuid = ?", first.Run.UUID).Error)
	require.Equal(t, models.RunStatusQueued, got.Status)
	require.NoError(t, svc.dispatchPendingTasks(ctx, now, 10))
	require.Len(t, p.calls, 2)
	raw, _ := json.Marshal(payload)
	require.NoError(t, svc.db.Model(job).Update("payload_json", string(raw)).Error)
	second, err := svc.TriggerJob(ctx, job.UUID.String(), "test", "")
	require.NoError(t, err)
	require.NotEqual(t, first.Run.EventID, second.Run.EventID)
	require.NotEqual(t, first.Run.UUID, second.Run.UUID)
}
func TestTaskDispatchDueCommitsBeforePublication(t *testing.T) {
	now := time.Now().UTC()
	svc, ctx := newTestServiceWithClock(t, func() time.Time { return now }, nil)
	p := &taskPublisherProbe{}
	svc.taskPublisher = p
	_, err := svc.CreateJob(ctx, JobSpec{OwnerType: models.OwnerTypePlugin, OwnerID: "com.powerx.plugins.ai-craft", Name: "worker", ScheduleType: "interval", ScheduleExpr: "1m", Payload: map[string]any{"business_action": "worker", "task_dispatch": map[string]any{"contract": TaskDispatchContract, "subscriber_id": "runtime.scheduler.jobs"}}}, "test", "")
	require.NoError(t, err)
	now = now.Add(2 * time.Minute)
	_, err = svc.DispatchDue(ctx, DispatchDueInput{Now: now, Limit: 10})
	require.NoError(t, err)
	require.Len(t, p.calls, 1)
}
func TestTaskDispatchPausedAndBoundedFailure(t *testing.T) {
	for _, pause := range []bool{true, false} {
		t.Run(map[bool]string{true: "paused", false: "bounded"}[pause], func(t *testing.T) {
			now := time.Now().UTC()
			svc, ctx := newTestServiceWithClock(t, func() time.Time { return now }, nil)
			p := &taskPublisherProbe{fail: true}
			svc.taskPublisher = p
			job, err := svc.CreateJob(ctx, JobSpec{OwnerType: models.OwnerTypePlugin, OwnerID: "com.powerx.plugins.ai-craft", Name: "worker", ScheduleType: "interval", ScheduleExpr: "1m", Payload: map[string]any{"business_action": "worker", "task_dispatch": map[string]any{"contract": TaskDispatchContract, "subscriber_id": "runtime.scheduler.jobs"}}}, "test", "")
			require.NoError(t, err)
			out, err := svc.TriggerJob(ctx, job.UUID.String(), "test", "")
			require.NoError(t, err)
			if pause {
				require.NoError(t, svc.db.Model(job).Update("status", models.JobStatusPaused).Error)
			}
			for i := 0; i < 6; i++ {
				now = now.Add(10 * time.Minute)
				require.NoError(t, svc.dispatchPendingTasks(ctx, now, 10))
			}
			var run models.SchedulerJobRun
			require.NoError(t, svc.db.First(&run, "uuid = ?", out.Run.UUID).Error)
			if pause {
				require.Equal(t, models.RunStatusSkipped, run.Status)
				require.Len(t, p.calls, 1)
			} else {
				require.Equal(t, models.RunStatusFailed, run.Status)
				require.Len(t, p.calls, 5)
			}
			require.Nil(t, run.NextDispatchAt)
		})
	}
}
func TestTaskDispatchRejectsInvalidDeclaration(t *testing.T) {
	for _, d := range []any{nil, map[string]any{"contract": TaskDispatchContract, "subscriber_id": "q", "payload": map[string]any{"owner_id": "foreign"}}, map[string]any{"contract": TaskDispatchContract, "subscriber_id": " q"}, map[string]any{"contract": "wrong", "subscriber_id": "q"}} {
		_, err := taskDispatchDeclaration(map[string]any{"business_action": "worker", "task_dispatch": d})
		require.Error(t, err)
	}
}
