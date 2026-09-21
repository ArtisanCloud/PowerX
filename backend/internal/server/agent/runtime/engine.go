package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	agenttrace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	modelagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
)

type EventSink interface {
	Emit(event string, payload any) error
}

func approvedResumeFromContext(ctx context.Context) bool {
	approved, _ := ctx.Value("runtime_approved_resume").(bool)
	return approved
}

type Engine struct {
	mgr *agent.Manager
}

func NewEngine() *Engine { return &Engine{mgr: agent.GetAgentManager()} }

func (e *Engine) detectTasks(ctx context.Context, msg string, reqCfg *dto.ChatConfig) ([]flowschema.DetectedTask, error) {
	if task, ok := pendingDetectedTaskFromContext(ctx, msg); ok {
		return []flowschema.DetectedTask{task}, nil
	}
	if tasks, ok := e.deterministicMarketingReviewTasks(ctx, msg); ok {
		return tasks, nil
	}
	return e.mgr.DetectTasksWithToolCalling(ctx, msg, reqCfg)
}

func (e *Engine) Run(ctx context.Context, msg string, reqCfg *dto.ChatConfig, explicitFlow string, sink EventSink) error {
	ctx = context.WithValue(ctx, "team_user_message", strings.TrimSpace(msg))

	tr, traceErr := e.newTraceRuntime(ctx, msg, reqCfg, explicitFlow, "engine.stream")
	if traceErr != nil {
		emitAgentRunFailure(ctx, sink, explicitFlow, "trace.init_error", "Agent Trace 初始化失败", traceErr, "")
		return traceErr
	}
	if rs, ok := sink.(*RunStateSink); ok {
		rs.SetRunStateRecorder(func(event string, payload any) {
			tr.appendRunStateEvent(ctx, event, payload)
		})
	}
	runStatus := agenttrace.RunStatusCompleted
	var runErr error
	var finalText string
	var streamBuffer strings.Builder
	defer func() {
		if runErr != nil {
			runStatus = agenttrace.RunStatusFailed
		}
		tr.complete(ctx, runStatus, finalText, runErr)
	}()
	receiveNode := tr.startNode(ctx, "receive_message", "agent.stream", map[string]any{"message_digest": digestString(msg)})
	tr.endNode(ctx, receiveNode, "receive_message", "agent.stream", map[string]any{"accepted": true})
	if snapshot, ok := ResourceSnapshotFromContext(ctx); ok {
		resourceNode := tr.startNode(ctx, "resource_snapshot", "runtime.prepare", map[string]any{"snapshot_uuid": snapshot.SnapshotUUID.String(), "resource_count": len(snapshot.Resources)})
		tr.endNode(ctx, resourceNode, "resource_snapshot", "runtime.prepare", map[string]any{"snapshot_uuid": snapshot.SnapshotUUID.String(), "resource_count": len(snapshot.Resources)})
	}
	responsePlan := responsePlanFromContext(ctx)
	if responsePlan != nil {
		responsePlan.TraceID = tr.meta.TraceID
		responsePlan.RunID = tr.meta.RunID
		responsePlan.SessionID = tr.meta.SessionID
		responsePlan.MessageID = tr.meta.MessageID
		rpNode := tr.startNode(ctx, "response_planner", string(responsePlan.ResponseMode), responsePlan.ToDebugEvent())
		tr.endNode(ctx, rpNode, "response_planner", string(responsePlan.ResponseMode), responsePlan.ToDebugEvent())
		_ = sink.Emit("response_plan", responsePlan.ToDebugEvent())
	}

	teamPlan, teamPlanHandled, teamPlanErr := builtInTeamPlanFromContext(ctx)
	if teamPlanErr != nil {
		runErr = teamPlanErr
		emitAgentRunFailure(ctx, sink, explicitFlow, "planner.team_plan_error", "团队执行计划生成失败", teamPlanErr, "")
		return teamPlanErr
	}
	var (
		tasks []flowschema.DetectedTask
		err   error
	)
	if !teamPlanHandled {
		intentNode := tr.startNode(ctx, "intent_recognition", "DetectTasksWithToolCalling", nil)
		// 非团队会话才由通用意图规划器判断任务；已声明团队图不得被它覆盖。
		tasks, err = e.detectTasks(ctx, msg, reqCfg)
		if err != nil {
			tr.failNode(ctx, intentNode, "intent_recognition", "DetectTasksWithToolCalling", err)
			runErr = err
			emitAgentRunFailure(ctx, sink, explicitFlow, "intent.detect_error", "意图识别失败", err, "")
			return err
		}
		tr.endNode(ctx, intentNode, "intent_recognition", "DetectTasksWithToolCalling", map[string]any{"task_count": len(tasks)})
		_ = sink.Emit(dto.EventIntent, map[string]any{"mode": "intent_multi", "planner_mode": dto.PlannerModeUnified, "tasks": tasks})
	} else {
		_ = sink.Emit(dto.EventIntent, map[string]any{"mode": "team_orchestration", "planner_mode": "persisted_declaration", "tasks": teamPlan.Tasks})
	}

	plannerNode := tr.startNode(ctx, "planner", "BuildPlan", map[string]any{"task_count": len(tasks), "team_orchestration": teamPlanHandled})
	// 1) 团队图由持久化声明编译；其他会话才使用通用 BuildPlan。
	var rawPlan any
	var plan *flowschema.ExecutionPlan
	ok := false
	if teamPlanHandled {
		rawPlan = teamPlan
		plan = teamPlan
		ok = true
	} else {
		rawPlan = e.mgr.BuildPlan(tasks)
		plan, ok = NormalizeExecPlan(rawPlan)
	}
	if ok && plan != nil {
		tr.withPlan(plan.PlanID)
		plan = e.applyRuntimeParamState(ctx, plan)
		markResponsePlanExecutable(responsePlan, plan)
	}
	tr.endNode(ctx, plannerNode, "planner", "BuildPlan", map[string]any{"has_plan": ok, "plan_id": tr.meta.PlanID})
	_ = sink.Emit(dto.EventPlan, map[string]any{
		"planner_mode": dto.PlannerModeUnified,
		"plan":         PlanOrRaw(plan, rawPlan),
	})
	if responsePlanRequiresExecution(responsePlan) && (!ok || plan == nil || len(plan.Tasks) == 0) {
		err := fmt.Errorf("agent execution target selected but no executable task was produced: target_capability_ids=%s", strings.Join(responseTargetIDs(responsePlan), ","))
		tr.failNode(ctx, plannerNode, "planner", "BuildPlan", err)
		runErr = err
		emitAgentRunFailure(ctx, sink, explicitFlow, "planner.no_executable_task", "执行计划生成失败", err, "")
		return err
	}
	// skill/tooling 等非 workflow 节点必须走统一 Plan 执行链路，
	// 不能再把 node_ref 当 flow_id 交给 ag.Stream。
	if planHasNonWorkflow(plan) {
		if shouldExecutePlanForResponse(responsePlan, plan) {
			out, err := e.runResolvedPlan(ctx, plan, sink, tr)
			if err == nil {
				finalText = finalTraceContent(out)
			}
			runErr = err
			return err
		}
		ok = false
		plan = nil
	}

	// 先根据 plan 拿一个候选 flowID（按 stage 最小、原序）
	var flowID string
	if ok && len(plan.Tasks) > 0 {
		flowID = plan.Tasks[0].FlowID
	}

	// 若调用方显式传了 flow，优先生效；否则再按“第一条任务”兜底
	flowID = explicitFlow // FIX: 原来用 := 重新定义，导致覆盖问题
	if flowID == "" {
		flowID = PickFirstFlowID(plan)
	}

	// 3) 路由 & 执行
	ag, fallbackFlowID, err := e.mgr.GetDefaultRoute()
	if err != nil {
		runErr = err
		emitAgentRunFailure(ctx, sink, flowID, "route.default_error", "创建 Agent 失败", err, "")
		return err
	}
	if flowID == "" {
		flowID = fallbackFlowID
	}
	if len(tasks) == 0 && strings.TrimSpace(fallbackFlowID) != "" && flowID == fallbackFlowID {
		reqCfg = applyBaseFlowDirectGuardrail(reqCfg)
	}
	llmMessage := BuildModeSpecificUserPrompt(msg, responsePlan)

	execID := fmt.Sprintf("exec_%d", time.Now().UnixNano())
	_ = sink.Emit(dto.EventStart, map[string]any{"flow_id": flowID, "execution_id": execID})

	streamNode := tr.startNode(ctx, "llm_call", flowID, map[string]any{"execution_id": execID, "flow_id": flowID})
	meta := tr.applyExecutionMeta(agentschema.ExecutionMeta{
		RequestID:  execID,
		UserID:     reqctx.GetUserID(ctx),
		TenantUUID: strings.TrimSpace(reqctx.GetTenantUUID(ctx)),
		TraceID:    strings.TrimSpace(reqctx.GetTraceID(ctx)),
		Metadata: map[string]any{
			"transport": "engine",
			"env":       strings.TrimSpace(reqctx.GetEnv(ctx)),
		},
	})
	sr, err := ag.Stream(ctx, flowID, flowschema.Context{
		"message": llmMessage,
		"config":  reqCfg,
	}, meta)
	if err != nil {
		tr.failNode(ctx, streamNode, "llm_call", flowID, err)
		runErr = err
		emitAgentRunFailure(ctx, sink, flowID, "stream.start_error", "流式聊天执行失败", err, "")
		return err
	}

	// 4) 转发流事件
	defer sr.Close()
	type recvItem struct {
		ch  *agentschema.ExecutionResult
		err error
	}
	recvCh := make(chan recvItem, 1)

	go func() {
		defer close(recvCh)
		for {
			ch, err := sr.Recv()
			recvCh <- recvItem{ch: ch, err: err}
			if err != nil {
				return
			}
		}
	}()

	hb := time.NewTicker(15 * time.Second)
	defer hb.Stop()
	lastRecvAt := time.Now()
	emitBufferedFinal := func(reason string) bool {
		content := SanitizeAssistantVisibleText(streamBuffer.String())
		if strings.TrimSpace(content) == "" {
			return false
		}
		finalText = BuildFinalResponseContent(responsePlan, content, nil)
		contextLayers := responseContextLayersFromContext(ctx)
		cbNode := tr.startNode(ctx, "context_builder", responseModeString(responsePlan), map[string]any{
			"response_mode":       responseModeString(responsePlan),
			"used_context_layers": contextLayers,
			"model_selection":     modelSelectionFromContext(ctx, ModelPolicyNodeContextBuilder),
			"finalize_reason":     reason,
		})
		tr.endNode(ctx, cbNode, "context_builder", responseModeString(responsePlan), map[string]any{"used_context_layers": contextLayers})
		finalNode := tr.startNode(ctx, "final_response", reason, map[string]any{
			"response_mode":         responseModeString(responsePlan),
			"target_capability_ids": responseTargetIDs(responsePlan),
			"model_selection":       modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse),
		})
		_ = sink.Emit(dto.EventFinal, map[string]any{
			"success": true,
			"data": map[string]any{
				"content": finalText,
			},
			"metadata": mergeResponseMetadata(mergeTraceMetadata(map[string]any{
				"trace_id":              strings.TrimSpace(reqctx.GetTraceID(ctx)),
				"finalized_from_buffer": true,
				"finalize_reason":       reason,
			}, tr), responsePlan, contextLayers, modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse)),
		})
		tr.endNode(ctx, finalNode, "final_response", reason, map[string]any{
			"content_digest":        digestString(finalText),
			"response_mode":         responseModeString(responsePlan),
			"target_capability_ids": responseTargetIDs(responsePlan),
			"used_context_layers":   contextLayers,
		})
		return true
	}
	failMissingFinal := func(reason string) error {
		err := fmt.Errorf("agent stream ended without final response: flow_id=%s reason=%s", flowID, reason)
		tr.failNode(ctx, streamNode, "llm_call", flowID, err)
		runErr = err
		payload := agentRunFailurePayload(ctx, flowID, "stream.missing_final", err, streamBuffer.String())
		payload["message"] = "Agent Run State 协议错误：运行已结束但未收到 final。"
		_ = sink.Emit(dto.EventError, payload)
		_ = sink.Emit(dto.EventEnd, agentRunEndFailurePayload(payload))
		return err
	}

	for {
		select {
		case <-ctx.Done():
			tr.failNode(ctx, streamNode, "llm_call", flowID, ctx.Err())
			runErr = ctx.Err()
			payload := agentRunFailurePayload(ctx, flowID, "context.done", ctx.Err(), streamBuffer.String())
			payload["message"] = "请求超时或已取消"
			_ = sink.Emit(dto.EventError, payload)
			_ = sink.Emit(dto.EventEnd, agentRunEndFailurePayload(payload))
			return ctx.Err()
		case <-hb.C:
			// 心跳：让前端/网关确认连接仍然活着（前端可选择忽略，仅用于避免“看似挂死”）。
			_ = sink.Emit(dto.EventHeartbeat, map[string]any{
				"ts":           time.Now().UTC().Unix(),
				"idle_seconds": int(time.Since(lastRecvAt).Seconds()),
			})
		case it, ok := <-recvCh:
			if !ok {
				if emitBufferedFinal("stream.eof") {
					tr.endNode(ctx, streamNode, "llm_call", flowID, map[string]any{"eof": true, "finalized_from_buffer": true})
					_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
					return nil
				}
				return failMissingFinal("stream.closed")
			}
			if it.err != nil {
				if errors.Is(it.err, io.EOF) {
					if emitBufferedFinal("stream.eof") {
						tr.endNode(ctx, streamNode, "llm_call", flowID, map[string]any{"eof": true, "finalized_from_buffer": true})
						_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
						return nil
					}
					return failMissingFinal("io.eof")
				}
				tr.failNode(ctx, streamNode, "llm_call", flowID, it.err)
				runErr = it.err
				payload := agentRunFailurePayload(ctx, flowID, "stream.recv_error", it.err, streamBuffer.String())
				_ = sink.Emit(dto.EventError, payload)
				_ = sink.Emit(dto.EventEnd, agentRunEndFailurePayload(payload))
				return it.err
			}
			lastRecvAt = time.Now()
			ch := it.ch
			if ch == nil {
				continue
			}

			if delta, ok := ch.Metadata["delta_text"].(string); ok && delta != "" {
				streamBuffer.WriteString(delta)
				_ = sink.Emit(dto.EventToken, map[string]any{"delta": delta, "step_id": ch.StepID, "timestamp": ch.Timestamp})
				continue
			}

			_ = sink.Emit(dto.EventData, map[string]any{
				"success":   ch.Success,
				"data":      sanitizeExecutionData(ch.Data),
				"step_id":   ch.StepID,
				"timestamp": ch.Timestamp,
				"metadata":  ch.Metadata,
			})

			if isFinal, _ := ch.Metadata["is_final"].(bool); isFinal {
				finalText = BuildFinalResponseContent(responsePlan, buildFinalContent(ch), nil)
				contextLayers := responseContextLayersFromContext(ctx)
				cbNode := tr.startNode(ctx, "context_builder", responseModeString(responsePlan), map[string]any{
					"response_mode":       responseModeString(responsePlan),
					"used_context_layers": contextLayers,
					"model_selection":     modelSelectionFromContext(ctx, ModelPolicyNodeContextBuilder),
				})
				tr.endNode(ctx, cbNode, "context_builder", responseModeString(responsePlan), map[string]any{"used_context_layers": contextLayers})
				finalNode := tr.startNode(ctx, "final_response", "stream.final", map[string]any{
					"step_id":               ch.StepID,
					"response_mode":         responseModeString(responsePlan),
					"target_capability_ids": responseTargetIDs(responsePlan),
					"model_selection":       modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse),
				})
				_ = sink.Emit(dto.EventFinal, map[string]any{
					"success":   ch.Success,
					"data":      mergeFinalContent(sanitizeExecutionData(ch.Data), finalText),
					"metadata":  mergeResponseMetadata(mergeTraceMetadata(ch.Metadata, tr), responsePlan, contextLayers, modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse)),
					"timestamp": ch.Timestamp,
				})
				tr.endNode(ctx, finalNode, "final_response", "stream.final", map[string]any{
					"success":               ch.Success,
					"content_digest":        digestString(finalText),
					"response_mode":         responseModeString(responsePlan),
					"target_capability_ids": responseTargetIDs(responsePlan),
					"used_context_layers":   contextLayers,
				})
				tr.endNode(ctx, streamNode, "llm_call", flowID, map[string]any{"final": true})
				_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
				return nil
			}
		}
	}
}

