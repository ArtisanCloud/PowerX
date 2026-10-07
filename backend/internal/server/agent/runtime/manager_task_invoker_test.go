package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestManagerTaskInvokerRejectsUnguardedCall(t *testing.T) {
	m := &ManagerTaskInvoker{Manager: agent.NewAgentManager()}
	_, err := m.InvokeTask(context.Background(), flowschema.ExecutionPlan{}, "task", nil, InvokePlanningInput{}, "agent:run:1:task")
	require.ErrorIs(t, err, agent_run.ErrInvalid)
}

func TestWorkerManagerRunsParallelAnalysisThenPersistedSummary(t *testing.T) {
	t.Run("verified_report", func(t *testing.T) { testWorkerManagerSummary(t, true) })
	t.Run("missing_report", func(t *testing.T) { testWorkerManagerSummary(t, false) })
}

func testWorkerManagerSummary(t *testing.T, withReport bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	addr := os.Getenv("POWERX_AGENT_RUN_TEST_REDIS_ADDR")
	if addr == "" {
		addr = miniredis.RunT(t).Addr()
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	if os.Getenv("POWERX_AGENT_RUN_TEST_REDIS_ADDR") != "" {
		require.NoError(t, agent_run.ValidateProductionRedis(ctx, client))
	}
	store, err := agent_run.NewRedisStore(client)
	require.NoError(t, err)
	queue, err := event_bus.NewRedisStreamTaskDriver(client, time.Minute)
	require.NoError(t, err)
	id := agent_run.Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: time.Now().Add(time.Minute)}
	_, err = store.Create(ctx, id)
	require.NoError(t, err)
	run, err := store.StartPlanning(ctx, id)
	require.NoError(t, err)
	_, run, err = store.LeaseTask(ctx, id, run.Version, 0, "plan", "planner", 1, time.Now().Add(time.Minute))
	require.NoError(t, err)
	_, run, err = store.StartTask(ctx, id, run.Version, 0, "plan", "planner", 1)
	require.NoError(t, err)
	full := &flowschema.ExecutionPlan{PlanID: "review", Tasks: []flowschema.PlanTask{
		{TaskID: "source", NodeKind: "skill", NodeRef: "skill.source", Stage: 1},
		{TaskID: "campaign", NodeKind: "skill", NodeRef: "skill.campaign", Stage: 1},
		{TaskID: "summary", NodeKind: "skill", NodeRef: "skill.summary", Stage: 2, DependsOn: []string{"source", "campaign"}, ParamRefs: map[string]string{
			"source": "{{task.source.output.result.content}}", "campaign": "{{task.campaign.output.result.content}}",
		}},
	}}
	objects := &planMemoryObjects{items: map[string][]byte{}}
	planRef := agent_run.TaskRef{TenantUUID: id.TenantUUID, Env: id.Env, RunID: id.RunID, TaskID: "plan", Attempt: 1}
	planKey, proof, err := SaveExecutionPlanArtifact(ctx, objects, planRef, full)
	require.NoError(t, err)
	schedule, err := TranslateInvokePlan(full, func(flowschema.PlanTask) (string, error) { return "analysis", nil })
	require.NoError(t, err)
	_, err = store.CompletePlanning(ctx, id, run.Version, "planner", 1, schedule, planKey, proof)
	require.NoError(t, err)
	_, err = store.ReconcilePlan(ctx, id)
	require.NoError(t, err)
	_, err = store.DispatchPending(ctx, id, queue, 100)
	require.NoError(t, err)
	manager := agent.NewAgentManager()
	var calls atomic.Int32
	started := make(chan string, 2)
	release := make(chan struct{})
	manager.SetSkillInvoker(func(callCtx context.Context, in agent.SkillInvokeInput) (*agent.SkillInvokeOutput, error) {
		calls.Add(1)
		if in.SkillID == "skill.summary" {
			if in.Payload["source"] != "skill.source" || in.Payload["campaign"] != "skill.campaign" {
				return nil, fmt.Errorf("missing persisted analysis results: %v", in.Payload)
			}

			if !withReport {
				return &agent.SkillInvokeOutput{Status: "completed", SkillID: in.SkillID, Result: map[string]any{"content": "unverified narrative"}}, nil
			}
			draft := evidence.Draft{Schema: evidence.DraftSchema, Kind: "analysis",
				Data:         []evidence.DraftDatum{{Key: "revenue", Label: "revenue", Unit: "万元", Scope: "campaign", Kind: "quantity", Source: evidence.Source{Pointer: "/message", Quote: "29万元", Literal: "29"}}},
				Calculations: []evidence.Calculation{}, Hypotheses: []string{}, Gaps: []string{}, Actions: []string{}}
			report, err := evidence.Compile(callCtx, draft, map[string]any{"message": "29万元"}, []string{"/message"}, id.TenantUUID, uuid.NewString(), id.TraceID)
			if err != nil {
				return nil, err
			}
			return &agent.SkillInvokeOutput{Status: "completed", SkillID: in.SkillID, Result: map[string]any{"response_envelope": report}}, nil
		} else {
			started <- in.SkillID
			select {
			case <-release:
			case <-callCtx.Done():
				return nil, callCtx.Err()
			}
		}
		return &agent.SkillInvokeOutput{Status: "completed", SkillID: in.SkillID, Result: map[string]any{"content": in.SkillID}}, nil
	})
	executor := &ServiceSessionTaskExecutor{Runs: store, Objects: objects, Invoker: &ManagerTaskInvoker{Manager: manager}, Loader: planningInputLoaderFunc(func(callCtx context.Context, _ agent_run.TaskRef) (InvokePlanningInput, error) {
		return InvokePlanningInput{Context: reqctx.WithEnv(reqctx.WithTenantUUID(callCtx, id.TenantUUID), id.Env), Message: "review"}, nil
	})}
	deliveries, err := queue.Dequeue(ctx, id.TenantUUID+":"+id.Env, agent_run.AgentTaskSubscriber, "workers", "worker", 10, time.Millisecond)
	require.NoError(t, err)
	var roots []event_bus.StreamTaskDelivery
	for _, d := range deliveries {
		if d.Message.ID == id.RunID+":0:plan:1" {
			require.NoError(t, queue.Ack(ctx, d))
			continue
		}
		roots = append(roots, d)
	}
	require.Len(t, roots, 2)
	finished := make(chan error, 2)
	for _, d := range roots {
		go func(d event_bus.StreamTaskDelivery) { finished <- store.ProcessDelivery(ctx, queue, d, executor) }(d)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("parallel roots did not both start")
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		require.NoError(t, <-finished)
	}
	deliveries, err = queue.Dequeue(ctx, id.TenantUUID+":"+id.Env, agent_run.AgentTaskSubscriber, "workers", "worker", 10, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	require.NoError(t, store.ProcessDelivery(ctx, queue, deliveries[0], executor))
	final, err := store.Get(ctx, id.TenantUUID, id.Env, id.RunID)
	require.NoError(t, err)
	if !withReport {
		require.NotEqual(t, "completed", final.Status)
		task, err := store.GetTask(ctx, id, 1, "summary")
		require.NoError(t, err)
		require.Equal(t, "failed", task.Status)
		require.Empty(t, task.ResultRef)
		require.EqualValues(t, 3, calls.Load())
		return
	}
	require.Equal(t, "completed", final.Status)
	require.EqualValues(t, 3, calls.Load())
	task, err := store.GetTask(ctx, id, 1, "summary")
	require.NoError(t, err)
	ref := planRef
	ref.Revision, ref.TaskID = 1, "summary"
	out, err := LoadTaskResultArtifact(ctx, objects, ref, task.ResultRef, task.EvidenceRef)
	require.NoError(t, err)
	require.Equal(t, true, out.Metadata["durable_response_verified"])
	reader := &DurableFinalOutputReader{Runs: store, Objects: objects}
	result, err := reader.ReadFinalOutput(ctx, final)
	require.NoError(t, err)
	require.Empty(t, result.Content)
	require.True(t, json.Valid(result.ResponseEnvelope))
	var report map[string]any
	require.NoError(t, json.Unmarshal(result.ResponseEnvelope, &report))
	require.Equal(t, evidence.ReportSchema, report["schema"])
	_, err = evidence.ValidateReport(report)
	require.NoError(t, err)
}
