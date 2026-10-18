package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	agenttrace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

// ManagerTaskInvoker 把持久化 Worker 接到正式 Manager；未取得执行权时拒绝调用。
type ManagerTaskInvoker struct{ Manager *agent.Manager }

var _ DurableTaskInvoker = (*ManagerTaskInvoker)(nil)

func (m *ManagerTaskInvoker) InvokeTask(ctx context.Context, plan flowschema.ExecutionPlan, taskID string,
	prior map[string]*aschema.ExecutionResult, input InvokePlanningInput, key string) (*aschema.ExecutionResult, error) {
	if m == nil || m.Manager == nil || key == "" || agent_run.GuardedExecutionKey(ctx) != key || reqctx.GetTenantUUID(ctx) == "" {
		return nil, agent_run.ErrInvalid
	}
	parts := strings.SplitN(key, ":", 4)
	if len(parts) != 4 || parts[0] != "agent" || parts[3] != taskID {
		return nil, agent_run.ErrInvalid
	}
	plan.Tasks = append([]flowschema.PlanTask(nil), plan.Tasks...)
	var selected flowschema.PlanTask
	for i, task := range plan.Tasks {
		if task.TaskID != taskID {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(task.NodeKind))
		if kind == "" {
			kind = "workflow"
			if strings.HasPrefix(task.FlowID, "skill.") {
				kind = "skill"
			}
		}
		switch kind {
		case "workflow", "skill", "tooling", "llm", "agent_handoff":
		default:
			return nil, agent_run.ErrInvalid
		}
		params := make(map[string]any, len(task.Params)+2)
		for k, v := range task.Params {
			params[k] = v
		}
		if kind == "workflow" || kind == "llm" {
			if _, exists := params["message"]; !exists {
				params["message"] = input.Message
			}
			params["config"] = input.Config
		}
		plan.Tasks[i].Params = params
		selected = plan.Tasks[i]
		break
	}
	if selected.TaskID == "" {
		return nil, agent_run.ErrInvalid
	}
	meta := aschema.ExecutionMeta{RequestID: key, TenantUUID: reqctx.GetTenantUUID(ctx), TraceID: reqctx.GetTraceID(ctx), Metadata: map[string]any{
		"run_id": parts[1], "plan_id": plan.PlanID, "env": reqctx.GetEnv(ctx), "agent_id": ctx.Value("agent_numeric_id"),
		"session_id": ctx.Value("session_id"), "message_id": ctx.Value("message_id"), "idempotency_key": key,
	}}
	ctx = agent.WithOriginalUserMessage(ctx, input.Message)
	ctx = evidence.EnsureLedger(ctx)
	out, err := m.Manager.ExecutePlanTask(ctx, plan, taskID, prior, meta)
	if err != nil {
		return nil, err
	}
	report := &agent.PlanExecutionReport{PlanID: plan.PlanID, FinalResult: out, Tasks: []agent.PlanTaskExecution{{TaskID: taskID, Status: agent.PlanTaskExecutionCompleted, Result: out}}}
	verdict, err := (DeterministicExecutionVerifier{}).Verify(ctx, report, out, nil)
	if err != nil {
		return nil, err
	}
	if verdict.Class != VerificationPass {
		return nil, fmt.Errorf("task verification rejected: %s", verdict.ReasonCode)
	}
	if snapshot, ok := ResourceSnapshotFromContext(ctx); ok {
		if err := AttachCapabilityVerificationEvidence(ctx, snapshot, selected, out); err != nil {
			return nil, err
		}
		if err := VerifyCapabilityContracts(snapshot, flowschema.ExecutionPlan{PlanID: plan.PlanID, Tasks: []flowschema.PlanTask{selected}}, report); err != nil {
			return nil, err
		}
	}
	// 仅在同次调用的可信计算账本中核验报告，后续 Worker 读取校验后的对象回执。
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	delete(out.Metadata, "durable_response_verified")
	envelope, err := responseEnvelopeFromExecutionResultWithTaskRefs(out.Data, finalResponseUpstreamTaskRefs(&plan))
	if err != nil {
		return nil, m.failFinalEvidence(ctx, parts[1], taskID, err)
	}
	final := finalDurableTask(plan)
	if envelope == nil && plan.PlanID != "invoke_default" && final.TaskID == taskID {
		return nil, m.failFinalEvidence(ctx, parts[1], taskID, fmt.Errorf("agent_run.final_response_required"))
	}
	if envelope != nil {
		if err := evidence.Verify(ctx, envelope); err != nil {
			return nil, m.failFinalEvidence(ctx, parts[1], taskID, err)
		}
		out.Metadata["durable_response_verified"] = true
	}
	return out, nil
}

func (m *ManagerTaskInvoker) failFinalEvidence(ctx context.Context, runID, taskID string, cause error) error {
	err := &evidence.ToolEvidenceError{ToolFailure: evidence.ToolFailure{Code: "agent.response_contract_invalid", Stage: "final_validation"}, Cause: cause}
	tr := &traceRuntime{logger: m.Manager.AgentTraceLogger(), meta: agenttrace.AgentRunMeta{RunID: runID, TraceID: reqctx.GetTraceID(ctx), TenantUUID: reqctx.GetTenantUUID(ctx), UserUUID: reqctx.GetUserUUID(ctx), AgentID: firstTraceString(contextString(ctx, "agent_numeric_id"), contextString(ctx, "agent_id")), SessionID: contextString(ctx, "session_id"), MessageID: contextString(ctx, "message_id")}, startedAt: time.Now().UTC(), starts: map[string]time.Time{}, nodeSeq: map[string]int{}}
	node := tr.startNode(ctx, "final_response", taskID+"/response_envelope_validation", map[string]any{"node_id": taskID + "_final_validation"})
	tr.failNode(context.WithoutCancel(ctx), node, "final_response", "response_envelope_validation", err)
	return err
}

// 与 Manager 的终态选择保持一致：最高阶段中最后一个任务负责最终回复。
func finalDurableTask(plan flowschema.ExecutionPlan) flowschema.PlanTask {
	var selected flowschema.PlanTask
	for i, task := range plan.Tasks {
		if i == 0 || task.Stage >= selected.Stage {
			selected = task
		}
	}
	return selected
}
