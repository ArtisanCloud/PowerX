package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	agenttrace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type EventSink interface {
	Emit(event string, payload any) error
}

type Engine struct {
	mgr *agent.Manager
}

func NewEngine() *Engine { return &Engine{mgr: agent.GetAgentManager()} }

func (e *Engine) detectTasks(ctx context.Context, msg string, reqCfg *dto.ChatConfig) ([]flowschema.DetectedTask, error) {
	if task, ok := pendingDetectedTaskFromContext(ctx, msg); ok {
		return []flowschema.DetectedTask{task}, nil
	}
	return e.mgr.DetectTasksWithToolCalling(ctx, msg, reqCfg)
}

func (e *Engine) Run(ctx context.Context, msg string, reqCfg *dto.ChatConfig, explicitFlow string, sink EventSink) error {
	// 统一超时：避免 LLM/下游卡死导致“前端永远生成中”
	// - 目标体验是“不断开连接、持续可见”，但也要有上限兜底（默认 10 分钟）。
	execTimeout := 10 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	tr, traceErr := e.newTraceRuntime(ctx, msg, reqCfg, explicitFlow, "engine.stream")
	if traceErr != nil {
		return sink.Emit(dto.EventError, map[string]any{"message": "Agent Trace 初始化失败", "detail": traceErr.Error()})
	}
	if rs, ok := sink.(*RunStateSink); ok {
		rs.SetRunStateRecorder(func(event string, payload any) {
			tr.appendRunStateEvent(ctx, event, payload)
		})
	}
	runStatus := agenttrace.RunStatusCompleted
	var runErr error
	var finalText string
	defer func() {
		if runErr != nil {
			runStatus = agenttrace.RunStatusFailed
		}
		tr.complete(ctx, runStatus, finalText, runErr)
	}()
	receiveNode := tr.startNode(ctx, "receive_message", "agent.stream", map[string]any{"message_digest": digestString(msg)})
	tr.endNode(ctx, receiveNode, "receive_message", "agent.stream", map[string]any{"accepted": true})
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

	intentNode := tr.startNode(ctx, "intent_recognition", "DetectTasksWithToolCalling", nil)
	// 1) 多意图识别
	tasks, err := e.detectTasks(ctx, msg, reqCfg)
	if err != nil {
		tr.failNode(ctx, intentNode, "intent_recognition", "DetectTasksWithToolCalling", err)
		runErr = err
		return sink.Emit(dto.EventError, map[string]any{"message": "意图识别失败", "detail": err.Error()})
	}
	tr.endNode(ctx, intentNode, "intent_recognition", "DetectTasksWithToolCalling", map[string]any{"task_count": len(tasks)})
	_ = sink.Emit(dto.EventIntent, map[string]any{"mode": "intent_multi", "planner_mode": dto.PlannerModeUnified, "tasks": tasks})

	plannerNode := tr.startNode(ctx, "planner", "BuildPlan", map[string]any{"task_count": len(tasks)})
	// 2) 生成计划（强/弱类型兼容）
	rawPlan := e.mgr.BuildPlan(tasks) // FIX: 原来写成了 BuildPl
	plan, ok := NormalizeExecPlan(rawPlan)
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
		_ = sink.Emit(dto.EventError, map[string]any{"message": "执行计划生成失败", "detail": err.Error()})
		_ = sink.Emit(dto.EventEnd, map[string]any{"success": false})
		return err
	}
	// skill/tooling 等非 workflow 节点必须走统一 Plan 执行链路，
	// 不能再把 node_ref 当 flow_id 交给 ag.Stream。
	if planHasNonWorkflow(plan) {
		if shouldExecutePlanForResponse(responsePlan, plan) {
			_, err := e.runResolvedPlan(ctx, plan, sink, tr)
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
		return sink.Emit(dto.EventError, map[string]any{"message": "创建 Agent 失败", "detail": err.Error()})
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
		Timeout:    execTimeout,
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
		return sink.Emit(dto.EventError, map[string]any{"message": "流式聊天执行失败", "detail": err.Error()})
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

	for {
		select {
		case <-ctx.Done():
			tr.failNode(ctx, streamNode, "llm_call", flowID, ctx.Err())
			runErr = ctx.Err()
			_ = sink.Emit(dto.EventError, map[string]any{"message": "请求超时或已取消", "detail": ctx.Err().Error()})
			_ = sink.Emit(dto.EventEnd, map[string]any{"success": false})
			return ctx.Err()
		case <-hb.C:
			// 心跳：让前端/网关确认连接仍然活着（前端可选择忽略，仅用于避免“看似挂死”）。
			_ = sink.Emit(dto.EventHeartbeat, map[string]any{
				"ts":           time.Now().UTC().Unix(),
				"idle_seconds": int(time.Since(lastRecvAt).Seconds()),
			})
		case it, ok := <-recvCh:
			if !ok {
				tr.endNode(ctx, streamNode, "llm_call", flowID, map[string]any{"eof": true})
				_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
				return nil
			}
			if it.err != nil {
				if errors.Is(it.err, io.EOF) {
					tr.endNode(ctx, streamNode, "llm_call", flowID, map[string]any{"eof": true})
					_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
					return nil
				}
				tr.failNode(ctx, streamNode, "llm_call", flowID, it.err)
				runErr = it.err
				_ = sink.Emit(dto.EventError, map[string]any{"message": it.err.Error()})
				_ = sink.Emit(dto.EventEnd, map[string]any{"success": false})
				return it.err
			}
			lastRecvAt = time.Now()
			ch := it.ch
			if ch == nil {
				continue
			}

			if delta, ok := ch.Metadata["delta_text"].(string); ok && delta != "" {
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

func (e *Engine) RunPlanInvoke(ctx context.Context, msg string, reqCfg *dto.ChatConfig, explicitFlow string, sink EventSink) (*agentschema.ExecutionResult, *flowschema.ExecutionPlan, error) {
	execTimeout := 10 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	tr, traceErr := e.newTraceRuntime(ctx, msg, reqCfg, explicitFlow, "engine.invoke")
	if traceErr != nil {
		_ = sink.Emit(dto.EventError, map[string]any{"message": "Agent Trace 初始化失败", "detail": traceErr.Error()})
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

	intentNode := tr.startNode(ctx, "intent_recognition", "DetectTasksWithToolCalling", nil)
	tasks, err := e.detectTasks(ctx, msg, reqCfg)
	if err != nil {
		tr.failNode(ctx, intentNode, "intent_recognition", "DetectTasksWithToolCalling", err)
		runErr = err
		_ = sink.Emit(dto.EventError, map[string]any{"message": "意图识别失败", "detail": err.Error()})
		_ = sink.Emit(dto.EventEnd, map[string]any{"success": false})
		return nil, nil, err
	}
	tr.endNode(ctx, intentNode, "intent_recognition", "DetectTasksWithToolCalling", map[string]any{"task_count": len(tasks)})
	_ = sink.Emit(dto.EventIntent, map[string]any{"mode": "intent_multi", "planner_mode": dto.PlannerModeUnified, "tasks": tasks})

	plannerNode := tr.startNode(ctx, "planner", "BuildPlan", map[string]any{"task_count": len(tasks)})
	rawPlan := e.mgr.BuildPlan(tasks)
	plan, ok := NormalizeExecPlan(rawPlan)
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
		_ = sink.Emit(dto.EventError, map[string]any{"message": "执行计划生成失败", "detail": err.Error()})
		_ = sink.Emit(dto.EventEnd, map[string]any{"success": false})
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
		Timeout:    execTimeout,
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
			_ = sink.Emit(dto.EventError, map[string]any{"message": "执行失败", "detail": dispatchErr.Error()})
			_ = sink.Emit(dto.EventEnd, map[string]any{"success": false})
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
	finalText = BuildFinalResponseContent(responsePlan, buildFinalContent(out), nil)
	return out, plan, nil
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
	if plan == nil || len(plan.Tasks) == 0 {
		return nil, fmt.Errorf("empty plan")
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
		Timeout:    10 * time.Minute,
		Metadata: map[string]any{
			"transport": "engine.invoke",
			"env":       strings.TrimSpace(reqctx.GetEnv(ctx)),
		},
	})

	emitMu := &sync.Mutex{}
	hooks := &agent.PlanExecutionHooks{
		OnTaskStart: func(task flowschema.PlanTask) error {
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
			status := "completed"
			if runErr != nil {
				status = "failed"
			} else if out != nil && isAwaitingParamsResult(out) {
				status = dto.AgentTaskStatusAwaitingParams
			}
			if err := persistTaskSkillState(ctx, task, status, out, runErr); err != nil {
				_ = sink.Emit(dto.EventError, map[string]any{"message": "保存 Skill 状态失败", "detail": err.Error()})
				return err
			}
			_ = sink.Emit(dto.EventNodeEnd, map[string]any{
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
					if runErr == nil {
						return ""
					}
					return runErr.Error()
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
			})
			if status == dto.AgentTaskStatusAwaitingParams {
				_ = sink.Emit(dto.EventAgentRunAwaitingParams, awaitingPayloadFromResult(task, out))
			}
			return nil
		},
	}

	out, execErr := e.mgr.ExecutePlanWithHooks(ctx, *plan, meta, hooks)
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
			"metadata": mergeResponseMetadata(mergeTraceMetadata(map[string]any{
				"trace_id": traceID,
				"plan_id":  plan.PlanID,
			}, tr), responsePlan, contextLayers, modelSelectionFromContext(ctx, ModelPolicyNodeErrorExplain)),
		})
		tr.endNode(ctx, finalNode, "final_response", "plan.error", map[string]any{
			"success":               false,
			"content_digest":        digestString(userMsg),
			"response_mode":         responseModeString(responsePlan),
			"target_capability_ids": responseTargetIDs(responsePlan),
			"used_context_layers":   contextLayers,
		})
		_ = sink.Emit(dto.EventError, map[string]any{"message": "执行失败", "detail": execErr.Error(), "plan_id": plan.PlanID, "user_message": userMsg})
		_ = sink.Emit(dto.EventEnd, map[string]any{"success": false})
		return nil, execErr
	}
	if isAwaitingParamsResult(out) {
		responsePlan := responsePlanFromContext(ctx)
		responsePlan = ensureResponsePlanForClarify(ctx, responsePlan, stringSliceFromAny(resultValue(out, "missing_fields")))
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
			"data": map[string]any{
				"content": userMsg,
			},
			"metadata": mergeResponseMetadata(mergeTraceMetadata(map[string]any{
				"trace_id": traceID,
				"plan_id":  plan.PlanID,
			}, tr), responsePlan, contextLayers, modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse)),
		})
		tr.endNode(ctx, finalNode, "final_response", "plan.awaiting_params", map[string]any{
			"content_digest":        digestString(userMsg),
			"response_mode":         responseModeString(responsePlan),
			"target_capability_ids": responseTargetIDs(responsePlan),
			"used_context_layers":   contextLayers,
		})
		_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
		return out, nil
	}
	responsePlan := responsePlanFromContext(ctx)
	content := BuildFinalResponseContent(responsePlan, buildFinalContent(out), nil)
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
		"data": map[string]any{
			"content": content,
		},
		"metadata": mergeResponseMetadata(mergeTraceMetadata(map[string]any{
			"trace_id": traceID,
			"plan_id":  plan.PlanID,
		}, tr), responsePlan, contextLayers, modelSelectionFromContext(ctx, ModelPolicyNodeFinalResponse)),
	})
	tr.endNode(ctx, finalNode, "final_response", "plan.final", map[string]any{
		"content_digest":        digestString(content),
		"response_mode":         responseModeString(responsePlan),
		"target_capability_ids": responseTargetIDs(responsePlan),
		"used_context_layers":   contextLayers,
	})
	_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
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
	value = strings.Trim(value, "\"'“”‘’` ")
	for _, sep := range []string{"，", ",", "。", "；", ";", "\n"} {
		if idx := strings.Index(value, sep); idx >= 0 {
			value = strings.TrimSpace(value[:idx])
		}
	}
	return strings.Trim(value, "\"'“”‘’` ")
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

func buildFinalContent(out *agentschema.ExecutionResult) string {
	if out == nil {
		return ""
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
	raw := strings.TrimSpace(err.Error())
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "skill record not found"):
		return "执行失败：命中的技能记录不存在（可能是旧 skill ID）。请刷新技能列表后重试。"
	case strings.Contains(lower, "context canceled"):
		return "执行失败：任务在并行阶段被取消，请重试一次。"
	default:
		if raw == "" {
			return "执行失败，请稍后重试。"
		}
		return "执行失败：" + raw
	}
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
