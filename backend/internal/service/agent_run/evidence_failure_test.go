package agent_run

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestWorkerKeepsEvidenceFailureCodeAndStageInTaskAndEvent(t *testing.T) {
	for _, failure := range []evidence.ToolFailure{{Code: "agent.response_contract_invalid", Stage: "final_validation"}, {Code: "skill.tool_timeout", Stage: "tool_invoke", Retryable: true}} {
		t.Run(failure.Code, func(t *testing.T) {
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			store, err := NewRedisStore(client)
			require.NoError(t, err)
			queue, err := event_bus.NewRedisStreamTaskDriver(client, time.Second)
			require.NoError(t, err)
			ctx := context.Background()
			id := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: time.Now().Add(time.Minute)}
			if failure.Code == "skill.tool_timeout" {
				id.DeadlineAt = time.Now().Add(500 * time.Millisecond)
			}
			run, err := store.Create(ctx, id)
			require.NoError(t, err)
			run, err = store.InstallPlan(ctx, id, run.Version, Plan{Revision: 1, Tasks: []TaskDefinition{{TaskID: "review"}}})
			require.NoError(t, err)
			_, _, err = store.QueueTask(ctx, id, run.Version, 1, "review", "model_capacity")
			require.NoError(t, err)
			_, err = store.DispatchPending(ctx, id, queue, 10)
			require.NoError(t, err)
			deliveries, err := queue.Dequeue(ctx, taskQueueTenant(id), AgentTaskSubscriber, "workers", "worker-v5", 1, time.Millisecond)
			require.NoError(t, err)
			require.Len(t, deliveries, 1)
			err = store.ProcessDelivery(ctx, queue, deliveries[0], testExecutor(func(execCtx context.Context, _ TaskRef, _ string) (WorkResult, error) {
				if failure.Code == "skill.tool_timeout" {
					<-execCtx.Done()
				}
				return WorkResult{ReasonCode: "manual_review_required"}, &evidence.ToolEvidenceError{ToolFailure: failure}
			}))
			require.NoError(t, err)
			task, err := store.GetTask(ctx, id, 1, "review")
			require.NoError(t, err)
			require.Equal(t, "failed", task.Status)
			require.Equal(t, failure.Code, task.ReasonCode)
			require.Equal(t, failure.Stage, task.FailureStage)
			events, err := store.Events(ctx, id.TenantUUID, id.Env, id.RunID, 0, 100)
			require.NoError(t, err)
			found := false
			for _, event := range events {
				if event.Type == "agent_run.task_failed" {
					found = true
					require.Equal(t, task.ReasonCode, event.ReasonCode)
					require.Equal(t, task.FailureStage, event.FailureStage)
				}
			}
			require.True(t, found)
			final, err := store.Get(ctx, id.TenantUUID, id.Env, id.RunID)
			require.NoError(t, err)
			require.Equal(t, "failed", final.Status)
		})
	}
}