// RunApprovedResume executes only the caller-provided remaining subplan under
// a server-restored frozen runtime context. The handler must set
// runtime_approved_resume after resolving a persisted approved decision; this
// entrypoint deliberately has no planner or user-message input.
func (e *Engine) RunApprovedResume(ctx context.Context, plan flowschema.ExecutionPlan, sink EventSink) error {
	if !approvedResumeFromContext(ctx) {
		return fmt.Errorf("approved runtime resume context is required")
	}
	tr, err := e.newTraceRuntime(ctx, "", nil, "", "engine.approved_resume")
	if err != nil {
		return err
	}
	var runErr error
	var finalText string
	defer func() {
		status := agenttrace.RunStatusCompleted
		if runErr != nil {
			status = agenttrace.RunStatusFailed
		}
		tr.complete(ctx, status, finalText, runErr)
	}()
	out, err := e.runResolvedPlan(ctx, &plan, sink, tr)
	if err == nil {
		finalText = finalTraceContent(out)
	}
	runErr = err
	return err
}

func (e *Engine) RunPlanInvoke(ctx context.Context, msg string, reqCfg *dto.ChatConfig, explicitFlow string, sink EventSink) (*agentschema.ExecutionResult, *flowschema.ExecutionPlan, error) {
	ctx = context.WithValue(ctx, "team_user_message", strings.TrimSpace(msg))

	tr, traceErr := e.newTraceRuntime(ctx, msg, reqCfg, explicitFlow, "engine.invoke")
	if traceErr != nil {
		emitAgentRunFailure(ctx, sink, explicitFlow, "trace.init_error", "Agent Trace 初始化失败", traceErr, "")
		return nil, nil, traceErr
	}
	if rs, ok := sink.(*RunStateSink); ok {
		rs.SetRunStateRecorder(func(event string, payload any) {
			tr.appendRunStateEvent(ctx, event, payload)
		})
	}
	var runErr error
	var finalText string
	defer func() {
		status := agenttrace.RunStatusCompleted
		if runErr != nil {
			status = agenttrace.RunStatusFailed
		}
		tr.complete(ctx, status, finalText, runErr)
	}()
	receiveNode := tr.startNode(ctx, "receive_message", "agent.invoke", map[string]any{"message_digest": digestString(msg)})
	tr.endNode(ctx, receiveNode, "receive_message", "agent.invoke", map[string]any{"accepted": true})
	responsePlan := responsePlanFromContext(ctx)
	if responsePlan != nil {
		responsePlan.TraceID = tr.meta.TraceID
		responsePlan.RunID = tr.meta.RunID
		responsePlan.SessionID = tr.meta.SessionID
		responsePlan.MessageID = tr.meta.MessageID
		rpNode := tr.startNode(ctx, "response_planner", string(responsePlan.ResponseMode), responsePlan.ToDebugEvent())
		tr.endNode(ctx, rpNode, "response_planner", string(responsePlan.ResponseMode), responsePlan.ToDebugEvent())
		_ = sink.Emit("response_plan", responsePlan.ToDebugEvent())
	}

	teamPlan, teamPlanHandled, teamPlanErr := builtInTeamPlanFromContext(ctx)
	if teamPlanErr != nil {
		runErr = teamPlanErr
		emitAgentRunFailure(ctx, sink, explicitFlow, "planner.team_plan_error", "团队执行计划生成失败", teamPlanErr, "")
		return nil, nil, teamPlanErr
	}
	var (
		tasks []flowschema.DetectedTask
		err   error
	)
	if !teamPlanHandled {
		intentNode := tr.startNode(ctx, "intent_recognition", "DetectTasksWithToolCalling", nil)
		tasks, err = e.detectTasks(ctx, msg, reqCfg)
		if err != nil {
			tr.failNode(ctx, intentNode, "intent_recognition", "DetectTasksWithToolCalling", err)
			runErr = err
			emitAgentRunFailure(ctx, sink, explicitFlow, "intent.detect_error", "意图识别失败", err, "")
			return nil, nil, err
		}
		tr.endNode(ctx, intentNode, "intent_recognition", "DetectTasksWithToolCalling", map[string]any{"task_count": len(tasks)})
		_ = sink.Emit(dto.EventIntent, map[string]any{"mode": "intent_multi", "planner_mode": dto.PlannerModeUnified, "tasks": tasks})
	} else {
		_ = sink.Emit(dto.EventIntent, map[string]any{"mode": "team_orchestration", "planner_mode": "persisted_declaration", "tasks": teamPlan.Tasks})
	}

	plannerNode := tr.startNode(ctx, "planner", "BuildPlan", map[string]any{"task_count": len(tasks), "team_orchestration": teamPlanHandled})
	var rawPlan any
	var plan *flowschema.ExecutionPlan
	ok := false
	if teamPlanHandled {
		rawPlan = teamPlan
		plan = teamPlan
		ok = true
	} else {
		rawPlan = e.mgr.BuildPlan(tasks)
		plan, ok = NormalizeExecPlan(rawPlan)
	}
	if ok && plan != nil {
		tr.withPlan(plan.PlanID)
		plan = e.applyRuntimeParamState(ctx, plan)
		markResponsePlanExecutable(responsePlan, plan)
	}
	tr.endNode(ctx, plannerNode, "planner", "BuildPlan", map[string]any{"has_plan": ok, "plan_id": tr.meta.PlanID})
	_ = sink.Emit(dto.EventPlan, map[string]any{
		"planner_mode": dto.PlannerModeUnified,
		"plan":         PlanOrRaw(plan, rawPlan),
	})
	if responsePlanRequiresExecution(responsePlan) && (!ok || plan == nil || len(plan.Tasks) == 0) {
		err := fmt.Errorf("agent execution target selected but no executable task was produced: target_capability_ids=%s", strings.Join(responseTargetIDs(responsePlan), ","))
		tr.failNode(ctx, plannerNode, "planner", "BuildPlan", err)
		runErr = err
		emitAgentRunFailure(ctx, sink, explicitFlow, "planner.no_executable_task", "执行计划生成失败", err, "")
		return nil, nil, err
	}

	if explicitFlow = strings.TrimSpace(explicitFlow); explicitFlow != "" {
		plan = &flowschema.ExecutionPlan{
			PlanID: fmt.Sprintf("plan_%d", time.Now().UnixNano()),
			Tasks: []flowschema.PlanTask{
				{
					TaskID: fmt.Sprintf("task_%d", time.Now().UnixNano()),
					FlowID: explicitFlow,
					Stage:  1,
				},
			},
		}
		ok = true
		tr.withPlan(plan.PlanID)
		_ = sink.Emit(dto.EventPlan, map[string]any{
			"planner_mode": dto.PlannerModeUnified,
			"plan":         plan,
		})
	}

	dispatchMeta := tr.applyExecutionMeta(agentschema.ExecutionMeta{
		RequestID:  fmt.Sprintf("req_%d", time.Now().UnixNano()),
		UserID:     reqctx.GetUserID(ctx),
		TenantUUID: strings.TrimSpace(reqctx.GetTenantUUID(ctx)),
		TraceID:    strings.TrimSpace(reqctx.GetTraceID(ctx)),
		Metadata: map[string]any{
			"transport": "engine.invoke",
			"env":       strings.TrimSpace(reqctx.GetEnv(ctx)),
		},
	})
	if dispatchMeta.TraceID == "" {
		dispatchMeta.TraceID = fmt.Sprintf("trace_%d", time.Now().UnixNano())
	}
	if len(tasks) == 0 {
		reqCfg = applyBaseFlowDirectGuardrail(reqCfg)
	}

	if !shouldExecutePlanForResponse(responsePlan, plan) {
		ok = false
		plan = nil
	}

	if !ok || plan == nil || len(plan.Tasks) == 0 {
		dispatchNode := tr.startNode(ctx, "llm_call", "Dispatch", map[string]any{"request_id": dispatchMeta.RequestID})
		out, _, dispatchErr := e.mgr.Dispatch(ctx, msg, flowschema.Context{
			"message": msg,
			"config":  reqCfg,
		}, dispatchMeta)
		if dispatchErr != nil {
			tr.failNode(ctx, dispatchNode, "llm_call", "Dispatch", dispatchErr)
			runErr = dispatchErr
			emitAgentRunFailure(ctx, sink, "Dispatch", "dispatch.error", "执行失败", dispatchErr, "")
			return nil, nil, dispatchErr
		}
		content := BuildFinalResponseContent(responsePlan, buildFinalContent(out), nil)
		finalText = content
		tr.endNode(ctx, dispatchNode, "llm_call", "Dispatch", map[string]any{"success": out != nil && out.Success, "content_digest": digestString(content)})
		traceID := fmt.Sprintf("trace_%d", time.Now().UnixNano())
		if raw := strings.TrimSpace(reqctx.GetTraceID(ctx)); raw != "" {
			traceID = raw
		}
		contextLayers := responseContextLayersFromContext(ctx)
		cbNode := tr.startNode(ctx, "context_builder", responseModeString(responsePlan), map[string]any{
			"response_mode":       responseModeString(responsePlan),
			"used_context_layers": contextLayers,
			"model_selection":     modelSelectionFromContext(ctx, ModelPolicyNodeContextBuilder),
		})
		tr.endNode(ctx, cbNode, "context_builder", responseModeString(responsePlan), map[string]any{"used_context_layers": contextLayers})
		finalNode := tr.startNode(ctx, "final_response", "invoke.final", map[string]any{
			"response_mode":         responseModeString(responsePlan),
			"target_capability_ids": responseTargetIDs(responsePlan),
			"model_selection":       modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse),
		})
		_ = sink.Emit(dto.EventFinal, map[string]any{
			"success": true,
			"data": map[string]any{
				"content": content,
			},
			"metadata": mergeResponseMetadata(mergeTraceMetadata(map[string]any{
				"trace_id": traceID,
				"plan_id":  "",
			}, tr), responsePlan, contextLayers, modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse)),
		})
		tr.endNode(ctx, finalNode, "final_response", "invoke.final", map[string]any{
			"content_digest":        digestString(content),
			"response_mode":         responseModeString(responsePlan),
			"target_capability_ids": responseTargetIDs(responsePlan),
			"used_context_layers":   contextLayers,
		})
		_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
		return out, nil, nil
	}
	out, execErr := e.runResolvedPlan(ctx, plan, sink, tr)
	if execErr != nil {
		runErr = execErr
		return nil, plan, execErr
	}
	finalText = finalTraceContent(out)
	return out, plan, nil
}

