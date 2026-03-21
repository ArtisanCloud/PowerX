package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
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

func (e *Engine) Run(ctx context.Context, msg string, reqCfg *dto.ChatConfig, explicitFlow string, sink EventSink) error {
	// 统一超时：避免 LLM/下游卡死导致“前端永远生成中”
	// - 目标体验是“不断开连接、持续可见”，但也要有上限兜底（默认 10 分钟）。
	execTimeout := 10 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	// 1) 多意图识别
	tasks, err := e.mgr.DetectTasksWithToolCalling(ctx, msg, reqCfg)
	if err != nil {
		return sink.Emit(dto.EventError, map[string]any{"message": "意图识别失败", "detail": err.Error()})
	}
	_ = sink.Emit(dto.EventIntent, map[string]any{"mode": "intent_multi", "planner_mode": dto.PlannerModeUnified, "tasks": tasks})

	// 2) 生成计划（强/弱类型兼容）
	rawPlan := e.mgr.BuildPlan(tasks) // FIX: 原来写成了 BuildPl
	plan, ok := NormalizeExecPlan(rawPlan)
	_ = sink.Emit(dto.EventPlan, map[string]any{
		"planner_mode": dto.PlannerModeUnified,
		"plan":         PlanOrRaw(plan, rawPlan),
	})
	// skill/tooling 等非 workflow 节点必须走统一 Plan 执行链路，
	// 不能再把 node_ref 当 flow_id 交给 ag.Stream。
	if ok && planHasNonWorkflow(plan) {
		_, err := e.runResolvedPlan(ctx, plan, sink)
		return err
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

	execID := fmt.Sprintf("exec_%d", time.Now().UnixNano())
	_ = sink.Emit(dto.EventStart, map[string]any{"flow_id": flowID, "execution_id": execID})

	sr, err := ag.Stream(ctx, flowID, flowschema.Context{
		"message": msg,
		"config":  reqCfg,
	}, agentschema.ExecutionMeta{
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
	if err != nil {
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
				_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
				return nil
			}
			if it.err != nil {
				if errors.Is(it.err, io.EOF) {
					_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
					return nil
				}
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
				"data":      ch.Data,
				"step_id":   ch.StepID,
				"timestamp": ch.Timestamp,
				"metadata":  ch.Metadata,
			})

			if isFinal, _ := ch.Metadata["is_final"].(bool); isFinal {
				_ = sink.Emit(dto.EventFinal, map[string]any{
					"success":   ch.Success,
					"data":      ch.Data,
					"metadata":  ch.Metadata,
					"timestamp": ch.Timestamp,
				})
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

	tasks, err := e.mgr.DetectTasksWithToolCalling(ctx, msg, reqCfg)
	if err != nil {
		_ = sink.Emit(dto.EventError, map[string]any{"message": "意图识别失败", "detail": err.Error()})
		_ = sink.Emit(dto.EventEnd, map[string]any{"success": false})
		return nil, nil, err
	}
	_ = sink.Emit(dto.EventIntent, map[string]any{"mode": "intent_multi", "planner_mode": dto.PlannerModeUnified, "tasks": tasks})

	rawPlan := e.mgr.BuildPlan(tasks)
	plan, ok := NormalizeExecPlan(rawPlan)
	_ = sink.Emit(dto.EventPlan, map[string]any{
		"planner_mode": dto.PlannerModeUnified,
		"plan":         PlanOrRaw(plan, rawPlan),
	})

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
		_ = sink.Emit(dto.EventPlan, map[string]any{
			"planner_mode": dto.PlannerModeUnified,
			"plan":         plan,
		})
	}

	dispatchMeta := agentschema.ExecutionMeta{
		RequestID:  fmt.Sprintf("req_%d", time.Now().UnixNano()),
		UserID:     reqctx.GetUserID(ctx),
		TenantUUID: strings.TrimSpace(reqctx.GetTenantUUID(ctx)),
		TraceID:    strings.TrimSpace(reqctx.GetTraceID(ctx)),
		Timeout:    execTimeout,
		Metadata: map[string]any{
			"transport": "engine.invoke",
			"env":       strings.TrimSpace(reqctx.GetEnv(ctx)),
		},
	}
	if dispatchMeta.TraceID == "" {
		dispatchMeta.TraceID = fmt.Sprintf("trace_%d", time.Now().UnixNano())
	}

	if !ok || plan == nil || len(plan.Tasks) == 0 {
		out, _, dispatchErr := e.mgr.Dispatch(ctx, msg, flowschema.Context{
			"message": msg,
			"config":  reqCfg,
		}, dispatchMeta)
		if dispatchErr != nil {
			_ = sink.Emit(dto.EventError, map[string]any{"message": "执行失败", "detail": dispatchErr.Error()})
			_ = sink.Emit(dto.EventEnd, map[string]any{"success": false})
			return nil, nil, dispatchErr
		}
		content := buildFinalContent(out)
		traceID := fmt.Sprintf("trace_%d", time.Now().UnixNano())
		if raw := strings.TrimSpace(reqctx.GetTraceID(ctx)); raw != "" {
			traceID = raw
		}
		_ = sink.Emit(dto.EventFinal, map[string]any{
			"success": true,
			"data": map[string]any{
				"content": content,
			},
			"metadata": map[string]any{
				"trace_id": traceID,
				"plan_id":  "",
			},
		})
		_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
		return out, nil, nil
	}
	out, execErr := e.runResolvedPlan(ctx, plan, sink)
	if execErr != nil {
		return nil, plan, execErr
	}
	return out, plan, nil
}

func (e *Engine) runResolvedPlan(ctx context.Context, plan *flowschema.ExecutionPlan, sink EventSink) (*agentschema.ExecutionResult, error) {
	if plan == nil || len(plan.Tasks) == 0 {
		return nil, fmt.Errorf("empty plan")
	}
	traceID := fmt.Sprintf("trace_%d", time.Now().UnixNano())
	if raw := strings.TrimSpace(reqctx.GetTraceID(ctx)); raw != "" {
		traceID = raw
	}
	meta := agentschema.ExecutionMeta{
		RequestID:  fmt.Sprintf("req_%d", time.Now().UnixNano()),
		UserID:     reqctx.GetUserID(ctx),
		TenantUUID: strings.TrimSpace(reqctx.GetTenantUUID(ctx)),
		TraceID:    traceID,
		Timeout:    10 * time.Minute,
		Metadata: map[string]any{
			"transport": "engine.invoke",
			"env":       strings.TrimSpace(reqctx.GetEnv(ctx)),
		},
	}

	emitMu := &sync.Mutex{}
	hooks := &agent.PlanExecutionHooks{
		OnTaskStart: func(task flowschema.PlanTask) {
			emitMu.Lock()
			defer emitMu.Unlock()
			_ = sink.Emit(dto.EventNodeStart, map[string]any{
				"planner_mode": dto.PlannerModeUnified,
				"plan_id":      plan.PlanID,
				"task_id":      task.TaskID,
				"flow_id":      task.FlowID,
				"node_id":      task.TaskID,
				"node_kind":    normalizeNodeKind(task.NodeKind),
				"node_ref":     normalizeNodeRef(task),
				"node_name":    normalizeCandidateName(task),
				"node_desc":    normalizeCandidateDesc(task),
				"source_scope": normalizeSourceScope(task.SourceScope),
				"stage":        task.Stage,
				"depends_on":   task.DependsOn,
			})
		},
		OnTaskEnd: func(task flowschema.PlanTask, out *agentschema.ExecutionResult, runErr error) {
			emitMu.Lock()
			defer emitMu.Unlock()
			status := "completed"
			if runErr != nil {
				status = "failed"
			}
			_ = sink.Emit(dto.EventNodeEnd, map[string]any{
				"planner_mode": dto.PlannerModeUnified,
				"plan_id":      plan.PlanID,
				"task_id":      task.TaskID,
				"flow_id":      task.FlowID,
				"node_id":      task.TaskID,
				"node_kind":    normalizeNodeKind(task.NodeKind),
				"node_ref":     normalizeNodeRef(task),
				"node_name":    normalizeCandidateName(task),
				"node_desc":    normalizeCandidateDesc(task),
				"source_scope": normalizeSourceScope(task.SourceScope),
				"stage":        task.Stage,
				"depends_on":   task.DependsOn,
				"status":       status,
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
		},
	}

	out, execErr := e.mgr.ExecutePlanWithHooks(ctx, *plan, meta, hooks)
	if execErr != nil {
		userMsg := humanizeExecutionError(execErr)
		_ = sink.Emit(dto.EventError, map[string]any{"message": "执行失败", "detail": execErr.Error(), "plan_id": plan.PlanID, "user_message": userMsg})
		_ = sink.Emit(dto.EventFinal, map[string]any{
			"success": false,
			"data": map[string]any{
				"content": userMsg,
			},
			"metadata": map[string]any{
				"trace_id": traceID,
				"plan_id":  plan.PlanID,
			},
		})
		_ = sink.Emit(dto.EventEnd, map[string]any{"success": false})
		return nil, execErr
	}
	content := buildFinalContent(out)
	_ = sink.Emit(dto.EventFinal, map[string]any{
		"success": true,
		"data": map[string]any{
			"content": content,
		},
		"metadata": map[string]any{
			"trace_id": traceID,
			"plan_id":  plan.PlanID,
		},
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
			return s
		}
	}
	if s, ok := chunk.Data["content"].(string); ok {
		return s
	}
	return ""
}

func buildFinalContent(out *agentschema.ExecutionResult) string {
	if out == nil {
		return "任务已执行完成。"
	}
	if s := strings.TrimSpace(ExtractAssistantText(out)); s != "" {
		return s
	}
	data := out.Data
	if data == nil {
		return "任务已执行完成。"
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