// finalResponseUpstreamTaskRefs returns only tasks transitively upstream of a
// terminal task. Final results cannot cite their own task or model-invented
// labels as provenance.
func finalResponseUpstreamTaskRefs(plan *flowschema.ExecutionPlan) map[string]struct{} {
	if plan == nil || len(plan.Tasks) == 0 {
		return map[string]struct{}{}
	}
	tasks := make(map[string]flowschema.PlanTask, len(plan.Tasks))
	dependedOn := make(map[string]struct{}, len(plan.Tasks))
	for _, task := range plan.Tasks {
		taskID := strings.TrimSpace(task.TaskID)
		if taskID == "" {
			continue
		}
		tasks[taskID] = task
		for _, dependency := range task.DependsOn {
			if dependency = strings.TrimSpace(dependency); dependency != "" {
				dependedOn[dependency] = struct{}{}
			}
		}
	}
	refs := make(map[string]struct{}, len(tasks))
	var collect func(string)
	collect = func(taskID string) {
		task, ok := tasks[taskID]
		if !ok {
			return
		}
		for _, dependency := range task.DependsOn {
			dependency = strings.TrimSpace(dependency)
			if dependency == "" {
				continue
			}
			if _, exists := refs[dependency]; exists {
				continue
			}
			refs[dependency] = struct{}{}
			collect(dependency)
		}
	}
	for taskID := range tasks {
		if _, isUpstream := dependedOn[taskID]; !isUpstream {
			collect(taskID)
		}
	}
	return refs
}

// finalTraceContent is canonical trace-only data. It is never emitted as a
// second model-authored response, but gives response-envelope runs a durable
// final digest in their trace report.
func finalTraceContent(out *agentschema.ExecutionResult) string {
	if out == nil || out.Data == nil {
		return ""
	}
	envelope, err := responseEnvelopeFromExecutionResult(out.Data)
	if err != nil || envelope == nil {
		return buildFinalContent(out)
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return ""
	}
	return string(data)
}

func sanitizeExecutionData(in flowschema.Result) flowschema.Result {
	if in == nil {
		return nil
	}
	out := make(flowschema.Result, len(in))
	for k, v := range in {
		switch vv := v.(type) {
		case string:
			if strings.EqualFold(k, "content") {
				out[k] = SanitizeAssistantVisibleText(vv)
				continue
			}
			out[k] = vv
		case map[string]any:
			if strings.EqualFold(k, "result") {
				out[k] = sanitizeExecutionData(vv)
				continue
			}
			out[k] = vv
		default:
			out[k] = v
		}
	}
	return out
}

func (e *Engine) runResolvedPlan(ctx context.Context, plan *flowschema.ExecutionPlan, sink EventSink, tr *traceRuntime) (*agentschema.ExecutionResult, error) {
	ctx = evidence.WithLedger(ctx)
	if plan == nil || len(plan.Tasks) == 0 {
		return nil, fmt.Errorf("empty plan")
	}
	var (
		controller      *PlanController
		snapshot        *ResourceSnapshot
		revisionService *PlanRevisionService
		verificationSvc *VerificationEvidenceService
		runUUID         uuid.UUID
		runtimeEnv      string
	)
	approvedResume := approvedResumeFromContext(ctx)
	if resourceSnapshot, ok := ResourceSnapshotFromContext(ctx); ok {
		snapshot = resourceSnapshot
		var err error
		controller, err = NewPlanController(snapshot, RuntimeBudget{MaxPlanRevisions: 2, MaxObservations: 4, MaxSteps: 16, MaxCapabilityCalls: 8, MaxConcurrentTasks: 4, MaxExecutionDuration: 2 * time.Minute}, *plan)
		if err != nil {
			return nil, fmt.Errorf("initialize plan controller: %w", err)
		}
		revision := controller.Current()
		var hasRevisionService bool
		revisionService, hasRevisionService = PlanRevisionServiceFromContext(ctx)
		if !hasRevisionService {
			return nil, fmt.Errorf("plan revision service is not configured")
		}
		var hasVerificationService bool
		verificationSvc, hasVerificationService = VerificationEvidenceServiceFromContext(ctx)
		if !hasVerificationService {
			return nil, fmt.Errorf("verification evidence service is not configured")
		}
		var runUUIDErr error
		runUUID, runUUIDErr = uuid.Parse(contextString(ctx, "runtime_run_uuid"))
		if runUUIDErr != nil {
			return nil, fmt.Errorf("runtime_run_uuid is required for plan revision")
		}
		runtimeEnv = firstNonEmpty(strings.TrimSpace(reqctx.GetEnv(ctx)), contextString(ctx, "env"))
		if approvedResume {
			persistedRevisionUUID, parseErr := uuid.Parse(contextString(ctx, "runtime_plan_revision_uuid"))
			if parseErr != nil || persistedRevisionUUID == uuid.Nil {
				return nil, fmt.Errorf("approved runtime resume revision is required")
			}
			for _, task := range plan.Tasks {
				if normalizeNodeKind(task.NodeKind) == dto.NodeKindObservation {
					return nil, fmt.Errorf("approved runtime resume cannot execute observation tasks")
				}
			}
		} else {
			if err := revisionService.Persist(ctx, runtimeEnv, snapshot.TenantUUID, runUUID, revision, controller.budget, nil); err != nil {
				return nil, fmt.Errorf("persist initial plan revision: %w", err)
			}
			ctx = context.WithValue(ctx, "runtime_plan_revision_uuid", revision.RevisionUUID.String())
			tr.withRuntimeRevision(revision.RevisionUUID)
			controlNode := tr.startNode(ctx, "plan_revision", "runtime.initial_plan", map[string]any{
				"revision_uuid": revision.RevisionUUID.String(),
				"snapshot_uuid": snapshot.SnapshotUUID.String(),
				"reason_code":   revision.ReasonCode,
				"plan_id":       plan.PlanID,
			})
			tr.endNode(ctx, controlNode, "plan_revision", "runtime.initial_plan", map[string]any{
				"revision_uuid": revision.RevisionUUID.String(),
				"snapshot_uuid": snapshot.SnapshotUUID.String(),
				"reason_code":   revision.ReasonCode,
			})
		}
		executable := make([]flowschema.PlanTask, 0, len(plan.Tasks))
		for _, task := range plan.Tasks {
			if normalizeNodeKind(task.NodeKind) != dto.NodeKindObservation {
				executable = append(executable, task)
				continue
			}
			resourceUUID, parseErr := uuid.Parse(strings.TrimSpace(normalizeNodeRef(task)))
			purpose := strings.TrimSpace(anyToString(task.Params["purpose"]))
			if parseErr != nil || purpose == "" {
				return nil, fmt.Errorf("observation task %s requires node_ref resource_uuid and params.purpose", task.TaskID)
			}
			node := tr.startNode(ctx, "observation", resourceUUID.String(), map[string]any{"task_id": task.TaskID, "purpose": purpose})
			observation, observeErr := controller.Observe(ctx, firstNonEmpty(strings.TrimSpace(reqctx.GetEnv(ctx)), contextString(ctx, "env")), resourceUUID, purpose)
			if observeErr != nil {
				tr.failNode(ctx, node, "observation", resourceUUID.String(), observeErr)
				return nil, observeErr
			}
			tr.endNode(ctx, node, "observation", resourceUUID.String(), map[string]any{"observation_uuid": observation.ObservationUUID.String(), "resource_uuid": resourceUUID.String()})
		}
		if len(executable) == 0 {
			return nil, fmt.Errorf("observation plan requires a subsequent executable task")
		}
		if approvedResume {
			// The remaining plan is already a persisted revision. Observation would
			// create an unapproved replacement decision, so it is forbidden above.
			plan = &flowschema.ExecutionPlan{PlanID: plan.PlanID, Tasks: executable}
		} else {
			ctx = ContextWithRuntimeObservations(ctx, controller.Observations())
			planner, hasPlanner := PlanRevisionPlannerFromContext(ctx)
			if !hasPlanner {
				return nil, fmt.Errorf("plan revision planner is not configured")
			}
			reasonCode, nextPlan, replanErr := planner.Replan(ctx, PlanRevisionRequest{Snapshot: snapshot, Observations: controller.Observations(), CurrentPlan: flowschema.ExecutionPlan{PlanID: plan.PlanID, Tasks: executable}})
			if replanErr != nil {
				return nil, fmt.Errorf("replan after observations: %w", replanErr)
			}
			revision, revisionErr := controller.Revise(reasonCode, nextPlan)
			if revisionErr != nil {
				return nil, fmt.Errorf("create plan revision: %w", revisionErr)
			}
			revisionNode := tr.startNode(ctx, "plan_revision", "runtime.replan", map[string]any{"revision_uuid": revision.RevisionUUID.String(), "parent_revision_uuid": revision.ParentRevisionUUID.String(), "reason_code": revision.ReasonCode, "plan_id": revision.Plan.PlanID})
			if persistErr := revisionService.Persist(ctx, runtimeEnv, snapshot.TenantUUID, runUUID, revision, controller.budget, controller.Observations()); persistErr != nil {
				tr.failNode(ctx, revisionNode, "plan_revision", "runtime.replan", persistErr)
				return nil, fmt.Errorf("persist plan revision: %w", persistErr)
			}
			tr.endNode(ctx, revisionNode, "plan_revision", "runtime.replan", map[string]any{"revision_uuid": revision.RevisionUUID.String(), "reason_code": revision.ReasonCode, "superseded_task_refs": revision.SupersededTaskRefs, "new_task_refs": revision.NewTaskRefs})
			ctx = context.WithValue(ctx, "runtime_plan_revision_uuid", revision.RevisionUUID.String())
			tr.withRuntimeRevision(revision.RevisionUUID)
			plan = &revision.Plan
		}
	}
	if controller != nil {
		if err := controller.budget.ValidatePlanConcurrency(*plan); err != nil {
			return nil, fmt.Errorf("validate runtime concurrency budget: %w", err)
		}
		if controller.budget.MaxExecutionDuration <= 0 {
			return nil, fmt.Errorf("runtime execution duration budget is required")
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, controller.budget.MaxExecutionDuration, ErrRuntimeBudgetExhausted)
		defer cancel()
	}
	tr.withPlan(plan.PlanID)
	traceID := fmt.Sprintf("trace_%d", time.Now().UnixNano())
	if raw := strings.TrimSpace(reqctx.GetTraceID(ctx)); raw != "" {
		traceID = raw
	}
	meta := tr.applyExecutionMeta(agentschema.ExecutionMeta{
		RequestID:  fmt.Sprintf("req_%d", time.Now().UnixNano()),
		UserID:     reqctx.GetUserID(ctx),
		TenantUUID: strings.TrimSpace(reqctx.GetTenantUUID(ctx)),
		TraceID:    traceID,
		Metadata: map[string]any{
			"transport": "engine.invoke",
			"env":       strings.TrimSpace(reqctx.GetEnv(ctx)),
		},
	})

	emitMu := &sync.Mutex{}
	hooks := &agent.PlanExecutionHooks{
		OnTaskStart: func(task flowschema.PlanTask) error {
			if controller != nil {
				if err := controller.budget.ConsumeTask(normalizeNodeKind(task.NodeKind)); err != nil {
					return err
				}
			}
			if normalizeNodeKind(task.NodeKind) == "tooling" && snapshot != nil {
				capabilityUUID, err := taskCapabilityUUID(task)
				if err != nil {
					return err
				}
				if err := requireTaskApproval(ctx, snapshot, capabilityUUID); err != nil {
					return err
				}
			}
			emitMu.Lock()
			defer emitMu.Unlock()
			_ = sink.Emit(dto.EventNodeStart, map[string]any{
				"planner_mode":    dto.PlannerModeUnified,
				"plan_id":         plan.PlanID,
				"task_id":         task.TaskID,
				"flow_id":         task.FlowID,
				"node_id":         task.TaskID,
				"node_kind":       normalizeNodeKind(task.NodeKind),
				"node_ref":        normalizeNodeRef(task),
				"node_name":       normalizeCandidateName(task),
				"node_desc":       normalizeCandidateDesc(task),
				"source_scope":    normalizeSourceScope(task.SourceScope),
				"team_id":         strings.TrimSpace(task.TeamID),
				"handoff_task_id": strings.TrimSpace(task.HandoffTaskID),
				"failure_policy":  strings.TrimSpace(task.FailurePolicy),
				"context_ref_id":  strings.TrimSpace(task.ContextRefID),
				"stage":           task.Stage,
				"depends_on":      task.DependsOn,
			})
			return nil
		},
		OnTaskEnd: func(task flowschema.PlanTask, out *agentschema.ExecutionResult, runErr error) error {
			emitMu.Lock()
			defer emitMu.Unlock()
			if runErr == nil && snapshot != nil {
				if err := AttachCapabilityVerificationEvidence(ctx, snapshot, task, out); err != nil {
					return fmt.Errorf("attach capability verification evidence: %w", err)
				}
				if err := verificationSvc.PersistVerifiedCapabilityTask(ctx, runtimeEnv, snapshot.TenantUUID, runUUID, snapshot, task, out); err != nil {
					return fmt.Errorf("persist capability verification evidence: %w", err)
				}
			}
			taskErr := taskExecutionError(out, runErr)
			status := "completed"
			if taskErr != nil {
				status = "failed"
			} else if out != nil && isAwaitingParamsResult(out) {
				status = dto.AgentTaskStatusAwaitingParams
			}
			taskEndPayload := map[string]any{
				"planner_mode":    dto.PlannerModeUnified,
				"plan_id":         plan.PlanID,
				"task_id":         task.TaskID,
				"flow_id":         task.FlowID,
				"node_id":         task.TaskID,
				"node_kind":       normalizeNodeKind(task.NodeKind),
				"node_ref":        normalizeNodeRef(task),
				"node_name":       normalizeCandidateName(task),
				"node_desc":       normalizeCandidateDesc(task),
				"source_scope":    normalizeSourceScope(task.SourceScope),
				"team_id":         strings.TrimSpace(task.TeamID),
				"handoff_task_id": strings.TrimSpace(task.HandoffTaskID),
				"failure_policy":  strings.TrimSpace(task.FailurePolicy),
				"context_ref_id":  strings.TrimSpace(task.ContextRefID),
				"stage":           task.Stage,
				"depends_on":      task.DependsOn,
				"status":          status,
				"error": func() string {
					if taskErr == nil {
						return ""
					}
					return taskFailureReason(taskErr)
				}(),
				"result_summary": func() map[string]any {
					if out == nil {
						return nil
					}
					return map[string]any{
						"success": out.Success,
						"step_id": out.StepID,
					}
				}(),
			}
			if err := persistTaskSkillState(ctx, task, status, out, taskErr); err != nil {
				taskEndPayload["status"] = dto.AgentTaskStatusFailed
				taskEndPayload["error"] = "task_state.persist_failed"
				enrichTaskEndPayload(taskEndPayload, out)
				_ = sink.Emit(dto.EventNodeEnd, taskEndPayload)
				_ = sink.Emit(dto.EventError, map[string]any{
					"message":        "保存 Skill 状态失败",
					"reason_code":    "task_state.persist_failed",
					"plan_id":        plan.PlanID,
					"task_id":        task.TaskID,
					"flow_id":        task.FlowID,
					"node_id":        task.TaskID,
					"node_kind":      normalizeNodeKind(task.NodeKind),
					"node_ref":       normalizeNodeRef(task),
					"stage":          task.Stage,
					"depends_on":     task.DependsOn,
					"failure_policy": strings.TrimSpace(task.FailurePolicy),
				})
				return err
			}
			if stateService, ok := runTaskStateServiceFromContext(ctx); ok {
				if err := stateService.Persist(ctx, task, status); err != nil {
					return err
				}
			}
			enrichTaskEndPayload(taskEndPayload, out)
			_ = sink.Emit(dto.EventNodeEnd, taskEndPayload)
			if status == dto.AgentTaskStatusAwaitingParams {
				_ = sink.Emit(dto.EventAgentRunAwaitingParams, awaitingPayloadFromResult(task, out))
			}
			return nil
		},
	}

	report, execErr := e.mgr.ExecutePlanWithHooks(ctx, *plan, meta, hooks)
	var out *agentschema.ExecutionResult
	if report != nil {
		out = report.FinalResult
	}
	verificationNode := tr.startNode(ctx, "verification", "runtime.execution", map[string]any{"plan_id": plan.PlanID})
	verdict, verifyErr := (DeterministicExecutionVerifier{}).Verify(ctx, report, out, execErr)
	if verifyErr != nil {
		tr.failNode(ctx, verificationNode, "verification", "runtime.execution", verifyErr)
		if execErr == nil {
			execErr = fmt.Errorf("verify execution result: %w", verifyErr)
		}
		verdict = VerificationVerdict{Class: VerificationFatal, ReasonCode: "verification.contract_invalid", TaskRefs: failedTaskRefs(report)}
	} else {
		tr.endNode(ctx, verificationNode, "verification", "runtime.execution", map[string]any{"class": verdict.Class, "reason_code": verdict.ReasonCode, "task_refs": verdict.TaskRefs})
	}
	if verdict.Class == VerificationPass && snapshot != nil {
		contractNode := tr.startNode(ctx, "capability_contract_verification", "runtime.execution", map[string]any{"plan_id": plan.PlanID})
		if contractErr := VerifyCapabilityContracts(snapshot, *plan, report); contractErr != nil {
			tr.failNode(ctx, contractNode, "capability_contract_verification", "runtime.execution", contractErr)
			verdict = VerificationVerdict{Class: VerificationFatal, ReasonCode: "capability.verification_evidence_invalid", TaskRefs: incompleteTaskRefs(report)}
			if execErr == nil {
				execErr = fmt.Errorf("verify capability contract evidence: %w", contractErr)
			}
		} else {
			tr.endNode(ctx, contractNode, "capability_contract_verification", "runtime.execution", map[string]any{"plan_id": plan.PlanID})
		}
	}
	_ = sink.Emit("verification", map[string]any{"plan_id": plan.PlanID, "class": verdict.Class, "reason_code": verdict.ReasonCode, "task_refs": verdict.TaskRefs})
	if snapshot != nil {
		if persistErr := verificationSvc.Persist(ctx, runtimeEnv, snapshot.TenantUUID, runUUID, snapshot.SnapshotUUID, verdict, verificationArtifactRef(report, out)); persistErr != nil {
			return nil, fmt.Errorf("persist verification evidence: %w", persistErr)
		}
	}
	if verdict.Class == VerificationRetryable || verdict.Class == VerificationReplaceable {
		if approvedResume {
			return nil, fmt.Errorf("approved runtime resume requires a new plan revision for recovery")
		}
		recoveryMode := verdict.Class
		recoveryNode := tr.startNode(ctx, "recovery_decision", "runtime."+recoveryMode, map[string]any{"plan_id": plan.PlanID, "reason_code": verdict.ReasonCode, "task_refs": verdict.TaskRefs})
		if controller == nil || snapshot == nil || revisionService == nil || runUUID == uuid.Nil || runtimeEnv == "" {
			err := fmt.Errorf("runtime recovery requires an initialized runtime snapshot")
			tr.failNode(ctx, recoveryNode, "recovery_decision", "runtime."+recoveryMode, err)
			return nil, err
		}
		var (
			recoveryPlan       flowschema.ExecutionPlan
			recoveryPlanErr    error
			revisionReason     string
			triggerObservation *ResourceObservation
		)
		switch recoveryMode {
		case VerificationRetryable:
			recoveryPlan, recoveryPlanErr = retryPlanFromVerdict(snapshot, *plan, verdict)
			revisionReason = "verification.retryable"
		case VerificationReplaceable:
			planner, hasPlanner := PlanRevisionPlannerFromContext(ctx)
			if !hasPlanner {
				recoveryPlanErr = fmt.Errorf("plan revision planner is not configured")
				break
			}
			var observation ResourceObservation
			revisionReason, recoveryPlan, observation, recoveryPlanErr = semanticReplacementPlanFromVerdict(ctx, controller, planner, snapshot, *plan, verdict, runtimeEnv)
			if recoveryPlanErr == nil {
				triggerObservation = &observation
			}
		}
		if recoveryPlanErr != nil {
			tr.failNode(ctx, recoveryNode, "recovery_decision", "runtime."+recoveryMode, recoveryPlanErr)
			return nil, fmt.Errorf("build recovery plan: %w", recoveryPlanErr)
		}
		retryRevision, reviseErr := controller.Revise(revisionReason, recoveryPlan)
		if reviseErr != nil {
			tr.failNode(ctx, recoveryNode, "recovery_decision", "runtime."+recoveryMode, reviseErr)
			return nil, fmt.Errorf("create recovery plan revision: %w", reviseErr)
		}
		if triggerObservation != nil {
			retryRevision.TriggerObservationUUIDs = []uuid.UUID{triggerObservation.ObservationUUID}
		}
		if persistErr := revisionService.Persist(ctx, runtimeEnv, snapshot.TenantUUID, runUUID, retryRevision, controller.budget, controller.Observations()); persistErr != nil {
			tr.failNode(ctx, recoveryNode, "recovery_decision", "runtime."+recoveryMode, persistErr)
			return nil, fmt.Errorf("persist recovery plan revision: %w", persistErr)
		}
		tr.endNode(ctx, recoveryNode, "recovery_decision", "runtime."+recoveryMode, map[string]any{"revision_uuid": retryRevision.RevisionUUID.String(), "reason_code": retryRevision.ReasonCode, "plan_id": retryRevision.Plan.PlanID})
		ctx = context.WithValue(ctx, "runtime_plan_revision_uuid", retryRevision.RevisionUUID.String())
		tr.withRuntimeRevision(retryRevision.RevisionUUID)
		plan = &retryRevision.Plan
		tr.withPlan(plan.PlanID)
		if err := controller.budget.ValidatePlanConcurrency(*plan); err != nil {
			return nil, fmt.Errorf("validate recovery concurrency budget: %w", err)
		}
		report, execErr = e.mgr.ExecutePlanWithHooks(ctx, *plan, meta, hooks)
		out = nil
		if report != nil {
			out = report.FinalResult
		}
		retryVerificationNode := tr.startNode(ctx, "verification", "runtime."+recoveryMode, map[string]any{"plan_id": plan.PlanID})
		verdict, verifyErr = (DeterministicExecutionVerifier{}).Verify(ctx, report, out, execErr)
		if verifyErr != nil {
			tr.failNode(ctx, retryVerificationNode, "verification", "runtime."+recoveryMode, verifyErr)
			if execErr == nil {
				execErr = fmt.Errorf("verify retry execution result: %w", verifyErr)
			}
			verdict = VerificationVerdict{Class: VerificationFatal, ReasonCode: "verification.contract_invalid", TaskRefs: failedTaskRefs(report)}
		} else {
			tr.endNode(ctx, retryVerificationNode, "verification", "runtime."+recoveryMode, map[string]any{"class": verdict.Class, "reason_code": verdict.ReasonCode, "task_refs": verdict.TaskRefs})
		}
		if verdict.Class == VerificationPass && snapshot != nil {
			contractNode := tr.startNode(ctx, "capability_contract_verification", "runtime."+recoveryMode, map[string]any{"plan_id": plan.PlanID})
			if contractErr := VerifyCapabilityContracts(snapshot, *plan, report); contractErr != nil {
				tr.failNode(ctx, contractNode, "capability_contract_verification", "runtime."+recoveryMode, contractErr)
				verdict = VerificationVerdict{Class: VerificationFatal, ReasonCode: "capability.verification_evidence_invalid", TaskRefs: incompleteTaskRefs(report)}
				if execErr == nil {
					execErr = fmt.Errorf("verify recovery capability contract evidence: %w", contractErr)
				}
			} else {
				tr.endNode(ctx, contractNode, "capability_contract_verification", "runtime."+recoveryMode, map[string]any{"plan_id": plan.PlanID})
			}
		}
		if verdict.Class == VerificationRetryable || verdict.Class == VerificationReplaceable {
			verdict = VerificationVerdict{Class: VerificationFatal, ReasonCode: "recovery." + recoveryMode + "_exhausted", TaskRefs: verdict.TaskRefs}
			if execErr == nil {
				execErr = fmt.Errorf("runtime recovery exhausted")
			}
		}
		_ = sink.Emit("verification", map[string]any{"plan_id": plan.PlanID, "class": verdict.Class, "reason_code": verdict.ReasonCode, "task_refs": verdict.TaskRefs})
		if persistErr := verificationSvc.Persist(ctx, runtimeEnv, snapshot.TenantUUID, runUUID, snapshot.SnapshotUUID, verdict, verificationArtifactRef(report, out)); persistErr != nil {
			return nil, fmt.Errorf("persist recovery verification evidence: %w", persistErr)
		}
	}
	if verdict.Class == VerificationBlocked && execErr == nil {
		execErr = fmt.Errorf("runtime execution blocked")
	}
	outcome := runtimeOutcomeFromVerdict(verdict)
	recovery := recoveryDecisionFromVerdict(verdict)
	if verdict.Class == VerificationNeedsInput || isAwaitingParamsResult(out) {
		if verdict.ReasonCode == "approval.required" {
			_ = sink.Emit(dto.EventAgentRunAwaitingParams, map[string]any{
				"plan_id":                            plan.PlanID,
				"reason_code":                        verdict.ReasonCode,
				"required_approval_capability_uuids": verdict.RequiredApprovalCapabilityUUIDs,
				"approval_request_uuid":              verdict.ApprovalRequestUUID,
			})
		}
		responsePlan := responsePlanFromContext(ctx)
		missingFields := verdict.RequiredInputFields
		if len(missingFields) == 0 {
			missingFields = stringSliceFromAny(resultValue(out, "missing_fields"))
		}
		responsePlan = ensureResponsePlanForClarify(ctx, responsePlan, missingFields)
		userMsg := firstNonEmpty(
			anyToString(resultValue(out, "message")),
			BuildFinalResponseContent(responsePlan, "", nil),
		)
		contextLayers := responseContextLayersFromContext(ctx)
		finalNode := tr.startNode(ctx, "final_response", "plan.awaiting_params", map[string]any{
			"plan_id":               plan.PlanID,
			"response_mode":         responseModeString(responsePlan),
			"target_capability_ids": responseTargetIDs(responsePlan),
			"model_selection":       modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse),
		})
		_ = sink.Emit(dto.EventFinal, map[string]any{
			"success": true,
			"data":    map[string]any{"content": userMsg},
			"metadata": withRuntimeOutcome(mergeResponseMetadata(mergeTraceMetadata(map[string]any{
				"trace_id": traceID, "plan_id": plan.PlanID,
			}, tr), responsePlan, contextLayers, modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse)), outcome),
		})
		tr.endNode(ctx, finalNode, "final_response", "plan.awaiting_params", map[string]any{"content_digest": digestString(userMsg), "response_mode": responseModeString(responsePlan), "target_capability_ids": responseTargetIDs(responsePlan), "used_context_layers": contextLayers})
		userAction := verdict.UserAction
		if userAction == "" {
			userAction = userActionForOutcome(outcome)
		}
		_ = sink.Emit(dto.EventEnd, map[string]any{"success": true, "outcome": outcome.Status, "reason_code": outcome.ReasonCode, "user_action_required": userAction})
		return out, nil
	}
	if execErr != nil {
		responsePlan := responsePlanFromContext(ctx)
		if responsePlan != nil {
			responsePlan.ResponseMode = ResponseModeErrorExplain
		}
		userMsg := BuildFinalResponseContent(responsePlan, "", execErr)
		contextLayers := responseContextLayersFromContext(ctx)
		finalNode := tr.startNode(ctx, "final_response", "plan.error", map[string]any{
			"plan_id":               plan.PlanID,
			"response_mode":         responseModeString(responsePlan),
			"target_capability_ids": responseTargetIDs(responsePlan),
			"model_selection":       modelSelectionFromContext(ctx, ModelPolicyNodeErrorExplain),
		})
		_ = sink.Emit(dto.EventFinal, map[string]any{
			"success": false,
			"data": map[string]any{
				"content": userMsg,
			},
			"metadata": withRuntimeOutcome(mergeResponseMetadata(mergeTraceMetadata(map[string]any{
				"trace_id": traceID,
				"plan_id":  plan.PlanID,
			}, tr), responsePlan, contextLayers, modelSelectionFromContext(ctx, ModelPolicyNodeErrorExplain)), outcome),
		})
		tr.endNode(ctx, finalNode, "final_response", "plan.error", map[string]any{
			"success":               false,
			"content_digest":        digestString(userMsg),
			"response_mode":         responseModeString(responsePlan),
			"target_capability_ids": responseTargetIDs(responsePlan),
			"used_context_layers":   contextLayers,
		})
		_ = sink.Emit(dto.EventError, map[string]any{"message": "执行失败", "plan_id": plan.PlanID, "user_message": userMsg, "outcome": outcome.Status, "reason_code": outcome.ReasonCode, "recovery_class": recovery.Class})
		_ = sink.Emit(dto.EventEnd, map[string]any{"success": false, "outcome": outcome.Status, "reason_code": outcome.ReasonCode, "recovery_class": recovery.Class, "user_action_required": userActionForOutcome(outcome)})
		return nil, execErr
	}
	if out == nil {
		err := errors.New("agent execution completed without a terminal result")
		emitAgentRunFailure(ctx, sink, plan.PlanID, "execution.no_result", "执行未返回可验证结果", err, "")
		return nil, err
	}
	responsePlan := responsePlanFromContext(ctx)
	envelope, envelopeErr := responseEnvelopeFromExecutionResultWithTaskRefs(out.Data, finalResponseUpstreamTaskRefs(plan))
	if envelopeErr == nil && envelope != nil {
		envelopeErr = evidence.Verify(ctx, envelope)
	}
	if envelopeErr != nil {
		// The final Skill invocation may succeed while its result is rejected by
		// the platform contract. Persist that validation as a real failed trace
		// node; otherwise Trace only shows the successful Skill call and a red
		// run badge with no actionable failure detail.
		validationNode := tr.startNode(ctx, "final_response", "response_envelope_validation", map[string]any{
			"plan_id":      plan.PlanID,
			"failure_kind": "response_contract_invalid",
		})
		tr.failNode(ctx, validationNode, "final_response", "response_envelope_validation", envelopeErr)
		emitAgentRunFailure(ctx, sink, plan.PlanID, "final.response_contract_invalid", "最终答复结果不符合平台契约", envelopeErr, "")
		return nil, envelopeErr
	}
	if envelope == nil {
		err := fmt.Errorf("agent.response_contract_invalid: response_envelope is required for an execution plan")
		validationNode := tr.startNode(ctx, "final_response", "response_envelope_validation", map[string]any{
			"plan_id":      plan.PlanID,
			"failure_kind": "response_contract_invalid",
		})
		tr.failNode(ctx, validationNode, "final_response", "response_envelope_validation", err)
		emitAgentRunFailure(ctx, sink, plan.PlanID, "final.response_contract_invalid", "最终答复结果不符合平台契约", err, "")
		return nil, err
	}
	// The response envelope is the sole final-result source. Each channel renders
	// it with its locale; do not persist a second, model-authored summary.
	content := ""
	contextLayers := responseContextLayersFromContext(ctx)
	cbNode := tr.startNode(ctx, "context_builder", responseModeString(responsePlan), map[string]any{
		"response_mode":       responseModeString(responsePlan),
		"used_context_layers": contextLayers,
		"model_selection":     modelSelectionFromContext(ctx, ModelPolicyNodeContextBuilder),
	})
	tr.endNode(ctx, cbNode, "context_builder", responseModeString(responsePlan), map[string]any{"used_context_layers": contextLayers})
	finalNode := tr.startNode(ctx, "final_response", "plan.final", map[string]any{
		"plan_id":               plan.PlanID,
		"response_mode":         responseModeString(responsePlan),
		"target_capability_ids": responseTargetIDs(responsePlan),
		"model_selection":       modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse),
	})
	_ = sink.Emit(dto.EventFinal, map[string]any{
		"success": true,
		"data": mergeFinalContent(flowschema.Result{
			"response_envelope": envelope,
		}, content),
		"metadata": withRuntimeOutcome(mergeResponseMetadata(mergeTraceMetadata(map[string]any{
			"trace_id": traceID,
			"plan_id":  plan.PlanID,
		}, tr), responsePlan, contextLayers, modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse)), outcome),
	})
	tr.endNode(ctx, finalNode, "final_response", "plan.final", map[string]any{
		"content_digest":        digestAny(envelope),
		"calculation_evidence":  evidence.TraceSummary(envelope),
		"response_mode":         responseModeString(responsePlan),
		"target_capability_ids": responseTargetIDs(responsePlan),
		"used_context_layers":   contextLayers,
	})
	_ = sink.Emit(dto.EventEnd, map[string]any{"success": outcome.Status == RuntimeOutcomeCompleted, "outcome": outcome.Status, "reason_code": outcome.ReasonCode, "user_action_required": userActionForOutcome(outcome)})
	return out, nil
}

func planHasNonWorkflow(plan *flowschema.ExecutionPlan) bool {
	if plan == nil {
		return false
	}
	for _, task := range plan.Tasks {
		if normalizeNodeKind(task.NodeKind) != dto.NodeKindWorkflow {
			return true
		}
	}
	return false
}

func shouldExecutePlanForResponse(responsePlan *ResponsePlan, execPlan *flowschema.ExecutionPlan) bool {
	if execPlan == nil || len(execPlan.Tasks) == 0 {
		return false
	}
	// Tool/Skill Planner 是执行意图的裁决层；Response Planner 只决定最终回复形态，
	// 不能否决已经生成的执行计划，否则会退回 LLM 文案并造成“假成功”。
	return true
}

func responsePlanRequiresExecution(responsePlan *ResponsePlan) bool {
	if responsePlan == nil {
		return false
	}
	if responsePlan.ShouldCallTool {
		return true
	}
	if responsePlan.ResponseMode == ResponseModeSkillExecution {
		return true
	}
	for _, intent := range responsePlan.ResponseIntents {
		if intent == ResponseIntentSkillExecution {
			return true
		}
	}
	return false
}

func isAwaitingParamsResult(out *agentschema.ExecutionResult) bool {
	if out == nil {
		return false
	}
	status := strings.TrimSpace(anyToString(resultValue(out, "status")))
	return strings.EqualFold(status, dto.AgentTaskStatusAwaitingParams) || strings.EqualFold(status, "collecting")
}

func resultValue(out *agentschema.ExecutionResult, key string) any {
	if out == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	if out.Data != nil {
		if v, ok := out.Data[key]; ok {
			return v
		}
		if result := mapFromAny(out.Data["result"]); len(result) > 0 {
			if v, ok := result[key]; ok {
				return v
			}
		}
	}
	if out.Metadata != nil {
		if v, ok := out.Metadata[key]; ok {
			return v
		}
	}
	return nil
}

func awaitingPayloadFromResult(task flowschema.PlanTask, out *agentschema.ExecutionResult) map[string]any {
	payload := map[string]any{
		"task_id":        task.TaskID,
		"node_kind":      normalizeNodeKind(task.NodeKind),
		"node_ref":       normalizeNodeRef(task),
		"skill_id":       skillIDFromPlanTask(task),
		"source_scope":   normalizeSourceScope(task.SourceScope),
		"action":         taskAction(task),
		"status":         dto.AgentTaskStatusAwaitingParams,
		"missing_fields": stringSliceFromAny(resultValue(out, "missing_fields")),
		"message":        anyToString(resultValue(out, "message")),
	}
	if task.Params != nil {
		payload["collected_params"] = clonePlanParams(task.Params)
		if capabilityID := strings.TrimSpace(fmt.Sprint(task.Params["capability_id"])); capabilityID != "" {
			payload["capability_id"] = capabilityID
		}
	}
	if statePatch := mapFromAny(resultValue(out, "state_patch")); len(statePatch) > 0 {
		payload["state_patch"] = statePatch
		payload["collected_params"] = statePatch
	}
	return payload
}

func (e *Engine) emitClarifyFinal(ctx context.Context, sink EventSink, tr *traceRuntime, plan *flowschema.ExecutionPlan, responsePlan *ResponsePlan, content string, missing []string) {
	contextLayers := responseContextLayersFromContext(ctx)
	planID := ""
	if plan != nil {
		planID = plan.PlanID
	}
	finalNode := tr.startNode(ctx, "final_response", "clarify.params", map[string]any{
		"plan_id":               planID,
		"response_mode":         responseModeString(responsePlan),
		"target_capability_ids": responseTargetIDs(responsePlan),
		"missing_fields":        missing,
		"model_selection":       modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse),
	})
	_ = sink.Emit(dto.EventFinal, map[string]any{
		"success": true,
		"data": map[string]any{
			"content": content,
		},
		"metadata": mergeResponseMetadata(mergeTraceMetadata(map[string]any{
			"trace_id": tr.meta.TraceID,
			"plan_id":  planID,
		}, tr), responsePlan, contextLayers, modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse)),
	})
	tr.endNode(ctx, finalNode, "final_response", "clarify.params", map[string]any{
		"content_digest":        digestString(content),
		"response_mode":         responseModeString(responsePlan),
		"target_capability_ids": responseTargetIDs(responsePlan),
		"used_context_layers":   contextLayers,
	})
	_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
}

type teamMemberRuntime struct {
	ChildAgentID  uint64
	ChildAgentKey string
	Role          string
	SkillIDs      []string
}

// builtInTeamPlanFromContext compiles the selected team's persisted graph. The
// name intentionally remains for call-site stability only: no built-in team
// name, agent key or task graph is recognised here.
func builtInTeamPlanFromContext(ctx context.Context) (*flowschema.ExecutionPlan, bool, error) {
	if ctx == nil || !strings.EqualFold(strings.TrimSpace(contextString(ctx, "agent_workspace_mode")), "team") {
		return nil, false, nil
	}
	teamID := strings.TrimSpace(contextString(ctx, "team_id"))
	parentAgentID := contextUint64(ctx, "parent_agent_id")
	if teamID == "" || parentAgentID == 0 {
		return nil, true, fmt.Errorf("team runtime context is incomplete")
	}
	material := strings.TrimSpace(contextString(ctx, "team_user_message"))
	if material == "" {
		return nil, true, fmt.Errorf("team user message is required")
	}
	locale := strings.TrimSpace(contextString(ctx, "locale"))
	if locale == "" {
		return nil, true, fmt.Errorf("team runtime locale is required")
	}
	rawSpec, ok := ctx.Value("team_orchestration").(map[string]any)
	if !ok || len(rawSpec) == 0 {
		return nil, true, fmt.Errorf("team orchestration is required")
	}
	raw, err := json.Marshal(rawSpec)
	if err != nil {
		return nil, true, fmt.Errorf("team orchestration serialization failed: %w", err)
	}
	spec, err := modelagent.ParseTeamOrchestrationSpec(raw)
	if err != nil {
		return nil, true, err
	}
	members := make(map[string]teamMemberRuntime, len(spec.Tasks))
	for _, member := range teamMembersFromContext(ctx) {
		role := strings.ToLower(strings.TrimSpace(member.Role))
		if role == "" {
			continue
		}
		if _, exists := members[role]; exists {
			return nil, true, fmt.Errorf("team member role is duplicated: %s", role)
		}
		members[role] = member
	}
	parentSkillIDs := normalizeStringList(anyStringSlice(ctx.Value("agent_bound_skill_ids")))
	teamKey := strings.TrimSpace(contextString(ctx, "team_key"))
	plan := &flowschema.ExecutionPlan{PlanID: fmt.Sprintf("team_orchestration_%d", time.Now().UnixNano()), Tasks: make([]flowschema.PlanTask, 0, len(spec.Tasks))}
	for _, configuredTask := range spec.Tasks {
		taskID := strings.TrimSpace(configuredTask.TaskID)
		dependsOn := normalizeStringList(configuredTask.DependsOn)
		paramRefs := teamTaskDependencyRefs(dependsOn)
		failurePolicy := strings.TrimSpace(configuredTask.FailurePolicy)
		if failurePolicy == "" {
			failurePolicy = "fail-fast"
		}
		switch strings.ToLower(strings.TrimSpace(configuredTask.NodeKind)) {
		case "agent_handoff":
			member, ok := members[strings.ToLower(strings.TrimSpace(configuredTask.AssigneeRole))]
			if !ok || member.ChildAgentID == 0 || strings.TrimSpace(member.ChildAgentKey) == "" {
				return nil, true, fmt.Errorf("team task assignee is unavailable: %s", taskID)
			}
			if !containsNormalizedString(member.SkillIDs, configuredTask.SkillID) {
				return nil, true, fmt.Errorf("team task skill is not bound to assignee: task=%s skill=%s", taskID, configuredTask.SkillID)
			}
			plan.Tasks = append(plan.Tasks, flowschema.PlanTask{TaskID: taskID, FlowID: member.ChildAgentKey, NodeKind: dto.NodeKindHandoff, NodeRef: member.ChildAgentKey, AgentID: member.ChildAgentKey, TeamID: teamID, HandoffTaskID: taskID, FailurePolicy: failurePolicy, Stage: configuredTask.Stage, DependsOn: dependsOn, ParamRefs: paramRefs, Params: map[string]any{"team_key": teamKey, "child_agent_id": member.ChildAgentID, "child_agent_key": member.ChildAgentKey, "message": material, "context": map[string]any{"locale": locale}, "payload": map[string]any{"child_skill_id": configuredTask.SkillID, "content": material, "context": "team_orchestration", "source": map[string]any{"type": "text", "content": material, "context": "team_orchestration"}}}})
		case "skill":
			if !containsNormalizedString(parentSkillIDs, configuredTask.SkillID) {
				return nil, true, fmt.Errorf("team task skill is not bound to parent agent: task=%s skill=%s", taskID, configuredTask.SkillID)
			}
			// 团队 Skill 的原始用户材料使用 payload.message 作为稳定契约。content
			// 只保留为同一 payload 中的业务材料字段，不能替代 evidence_sources 声明的来源。
			plan.Tasks = append(plan.Tasks, flowschema.PlanTask{TaskID: taskID, FlowID: configuredTask.SkillID, NodeKind: dto.NodeKindSkill, NodeRef: configuredTask.SkillID, SourceScope: "agent", AgentID: fmt.Sprintf("%d", parentAgentID), FailurePolicy: failurePolicy, Stage: configuredTask.Stage, DependsOn: dependsOn, ParamRefs: paramRefs, Params: map[string]any{"context": map[string]any{"locale": locale}, "payload": map[string]any{"message": material, "content": material, "context": "team_orchestration"}}})
		}
	}
	return plan, true, nil
}

func teamTaskDependencyRefs(dependsOn []string) map[string]string {
	if len(dependsOn) == 0 {
		return nil
	}
	refs := make(map[string]string, len(dependsOn))
	for _, taskID := range dependsOn {
		// ParamRef must address a concrete member of task output. Handoff
		// tasks expose their business payload at output.result.
		refs["upstream_"+strings.TrimSpace(taskID)] = "{{task." + strings.TrimSpace(taskID) + ".output.result}}"
	}
	return refs
}

func containsNormalizedString(values []string, expected string) bool {
	expected = strings.ToLower(strings.TrimSpace(expected))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == expected {
			return true
		}
	}
	return false
}

func teamMembersFromContext(ctx context.Context) []teamMemberRuntime {
	raw := ctx.Value("team_members")
	switch v := raw.(type) {
	case []map[string]any:
		out := make([]teamMemberRuntime, 0, len(v))
		for _, item := range v {
			if member := teamMemberFromMap(item); member.ChildAgentID != 0 || member.ChildAgentKey != "" || member.Role != "" {
				out = append(out, member)
			}
		}
		return out
	case []any:
		out := make([]teamMemberRuntime, 0, len(v))
		for _, item := range v {
			if member := teamMemberFromMap(mapFromAny(item)); member.ChildAgentID != 0 || member.ChildAgentKey != "" || member.Role != "" {
				out = append(out, member)
			}
		}
		return out
	default:
		return nil
	}
}

func teamMemberFromMap(item map[string]any) teamMemberRuntime {
	if len(item) == 0 {
		return teamMemberRuntime{}
	}
	return teamMemberRuntime{
		ChildAgentID:  uint64FromAny(item["child_agent_id"]),
		ChildAgentKey: strings.TrimSpace(anyToString(item["child_agent_key"])),
		Role:          strings.ToLower(strings.TrimSpace(anyToString(item["role"]))),
		SkillIDs:      normalizeStringList(anyStringSlice(item["skill_ids"])),
	}
}

func contextString(ctx context.Context, key string) string {
	if ctx == nil {
		return ""
	}
	return strings.TrimSpace(anyToString(ctx.Value(key)))
}

func contextUint64(ctx context.Context, key string) uint64 {
	if ctx == nil {
		return 0
	}
	return uint64FromAny(ctx.Value(key))
}

func uint64FromAny(v any) uint64 {
	switch n := v.(type) {
	case uint64:
		return n
	case uint:
		return uint64(n)
	case uint32:
		return uint64(n)
	case int:
		if n > 0 {
			return uint64(n)
		}
	case int64:
		if n > 0 {
			return uint64(n)
		}
	case float64:
		if n > 0 {
			return uint64(n)
		}
	case json.Number:
		parsed, err := strconv.ParseUint(strings.TrimSpace(string(n)), 10, 64)
		if err == nil {
			return parsed
		}
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(n), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func (e *Engine) missingRequiredArgsForPlan(ctx context.Context, plan *flowschema.ExecutionPlan) []string {
	if e == nil || e.mgr == nil || plan == nil || len(plan.Tasks) == 0 {
		return nil
	}
	candidates := e.mgr.BuildToolCallCandidatesWithContext(agent.CandidateBuildContextFromRequest(ctx), 0)
	byRef := make(map[string]agent.ToolCallCandidate, len(candidates)*2)
	for _, candidate := range candidates {
		if ref := strings.TrimSpace(candidate.NodeRef); ref != "" {
			byRef[strings.ToLower(ref)] = candidate
		}
		if name := strings.TrimSpace(candidate.Name); name != "" {
			byRef[strings.ToLower(name)] = candidate
		}
		if flowID := strings.TrimSpace(candidate.FlowID); flowID != "" {
			byRef[strings.ToLower(flowID)] = candidate
		}
	}
	missing := make([]string, 0, 4)
	for _, task := range plan.Tasks {
		task = mergePendingTaskParams(ctx, task)
		candidate, ok := byRef[strings.ToLower(strings.TrimSpace(normalizeNodeRef(task)))]
		if !ok {
			candidate, ok = byRef[strings.ToLower(strings.TrimSpace(task.FlowID))]
		}
		if !ok || len(candidate.ActionRequiredArgs) == 0 {
			continue
		}
		action := taskAction(task)
		if action == "" {
			continue
		}
		for _, field := range candidate.ActionRequiredArgs[action] {
			if !hasPlanParamPath(task.Params, field) {
				missing = append(missing, field)
			}
		}
	}
	return normalizeStringList(missing)
}

func (e *Engine) applyRuntimeParamState(ctx context.Context, plan *flowschema.ExecutionPlan) *flowschema.ExecutionPlan {
	if e == nil || e.mgr == nil || plan == nil || len(plan.Tasks) == 0 {
		return plan
	}
	candidates := e.mgr.BuildToolCallCandidatesWithContext(agent.CandidateBuildContextFromRequest(ctx), 0)
	byRef := make(map[string]agent.ToolCallCandidate, len(candidates)*2)
	for _, candidate := range candidates {
		if ref := strings.TrimSpace(candidate.NodeRef); ref != "" {
			byRef[strings.ToLower(ref)] = candidate
		}
		if name := strings.TrimSpace(candidate.Name); name != "" {
			byRef[strings.ToLower(name)] = candidate
		}
		if flowID := strings.TrimSpace(candidate.FlowID); flowID != "" {
			byRef[strings.ToLower(flowID)] = candidate
		}
	}
	for i := range plan.Tasks {
		task := mergePendingTaskParams(ctx, plan.Tasks[i])
		candidate, ok := byRef[strings.ToLower(strings.TrimSpace(normalizeNodeRef(task)))]
		if !ok {
			candidate, ok = byRef[strings.ToLower(strings.TrimSpace(task.FlowID))]
		}
		if ok {
			task.Params = mergeUserMessageSlots(task.Params, candidate, taskAction(task))
		}
		plan.Tasks[i] = task
	}
	return plan
}

func mergePendingTaskParams(ctx context.Context, task flowschema.PlanTask) flowschema.PlanTask {
	if ctx == nil {
		return task
	}
	pending := mapFromAny(ctx.Value("agent_pending_task"))
	if len(pending) == 0 {
		return task
	}
	pendingRef := firstNonEmpty(anyToString(pending["node_ref"]), anyToString(pending["skill_id"]), anyToString(pending["capability_id"]))
	taskRef := normalizeNodeRef(task)
	if pendingRef != "" && taskRef != "" && !strings.EqualFold(pendingRef, taskRef) {
		return task
	}
	pendingAction := strings.ToLower(strings.TrimSpace(anyToString(pending["action"])))
	taskActionValue := taskAction(task)
	if pendingAction != "" && taskActionValue != "" && pendingAction != taskActionValue {
		return task
	}
	merged := clonePlanParams(task.Params)
	if merged == nil {
		merged = map[string]interface{}{}
	}
	if taskActionValue == "" && pendingAction != "" {
		merged["action"] = pendingAction
	}
	if collected := mapFromAny(pending["collected_params"]); len(collected) > 0 {
		mergePlanParams(merged, collected)
	}
	applyPendingConfirmationParams(pending, merged)
	task.Params = merged
	return task
}

func pendingDetectedTaskFromContext(ctx context.Context, msg string) (flowschema.DetectedTask, bool) {
	if ctx == nil {
		return flowschema.DetectedTask{}, false
	}
	pending := mapFromAny(ctx.Value("agent_pending_task"))
	if !pendingTaskStatusAwaitingParams(pending) {
		return flowschema.DetectedTask{}, false
	}
	nodeRef := firstNonEmpty(anyToString(pending["node_ref"]), anyToString(pending["skill_id"]), anyToString(pending["capability_id"]))
	if strings.TrimSpace(nodeRef) == "" {
		return flowschema.DetectedTask{}, false
	}
	nodeKind := firstNonEmpty(anyToString(pending["node_kind"]), dto.NodeKindSkill)
	params := map[string]interface{}{}
	if collected := mapFromAny(pending["collected_params"]); len(collected) > 0 {
		mergePlanParams(params, collected)
	}
	if action := strings.ToLower(strings.TrimSpace(anyToString(pending["action"]))); action != "" {
		params["action"] = action
	}
	params["_node_kind"] = nodeKind
	params["_node_ref"] = nodeRef
	params["_source_scope"] = firstNonEmpty(anyToString(pending["source_scope"]), "agent")
	params["_candidate_name"] = firstNonEmpty(anyToString(pending["candidate_name"]), nodeRef)
	params["_pending_task_id"] = anyToString(pending["task_id"])
	params["_pending_trace_id"] = anyToString(pending["trace_id"])
	if userText := strings.TrimSpace(msg); userText != "" {
		params["user_message"] = userText
	}
	applyPendingConfirmationParams(pending, params)
	return flowschema.DetectedTask{
		TaskID:   firstNonEmpty(anyToString(pending["task_id"]), fmt.Sprintf("pending_%d", time.Now().UnixNano())),
		FlowID:   nodeRef,
		AgentID:  anyToString(pending["agent_id"]),
		Score:    1,
		Strategy: "pending_task_resume:" + strings.TrimSpace(nodeKind),
		Reason:   "resume awaiting-params task with latest user message",
		Params:   params,
	}, true
}

func applyPendingConfirmationParams(pending map[string]any, params map[string]interface{}) {
	if len(pending) == 0 || params == nil {
		return
	}
	if !pendingMissingField(pending, "confirmation") && !pendingMissingField(pending, "confirmed") {
		return
	}
	userText := strings.TrimSpace(firstNonEmpty(anyToString(params["user_message"]), anyToString(params["message"])))
	if !isAffirmativeConfirmationText(userText) {
		return
	}
	if !hasPlanParamPath(params, "confirmation") {
		params["confirmation"] = userText
	}
	if !hasPlanParamPath(params, "confirmed") {
		params["confirmed"] = true
	}
}

func pendingMissingField(pending map[string]any, field string) bool {
	field = strings.ToLower(strings.TrimSpace(field))
	if len(pending) == 0 || field == "" {
		return false
	}
	for _, missing := range anyStringSlice(pending["missing_fields"]) {
		if strings.ToLower(strings.TrimSpace(missing)) == field {
			return true
		}
	}
	if state := mapFromAny(pending["state"]); len(state) > 0 {
		for _, missing := range anyStringSlice(state["missing"]) {
			if strings.ToLower(strings.TrimSpace(missing)) == field {
				return true
			}
		}
	}
	return false
}

func isAffirmativeConfirmationText(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	text = strings.Trim(text, " \t\r\n。.!！,，;；")
	switch text {
	case "确认", "确认删除", "确定", "确定删除", "同意", "同意删除", "是", "是的", "可以", "执行", "执行删除", "删除吧", "删吧", "yes", "y", "ok", "okay", "confirm", "confirmed":
		return true
	default:
		return false
	}
}

func isPendingResumePlan(ctx context.Context, plan *flowschema.ExecutionPlan) bool {
	if ctx == nil || plan == nil || len(plan.Tasks) == 0 {
		return false
	}
	pending := mapFromAny(ctx.Value("agent_pending_task"))
	if !pendingTaskStatusAwaitingParams(pending) {
		return false
	}
	for _, task := range plan.Tasks {
		if task.Params == nil {
			continue
		}
		if strings.TrimSpace(anyToString(task.Params["_pending_task_id"])) != "" ||
			strings.TrimSpace(anyToString(task.Params["user_message"])) != "" {
			return true
		}
	}
	return false
}

func pendingTaskStatusAwaitingParams(task map[string]any) bool {
	if len(task) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(anyToString(task["status"])), dto.AgentTaskStatusAwaitingParams)
}

func pendingTaskPayloadForPlan(ctx context.Context, tr *traceRuntime, plan *flowschema.ExecutionPlan, missing []string) map[string]any {
	payload := map[string]any{
		"missing_fields": missing,
		"status":         dto.AgentTaskStatusAwaitingParams,
	}
	if tr != nil {
		payload["run_id"] = tr.meta.RunID
		payload["session_id"] = tr.meta.SessionID
		payload["message_id"] = tr.meta.MessageID
		payload["trace_id"] = tr.meta.TraceID
		payload["plan_id"] = tr.meta.PlanID
		payload["agent_id"] = tr.meta.AgentID
	}
	if plan != nil && len(plan.Tasks) > 0 {
		task := mergePendingTaskParams(ctx, plan.Tasks[0])
		payload["task_id"] = task.TaskID
		payload["node_kind"] = normalizeNodeKind(task.NodeKind)
		payload["node_ref"] = normalizeNodeRef(task)
		payload["skill_id"] = skillIDFromPlanTask(task)
		if task.Params != nil {
			payload["capability_id"] = strings.TrimSpace(fmt.Sprint(task.Params["capability_id"]))
		}
		payload["action"] = taskAction(task)
		payload["collected_params"] = clonePlanParams(task.Params)
	}
	return payload
}

func skillIDFromPlanTask(task flowschema.PlanTask) string {
	if normalizeNodeKind(task.NodeKind) == dto.NodeKindSkill {
		return normalizeNodeRef(task)
	}
	return ""
}

func clonePlanParams(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		if nested, ok := v.(map[string]interface{}); ok {
			out[k] = clonePlanParams(nested)
			continue
		}
		out[k] = v
	}
	return out
}

func mergePlanParams(dst map[string]interface{}, src map[string]any) {
	for k, v := range src {
		if strings.TrimSpace(k) == "" || isEmptyPlanParam(v) {
			continue
		}
		if existing, ok := dst[k].(map[string]interface{}); ok {
			if nested := mapFromAny(v); len(nested) > 0 {
				mergePlanParams(existing, nested)
				continue
			}
		}
		if nested := mapFromAny(v); len(nested) > 0 {
			child := map[string]interface{}{}
			mergePlanParams(child, nested)
			dst[k] = child
			continue
		}
		dst[k] = v
	}
}

func mergeUserMessageSlots(params map[string]interface{}, candidate agent.ToolCallCandidate, action string) map[string]interface{} {
	if params == nil {
		params = map[string]interface{}{}
	}
	userText := strings.TrimSpace(anyToString(params["user_message"]))
	action = strings.ToLower(strings.TrimSpace(action))
	if userText == "" || action == "" {
		return params
	}
	required := candidate.ActionRequiredArgs[action]
	if len(required) == 0 {
		return params
	}
	for _, field := range required {
		field = strings.TrimSpace(field)
		if field == "" || hasPlanParamPath(params, field) {
			continue
		}
		value := extractSlotValueFromText(userText, slotLabelsForField(field, candidate.SlotMapping))
		if value == "" {
			continue
		}
		setPlanParamPath(params, field, value)
	}
	return params
}

func slotLabelsForField(field string, mapping map[string]any) []string {
	labels := []string{}
	if strings.TrimSpace(field) != "" {
		labels = append(labels, strings.TrimSpace(field))
		parts := strings.Split(field, ".")
		if len(parts) > 0 {
			labels = append(labels, strings.TrimSpace(parts[len(parts)-1]))
		}
	}
	if raw, ok := mapping[field]; ok {
		if m := mapFromAny(raw); len(m) > 0 {
			labels = append(labels, anyStringSlice(m["labels"])...)
		}
	}
	return normalizeStringList(labels)
}

func extractSlotValueFromText(text string, labels []string) string {
	text = strings.TrimSpace(text)
	if text == "" || len(labels) == 0 {
		return ""
	}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		for _, sep := range []string{"可以是", "可以为", "就是", "是", "为", ":", "：", "="} {
			token := label + sep
			idx := strings.Index(text, token)
			if idx < 0 {
				token = label + " " + sep
				idx = strings.Index(text, token)
			}
			if idx < 0 {
				continue
			}
			value := strings.TrimSpace(text[idx+len(token):])
			value = trimSlotValueBoundary(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func trimSlotValueBoundary(value string) string {
	value = strings.TrimSpace(value)
	if quoted := leadingQuotedValue(value); quoted != "" {
		return quoted
	}
	value = strings.Trim(value, "\"'“”‘’` ")
	for _, sep := range []string{"，", ",", "。", "；", ";", "\n"} {
		if idx := strings.Index(value, sep); idx >= 0 {
			value = strings.TrimSpace(value[:idx])
		}
	}
	return strings.Trim(value, "\"'“”‘’` ")
}

func leadingQuotedValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	pairs := map[rune]rune{
		'\'': '\'',
		'"':  '"',
		'`':  '`',
		'“':  '”',
		'‘':  '’',
	}
	runes := []rune(value)
	closeQuote, ok := pairs[runes[0]]
	if !ok {
		return ""
	}
	for i := 1; i < len(runes); i++ {
		if runes[i] == closeQuote {
			return strings.TrimSpace(string(runes[1:i]))
		}
	}
	return ""
}

func setPlanParamPath(params map[string]interface{}, path string, value interface{}) {
	path = strings.TrimSpace(path)
	if path == "" || params == nil {
		return
	}
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		params[path] = value
		return
	}
	cur := params
	for i, raw := range parts {
		key := strings.TrimSpace(raw)
		if key == "" {
			return
		}
		if i == len(parts)-1 {
			cur[key] = value
			return
		}
		next, ok := cur[key].(map[string]interface{})
		if !ok || next == nil {
			next = map[string]interface{}{}
			cur[key] = next
		}
		cur = next
	}
}

func taskAction(task flowschema.PlanTask) string {
	if task.Params == nil {
		return ""
	}
	for _, key := range []string{"action", "operation", "op"} {
		if v, ok := task.Params[key]; ok {
			return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
		}
	}
	return ""
}

func hasPlanParamPath(params map[string]interface{}, path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return true
	}
	if params == nil {
		return false
	}
	if v, ok := params[path]; ok {
		return !isEmptyPlanParam(v)
	}
	parts := strings.Split(path, ".")
	var cur interface{} = params
	for _, raw := range parts {
		key := strings.TrimSpace(raw)
		if key == "" {
			return false
		}
		obj, ok := cur.(map[string]interface{})
		if !ok {
			return false
		}
		next, ok := obj[key]
		if !ok {
			lowerKey := strings.ToLower(key)
			for candidateKey, candidateValue := range obj {
				if strings.ToLower(strings.TrimSpace(candidateKey)) == lowerKey {
					next = candidateValue
					ok = true
					break
				}
			}
		}
		if !ok {
			return false
		}
		cur = next
	}
	return !isEmptyPlanParam(cur)
}

func isEmptyPlanParam(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []interface{}:
		return len(x) == 0
	case map[string]interface{}:
		return len(x) == 0
	default:
		return false
	}
}

func ensureResponsePlanForClarify(ctx context.Context, plan *ResponsePlan, missing []string) *ResponsePlan {
	if plan == nil {
		plan = &ResponsePlan{}
	}
	if plan.TenantUUID == "" {
		plan.TenantUUID = strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	}
	plan.ResponseMode = ResponseModeClarifyParams
	plan.ResponseIntents = append(plan.ResponseIntents, ResponseIntentClarifyParams)
	plan.AnswerRequirements = append(plan.AnswerRequirements, answerRequirementsForMode(ResponseModeClarifyParams)...)
	plan.ShouldCallTool = false
	plan.UseCapabilityCtx = true
	plan.IncludeExamples = true
	plan.IncludeSchema = true
	plan.NeedsClarification = true
	plan.MissingFields = normalizeStringList(append(plan.MissingFields, missing...))
	plan.Reason = "plan has action-required args missing"
	if plan.ResponsePlanID == "" {
		plan.ResponsePlanID = fmt.Sprintf("rp_%d", time.Now().UnixNano())
	}
	return plan
}

func PickFirstFlowID(plan *flowschema.ExecutionPlan) string {
	if plan == nil || len(plan.Tasks) == 0 {
		return ""
	}
	tasks := make([]flowschema.PlanTask, len(plan.Tasks))
	copy(tasks, plan.Tasks)

	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Stage == tasks[j].Stage {
			return i < j
		}
		return tasks[i].Stage < tasks[j].Stage
	})

	if id := strings.TrimSpace(tasks[0].FlowID); id != "" {
		return id
	}
	if id := strings.TrimSpace(tasks[0].TaskID); id != "" {
		return id
	}
	return ""
}

func normalizeNodeKind(kind string) string {
	if strings.EqualFold(strings.TrimSpace(kind), dto.NodeKindObservation) {
		return dto.NodeKindObservation
	}
	k := strings.ToLower(strings.TrimSpace(kind))
	if k == "" {
		return dto.NodeKindWorkflow
	}
	return k
}

func normalizeNodeRef(task flowschema.PlanTask) string {
	if s := strings.TrimSpace(task.NodeRef); s != "" {
		return s
	}
	return strings.TrimSpace(task.FlowID)
}

func normalizeSourceScope(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "" {
		return "system"
	}
	return s
}

func normalizeCandidateName(task flowschema.PlanTask) string {
	if task.Params == nil {
		return ""
	}
	if v, ok := task.Params["_candidate_name"]; ok {
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return ""
}

func normalizeCandidateDesc(task flowschema.PlanTask) string {
	if task.Params == nil {
		return ""
	}
	if v, ok := task.Params["_candidate_desc"]; ok {
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return ""
}

func ExtractAssistantText(chunk *agentschema.ExecutionResult) string {
	if chunk == nil || chunk.Data == nil {
		return ""
	}
	if res, ok := chunk.Data["result"].(map[string]any); ok {
		if s, ok := res["content"].(string); ok {
			return SanitizeAssistantVisibleText(s)
		}
	}
	if s, ok := chunk.Data["content"].(string); ok {
		return SanitizeAssistantVisibleText(s)
	}
	return ""
}

func enrichTaskEndPayload(payload map[string]any, out *agentschema.ExecutionResult) {
	if payload == nil || out == nil || out.Data == nil {
		return
	}
	result, _ := out.Data["result"].(map[string]any)
	if len(result) > 0 {
		payload["result"] = result
		if links, ok := result["links"]; ok {
			payload["links"] = links
		}
		for _, key := range []string{"content", "message", "summary"} {
			if value := strings.TrimSpace(anyToString(result[key])); value != "" {
				payload["message"] = value
				break
			}
		}
	}
	if payload["message"] == nil {
		if content := strings.TrimSpace(ExtractAssistantText(out)); content != "" {
			payload["message"] = content
		}
	}
}

func taskExecutionError(out *agentschema.ExecutionResult, runErr error) error {
	if runErr != nil {
		return runErr
	}
	if out == nil || out.Success {
		return nil
	}
	message := firstNonEmpty(
		strings.TrimSpace(out.Error),
		strings.TrimSpace(anyToString(out.Data["error"])),
		strings.TrimSpace(anyToString(out.Metadata["error"])),
		"agent_task_unsuccessful",
	)
	return errors.New(message)
}

func buildFinalContent(out *agentschema.ExecutionResult) string {
	if out == nil {
		return ""
	}
	if out.Data != nil {
		if result, ok := out.Data["result"].(map[string]any); ok {
			if envelope, err := responseEnvelopeFromResult(result); err == nil && envelope != nil {
				return ""
			}
		}
		if envelope, err := responseEnvelopeFromResult(out.Data); err == nil && envelope != nil {
			return ""
		}
	}
	if s := strings.TrimSpace(ExtractAssistantText(out)); s != "" {
		return s
	}
	data := out.Data
	if data == nil {
		return ""
	}
	skillID := strings.TrimSpace(anyToString(data["skill_id"]))
	status := strings.TrimSpace(anyToString(data["status"]))
	protocol := strings.TrimSpace(anyToString(data["protocol_used"]))
	if status == "" {
		status = "completed"
	}
	if skillID != "" {
		if protocol != "" {
			return fmt.Sprintf("任务已执行完成（skill=%s，protocol=%s，status=%s）。", skillID, protocol, status)
		}
		return fmt.Sprintf("任务已执行完成（skill=%s，status=%s）。", skillID, status)
	}
	return fmt.Sprintf("任务已执行完成（status=%s）。", status)
}

func anyToString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func humanizeExecutionError(err error) string {
	if err == nil {
		return "执行失败，请稍后重试。"
	}
	if errors.Is(err, context.Canceled) {
		return "执行失败：任务在并行阶段被取消，请重试一次。"
	}
	return "执行失败，请稍后重试。"
}

func taskFailureReason(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "run.canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "run.timeout"
	default:
		return "execution.task_failed"
	}
}

func agentRunFailurePayload(ctx context.Context, flowID, reason string, err error, partial string) map[string]any {
	code := "agent_run.failed"
	retryable := false
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			code = "agent_run.canceled"
			retryable = true
		case errors.Is(err, context.DeadlineExceeded):
			code = "agent_run.timeout"
			retryable = true
		case strings.Contains(strings.ToLower(err.Error()), "without final response"):
			code = "agent_run.missing_final"
		}
	}
	outcome := runtimeFailureOutcome(strings.TrimSpace(reason), err)
	payload := map[string]any{
		"success":     false,
		"code":        code,
		"reason":      strings.TrimSpace(reason),
		"retryable":   retryable,
		"flow_id":     strings.TrimSpace(flowID),
		"trace_id":    strings.TrimSpace(reqctx.GetTraceID(ctx)),
		"outcome":     outcome.Status,
		"reason_code": outcome.ReasonCode,
		"error":       map[string]any{"code": code, "reason": strings.TrimSpace(reason), "retryable": retryable},
		"metadata":    map[string]any{"terminal": true, "failure_code": code, "failure_reason": strings.TrimSpace(reason), "outcome": outcome.Status, "reason_code": outcome.ReasonCode},
		"created_at":  time.Now().UTC().Format(time.RFC3339Nano),
	}
	if content := strings.TrimSpace(SanitizeAssistantVisibleText(partial)); content != "" {
		payload["partial_content"] = content
		if errMap, ok := payload["error"].(map[string]any); ok {
			errMap["partial_content"] = content
		}
	}
	return payload
}

func emitAgentRunFailure(ctx context.Context, sink EventSink, flowID, reason, message string, err error, partial string) {
	payload := agentRunFailurePayload(ctx, flowID, reason, err, partial)
	if strings.TrimSpace(message) != "" {
		payload["message"] = strings.TrimSpace(message)
		if errMap, ok := payload["error"].(map[string]any); ok {
			errMap["message"] = strings.TrimSpace(message)
		}
	}
	_ = sink.Emit(dto.EventError, payload)
	_ = sink.Emit(dto.EventEnd, agentRunEndFailurePayload(payload))
}

func agentRunEndFailurePayload(errorPayload map[string]any) map[string]any {
	out := map[string]any{"success": false}
	for _, key := range []string{"code", "reason", "message", "retryable", "flow_id", "trace_id", "outcome", "reason_code", "partial_content", "error", "metadata"} {
		if v, ok := errorPayload[key]; ok {
			out[key] = v
		}
	}
	return out
}

func responsePlanFromContext(ctx context.Context) *ResponsePlan {
	if ctx == nil {
		return nil
	}
	raw := ctx.Value("agent_response_plan")
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case *ResponsePlan:
		return v
	case ResponsePlan:
		cp := v
		return &cp
	case map[string]any:
		b, _ := json.Marshal(v)
		var plan ResponsePlan
		if err := json.Unmarshal(b, &plan); err == nil && plan.ResponseMode != "" {
			return &plan
		}
	}
	return nil
}

func responseContextLayersFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	switch v := ctx.Value("agent_response_context_layers").(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func modelSelectionFromContext(ctx context.Context, node ModelPolicyNode) NodeModelSelection {
	if ctx == nil {
		return NodeModelSelection{Node: node}
	}
	if policy, ok := ctx.Value("agent_node_model_policy").(NodeModelPolicy); ok {
		return policy.Selection(node)
	}
	if policy, ok := ctx.Value("agent_node_model_policy").(*NodeModelPolicy); ok && policy != nil {
		return policy.Selection(node)
	}
	return NodeModelSelection{Node: node}
}

func mergeResponseMetadata(meta map[string]any, plan *ResponsePlan, contextLayers []string, selection NodeModelSelection) map[string]any {
	if meta == nil {
		meta = map[string]any{}
	}
	if plan == nil {
		return meta
	}
	plan.ModelSelection = selection
	meta["response_plan"] = plan.ToDebugEvent()
	meta["response_mode"] = plan.ResponseMode
	meta["target_capability_ids"] = plan.TargetCapabilityIDs
	meta["capability_ids"] = plan.TargetCapabilityIDs
	meta["response_plan_id"] = plan.ResponsePlanID
	meta["used_context_layers"] = contextLayers
	meta["final_response_model"] = strings.TrimSpace(selection.Model)
	meta["model_selection"] = selection
	return meta
}

func mergeFinalContent(data flowschema.Result, content string) flowschema.Result {
	if data == nil {
		data = flowschema.Result{}
	}
	if strings.TrimSpace(content) == "" {
		return data
	}
	data["content"] = content
	if nested, ok := data["result"].(map[string]any); ok {
		nested["content"] = content
		data["result"] = nested
	}
	return data
}

func responseModeString(plan *ResponsePlan) string {
	if plan == nil || plan.ResponseMode == "" {
		return string(ResponseModeNormalChat)
	}
	return string(plan.ResponseMode)
}

func responseTargetIDs(plan *ResponsePlan) []string {
	if plan == nil {
		return nil
	}
	return append([]string(nil), plan.TargetCapabilityIDs...)
}

func markResponsePlanExecutable(plan *ResponsePlan, execPlan *flowschema.ExecutionPlan) {
	if plan == nil || execPlan == nil || len(execPlan.Tasks) == 0 {
		return
	}
	plan.ShouldCallTool = true
	if plan.ResponseMode == "" || plan.ResponseMode == ResponseModeNormalChat || plan.ResponseMode == ResponseModeClarifyParams {
		plan.ResponseMode = ResponseModeSkillExecution
		plan.NeedsClarification = false
		plan.MissingFields = nil
		plan.Reason = "planner produced executable task"
	}
	if plan.ResponsePlanID == "" {
		plan.ResponsePlanID = fmt.Sprintf("rp_%d", time.Now().UnixNano())
	}
}
