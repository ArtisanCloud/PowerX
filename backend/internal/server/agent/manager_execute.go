package agent

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	agenttrace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

type PlanExecutionHooks struct {
	OnTaskStart func(task flowschema.PlanTask)
	OnTaskEnd   func(task flowschema.PlanTask, out *aschema.ExecutionResult, err error)
}

/***************
 * Dispatch
 ***************/

// Dispatch：无意图时走默认 Agent + 默认 Flow（用于兜底聊天）
func (m *Manager) Dispatch(ctx context.Context, msg string, metaCtx flowschema.Context, mt aschema.ExecutionMeta) (*aschema.ExecutionResult, string, error) {
	ag, flowID, err := m.GetDefaultRoute()
	if err != nil {
		return nil, "", err
	}
	if metaCtx == nil {
		metaCtx = flowschema.Context{}
	}
	metaCtx["message"] = msg // eino 的兜底聊天会读取 message 与 model_config
	out, err := ag.Invoke(ctx, flowID, metaCtx, mt)
	if err != nil {
		return nil, "", err
	}
	return out, flowID, nil
}

/***********************
 * ExpandWithPrereqs
 ***********************/

// ExpandWithPrereqs 根据你的依赖策略补齐前置。
// 这里先返回原样；后续你可接一个全局 map[flowID][]flowID 来插入缺失前置并分配早期 Stage。
func (m *Manager) ExpandWithPreReqs(tasks []flowschema.DetectedTask) []flowschema.DetectedTask {
	if len(tasks) == 0 {
		return nil
	}

	m.mu.RLock()
	routes := make(map[string]routeRecord, len(m.routesByFlow))
	for k, v := range m.routesByFlow {
		routes[k] = v
	}
	m.mu.RUnlock()

	original := make(map[string]flowschema.DetectedTask, len(tasks))
	for _, t := range tasks {
		original[t.FlowID] = t
	}

	visited := make(map[string]bool)
	ordered := make([]flowschema.DetectedTask, 0, len(tasks))

	var visit func(string)
	visit = func(flowID string) {
		if flowID == "" || visited[flowID] {
			return
		}
		rec, ok := routes[flowID]
		if ok && rec.Spec != nil && rec.Spec.Metadata != nil {
			for _, dep := range rec.Spec.Metadata.Requires {
				visit(strings.TrimSpace(dep))
			}
		}
		visited[flowID] = true
		if orig, ok := original[flowID]; ok {
			ordered = append(ordered, orig)
			return
		}
		ordered = append(ordered, flowschema.DetectedTask{
			FlowID:   flowID,
			TaskID:   fmt.Sprintf("auto_%s", flowID),
			Score:    1,
			Strategy: "prereq",
		})
	}

	for _, t := range tasks {
		visit(t.FlowID)
	}
	return ordered
}

// ExpandWithPrereqs 兼容旧 API，委托给 ExpandWithPreReqs。
func (m *Manager) ExpandWithPrereqs(tasks []flowschema.DetectedTask) []flowschema.DetectedTask {
	return m.ExpandWithPreReqs(tasks)
}

/****************
 * ExecutePlan
 ****************/

func (m *Manager) ExecutePlan(ctx context.Context, plan flowschema.ExecutionPlan, mt aschema.ExecutionMeta) (*aschema.ExecutionResult, error) {
	return m.ExecutePlanWithHooks(ctx, plan, mt, nil)
}

func (m *Manager) ExecutePlanWithHooks(ctx context.Context, plan flowschema.ExecutionPlan, mt aschema.ExecutionMeta, hooks *PlanExecutionHooks) (*aschema.ExecutionResult, error) {
	if len(plan.Tasks) == 0 {
		return nil, errors.New("empty plan")
	}

	inSan := aschema.NewSanitizer(aschema.LogInputPolicy())
	outSan := aschema.NewSanitizer(aschema.ResultSummaryPolicy())

	// 计划级超时
	var cancel context.CancelFunc
	if mt.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, mt.Timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	results := aschema.NewResultStore() // 你自己的并发安全结果仓库
	tenantUUID := strings.TrimSpace(mt.TenantUUID)
	var tenantPtr *string
	if tenantUUID != "" {
		tenantPtr = &tenantUUID
	}
	userID := ""
	if mt.UserID > 0 {
		userID = strconv.FormatUint(mt.UserID, 10)
	}
	customerID := ""
	if mt.CustomerID > 0 {
		customerID = strconv.FormatUint(mt.CustomerID, 10)
	}

	// ---- 运行日志：PlanStart ----
	m.log().PlanStart(ctx, flow.AgentTaskEvent{
		PlanID:     plan.PlanID,
		Kind:       "plan.start",
		TenantUUID: tenantPtr,
		UserID:     userID,
		CustomerID: customerID,
		Ts:         time.Now(),
		Meta:       utils.J(map[string]any{"trace_id": mt.TraceID, "request_id": mt.RequestID}),
	})

	// Stage 分组并升序
	stageMap := map[int][]flowschema.PlanTask{}
	var stages []int
	for _, t := range plan.Tasks {
		stageMap[t.Stage] = append(stageMap[t.Stage], t)
	}
	for s := range stageMap {
		stages = append(stages, s)
	}
	sort.Ints(stages)

	var finalOut *aschema.ExecutionResult

	for _, s := range stages {
		stageTasks := stageMap[s]
		stageOut := make([]*aschema.ExecutionResult, len(stageTasks))

		eg, egCtx := errgroup.WithContext(ctx)
		for i := range stageTasks {
			i := i
			task := stageTasks[i]

			eg.Go(func() error {
				agID := ""
				// 参数物化
				finalParams := make(map[string]any, len(task.Params))
				for k, v := range task.Params {
					finalParams[k] = v
				}

				depMap := results.GetMany(task.DependsOn)
				ctxVars := make(flowschema.Context, len(finalParams)+2)
				ctxVars["_deps"] = depMap

				for pk, ref := range task.ParamRefs {
					val, ok, rerr := aschema.ResolveParamRef(ref, results, task)
					if rerr != nil {
						m.log().TaskErr(egCtx, flow.AgentTaskEvent{
							PlanID: plan.PlanID, TaskID: task.TaskID, FlowID: task.FlowID, Stage: task.Stage,
							AgentID: agID, Kind: "task.err", TenantUUID: tenantPtr, UserID: userID, CustomerID: customerID,
							Ts: time.Now(), Error: fmt.Sprintf("param_ref(%s): %v", pk, rerr),
							Input: inSan.JSON(finalParams),
						})
						return fmt.Errorf("param_ref resolve error (%s:%s): %w", task.TaskID, pk, rerr)
					}
					if !ok {
						m.log().TaskErr(egCtx, flow.AgentTaskEvent{
							PlanID: plan.PlanID, TaskID: task.TaskID, FlowID: task.FlowID, Stage: task.Stage,
							AgentID: agID, Kind: "task.err", TenantUUID: tenantPtr, UserID: userID, CustomerID: customerID,
							Ts: time.Now(), Error: fmt.Sprintf("param_ref not found (%s)", ref),
							Input: inSan.JSON(finalParams),
						})
						return fmt.Errorf("param_ref not found (%s:%s) => %s", task.TaskID, pk, ref)
					}
					finalParams[pk] = val
				}
				for k, v := range finalParams {
					ctxVars[k] = v
				}

				taskKind := planTaskKind(task)
				nodeRef := planTaskRef(task)
				flowID := task.FlowID
				if taskKind != "workflow" {
					flowID = nodeRef
				}

				var ag contract.AgentClient
				if taskKind == "workflow" {
					// workflow 节点仍走既有 Agent 路由
					var resolveErr error
					ag, agID, resolveErr = m.resolveAgentForTask(task)
					if resolveErr != nil {
						m.log().TaskErr(egCtx, flow.AgentTaskEvent{
							PlanID: plan.PlanID, TaskID: task.TaskID, FlowID: flowID, Stage: task.Stage,
							AgentID: agID, Kind: "task.err", TenantUUID: tenantPtr, UserID: userID, CustomerID: customerID,
							Ts: time.Now(), Error: resolveErr.Error(),
						})
						return fmt.Errorf("resolve agent for task(%s/%s) failed: %w", task.TaskID, flowID, resolveErr)
					}
				}

				// 任务开始日志
				start := time.Now()
				if hooks != nil && hooks.OnTaskStart != nil {
					hooks.OnTaskStart(task)
				}
				m.log().TaskStart(egCtx, flow.AgentTaskEvent{
					PlanID: plan.PlanID, TaskID: task.TaskID, FlowID: flowID, Stage: task.Stage,
					AgentID: agID, Kind: "task.start", TenantUUID: tenantPtr, UserID: userID, CustomerID: customerID,
					Ts: start, Input: inSan.JSON(finalParams),
				})
				traceMeta, traceSeq := m.startPlanTraceNode(egCtx, mt, plan.PlanID, task, taskKind, nodeRef, flowID, start, finalParams)

				runOnce := func(runCtx context.Context) (*aschema.ExecutionResult, error) {
					if taskKind == "workflow" {
						return ag.Invoke(runCtx, task.FlowID, ctxVars, mt)
					}
					return m.executeNonWorkflowTask(runCtx, task, ctxVars, mt)
				}
				// 执行：workflow 走 Agent；其余节点走统一节点执行器
				out, err := runOnce(egCtx)
				if err != nil && planTaskFailurePolicy(task) == "retry-once" {
					out, err = runOnce(egCtx)
				}
				dur := time.Since(start).Milliseconds()
				if err != nil {
					if hooks != nil && hooks.OnTaskEnd != nil {
						hooks.OnTaskEnd(task, nil, err)
					}
					m.log().TaskErr(egCtx, flow.AgentTaskEvent{
						PlanID: plan.PlanID, TaskID: task.TaskID, FlowID: flowID, Stage: task.Stage,
						AgentID: agID, Kind: "task.err", TenantUUID: tenantPtr, UserID: userID, CustomerID: customerID,
						Ts: time.Now(), DurationMS: dur, Error: err.Error(),
					})
					m.failPlanTraceNode(egCtx, traceMeta, traceSeq, task, taskKind, nodeRef, start, err)
					policy := planTaskFailurePolicy(task)
					if policy == "continue" {
						stageOut[i] = &aschema.ExecutionResult{
							Success: false,
							StepID:  task.TaskID,
							Data: flowschema.Result{
								"task_id":   task.TaskID,
								"flow_id":   flowID,
								"error":     err.Error(),
								"node_kind": taskKind,
							},
							Metadata: flowschema.Result{
								"is_final":       false,
								"status":         "failed",
								"failure_policy": policy,
							},
						}
						return nil
					}
					return fmt.Errorf("invoke task(%s/%s) failed: %w", task.TaskID, flowID, err)
				}
				if out != nil {
					if out.Metadata == nil {
						out.Metadata = flowschema.Result{}
					}
					out.Metadata["task_id"] = task.TaskID
					out.Metadata["flow_id"] = flowID
					out.Metadata["node_kind"] = taskKind
					out.Metadata["node_ref"] = nodeRef
					out.Metadata["source_scope"] = firstNonEmpty(strings.TrimSpace(task.SourceScope), "system")
					out.Duration = time.Duration(dur) * time.Millisecond

					results.Put(task.TaskID, flowID, s, out)
					stageOut[i] = out

					// 任务成功日志（输出做摘要&去循环）
					safeOut := outSan.SanitizeResult(out) // 得到瘦身后的 ExecutionResult
					m.log().TaskOK(egCtx, flow.AgentTaskEvent{
						PlanID: plan.PlanID, TaskID: task.TaskID, FlowID: flowID, Stage: task.Stage,
						AgentID: agID, Kind: "task.ok", TenantUUID: tenantPtr, UserID: userID, CustomerID: customerID,
						Ts: time.Now(), DurationMS: dur,
						Output: outSan.JSON(safeOut.Data),
						Meta:   outSan.JSON(safeOut.Metadata),
					})
					m.endPlanTraceNode(egCtx, traceMeta, traceSeq, task, taskKind, nodeRef, start, out)
					if hooks != nil && hooks.OnTaskEnd != nil {
						hooks.OnTaskEnd(task, out, nil)
					}
				}
				return nil
			})
		}

		if err := eg.Wait(); err != nil {
			// 计划结束（失败）
			m.log().PlanEnd(ctx, flow.AgentTaskEvent{
				PlanID: plan.PlanID, Kind: "plan.end", TenantUUID: tenantPtr, UserID: userID, CustomerID: customerID,
				Ts: time.Now(), Meta: outSan.JSON(map[string]any{"status": "failed", "error": err.Error()}),
			})
			return nil, err
		}

		// 选择“本阶段最后一个非空输出”作为阶段结果（确定性）
		for i := len(stageOut) - 1; i >= 0; i-- {
			if stageOut[i] != nil {
				finalOut = stageOut[i]
				break
			}
		}
	}

	// 计划结束（成功）
	m.log().PlanEnd(ctx, flow.AgentTaskEvent{
		PlanID: plan.PlanID, Kind: "plan.end", TenantUUID: tenantPtr, UserID: userID, CustomerID: customerID,
		Ts: time.Now(), Meta: outSan.JSON(map[string]any{"status": "completed"}),
	})

	return finalOut, nil
}

/*************************
 * helpers: route & store
 *************************/

func (m *Manager) resolveAgentForTask(t flowschema.PlanTask) (contract.AgentClient, string, error) {
	// 1) task.AgentID 优先
	if t.AgentID != "" {
		m.mu.RLock()
		ag := m.agentClients[t.AgentID]
		m.mu.RUnlock()
		if ag == nil {
			return nil, "", fmt.Errorf("agent not found: %s", t.AgentID)
		}
		return ag, t.AgentID, nil
	}
	// 2) routesByFlow
	m.mu.RLock()
	rec, ok := m.routesByFlow[t.FlowID]
	m.mu.RUnlock()
	if ok {
		m.mu.RLock()
		ag := m.agentClients[rec.AgentID]
		m.mu.RUnlock()
		if ag == nil {
			return nil, "", fmt.Errorf("agent not found for flow(%s): %s", t.FlowID, rec.AgentID)
		}
		return ag, rec.AgentID, nil
	}
	// 3) 默认
	ag, _, err := m.GetDefaultRoute()
	if err != nil {
		return nil, "", fmt.Errorf("no route for flow(%s) and no default agent: %w", t.FlowID, err)
	}
	return ag, m.defaultAgID, nil
}

func (m *Manager) GetDefaultRoute() (contract.AgentClient, string, error) {
	if m.defaultAgID == "" || m.defaultFlowID == "" {
		return nil, "", errors.New("default agent/flow is not set")
	}
	m.mu.RLock()
	ag := m.agentClients[m.defaultAgID]
	m.mu.RUnlock()
	if ag == nil {
		return nil, "", fmt.Errorf("default agent not found: %s", m.defaultAgID)
	}
	return ag, m.defaultFlowID, nil
}

func (m *Manager) startPlanTraceNode(ctx context.Context, mt aschema.ExecutionMeta, planID string, task flowschema.PlanTask, kind, nodeRef, flowID string, startedAt time.Time, input map[string]any) (agenttrace.AgentRunMeta, int) {
	logger := m.AgentTraceLogger()
	if logger == nil {
		return agenttrace.AgentRunMeta{}, 0
	}
	meta := agenttrace.AgentRunMeta{
		TraceID:    firstNonEmpty(mt.TraceID, asString(mt.Metadata["trace_id"])),
		RunID:      asString(mt.Metadata["run_id"]),
		TenantUUID: firstNonEmpty(mt.TenantUUID, asString(mt.Metadata["tenant_uuid"])),
		UserUUID:   asString(mt.Metadata["user_uuid"]),
		AgentID:    firstNonEmpty(asString(mt.Metadata["agent_id"]), task.AgentID, m.defaultAgID),
		SessionID:  asString(mt.Metadata["session_id"]),
		MessageID:  asString(mt.Metadata["message_id"]),
		PlanID:     firstNonEmpty(planID, asString(mt.Metadata["plan_id"])),
		Channel:    asString(mt.Metadata["channel"]),
	}
	seq := task.Stage*1000 + stableTaskOrdinal(task.TaskID)
	attrs := map[string]any{
		"task_id":          task.TaskID,
		"flow_id":          flowID,
		"node_id":          task.TaskID,
		"node_kind":        kind,
		"node_ref":         nodeRef,
		"source_scope":     firstNonEmpty(strings.TrimSpace(task.SourceScope), "system"),
		"team_id":          strings.TrimSpace(task.TeamID),
		"handoff_task_id":  strings.TrimSpace(task.HandoffTaskID),
		"context_ref_id":   strings.TrimSpace(task.ContextRefID),
		"failure_policy":   planTaskFailurePolicy(task),
		"capability_id":    asString(input["capability_id"]),
		"plugin_id":        firstNonEmpty(asString(input["plugin_id"]), asString(input["provider"]), asString(input["plugin"])),
		"skill_id":         firstNonEmpty(nodeRef, asString(input["skill_id"])),
		"stage":            task.Stage,
		"depends_on_count": len(task.DependsOn),
	}
	_ = logger.StartNode(ctx, agenttrace.AgentTraceNode{
		AgentRunMeta: meta,
		NodeID:       task.TaskID,
		NodeSeq:      seq,
		NodeKind:     kind,
		NodeRef:      nodeRef,
		InputSummary: attrs,
		ContextRef:   strings.TrimSpace(task.ContextRefID),
		SkillID:      firstNonEmpty(nodeRef, asString(input["skill_id"])),
		PluginID:     firstNonEmpty(asString(input["plugin_id"]), asString(input["provider"]), asString(input["plugin"])),
		CapabilityID: asString(input["capability_id"]),
		Attributes:   attrs,
		StartedAt:    startedAt.UTC(),
	})
	return meta, seq
}

func (m *Manager) endPlanTraceNode(ctx context.Context, meta agenttrace.AgentRunMeta, seq int, task flowschema.PlanTask, kind, nodeRef string, startedAt time.Time, out *aschema.ExecutionResult) {
	if meta.RunID == "" || seq == 0 {
		return
	}
	logger := m.AgentTraceLogger()
	if logger == nil {
		return
	}
	summary := map[string]any{}
	if out != nil {
		summary["success"] = out.Success
		summary["step_id"] = out.StepID
		summary["duration_ms"] = out.Duration.Milliseconds()
		if out.Metadata != nil {
			summary["metadata"] = out.Metadata
		}
	}
	_ = logger.EndNode(ctx, agenttrace.AgentTraceNodeResult{
		AgentRunMeta:  meta,
		NodeID:        task.TaskID,
		NodeSeq:       seq,
		NodeKind:      kind,
		NodeRef:       nodeRef,
		OutputSummary: summary,
		StartedAt:     startedAt.UTC(),
		EndedAt:       time.Now().UTC(),
	})
}

func (m *Manager) failPlanTraceNode(ctx context.Context, meta agenttrace.AgentRunMeta, seq int, task flowschema.PlanTask, kind, nodeRef string, startedAt time.Time, runErr error) {
	if meta.RunID == "" || seq == 0 {
		return
	}
	logger := m.AgentTraceLogger()
	if logger == nil {
		return
	}
	summary := ""
	if runErr != nil {
		summary = runErr.Error()
	}
	_ = logger.FailNode(ctx, agenttrace.AgentTraceNodeFailure{
		AgentTraceNodeResult: agenttrace.AgentTraceNodeResult{
			AgentRunMeta: meta,
			NodeID:       task.TaskID,
			NodeSeq:      seq,
			NodeKind:     kind,
			NodeRef:      nodeRef,
			StartedAt:    startedAt.UTC(),
			EndedAt:      time.Now().UTC(),
		},
		ErrorCode:    "AGENT_PLAN_TASK_FAILED",
		ErrorSummary: summary,
	})
}

func stableTaskOrdinal(taskID string) int {
	if strings.TrimSpace(taskID) == "" {
		return 1
	}
	n := 0
	for _, r := range taskID {
		n += int(r)
	}
	if n <= 0 {
		return 1
	}
	return n % 997
}

func planTaskKind(t flowschema.PlanTask) string {
	k := strings.ToLower(strings.TrimSpace(t.NodeKind))
	if k != "" {
		return k
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t.FlowID)), "skill.") {
		return "skill"
	}
	return "workflow"
}

func planTaskRef(t flowschema.PlanTask) string {
	if ref := strings.TrimSpace(t.NodeRef); ref != "" {
		return ref
	}
	return strings.TrimSpace(t.FlowID)
}

func planTaskFailurePolicy(t flowschema.PlanTask) string {
	raw := strings.ToLower(strings.TrimSpace(t.FailurePolicy))
	if raw == "" {
		raw = strings.ToLower(strings.TrimSpace(asString(t.Params["failure_policy"])))
	}
	switch raw {
	case "fail-fast", "continue", "retry-once":
		return raw
	default:
		return "fail-fast"
	}
}

func (m *Manager) executeNonWorkflowTask(ctx context.Context, t flowschema.PlanTask, params flowschema.Context, mt aschema.ExecutionMeta) (*aschema.ExecutionResult, error) {
	kind := planTaskKind(t)
	ref := planTaskRef(t)
	switch kind {
	case "skill":
		return m.executeSkillTask(ctx, t, params, mt, ref)
	case "tooling":
		return m.executeToolingTask(ctx, t, params, mt, ref)
	case "agent_handoff":
		return m.executeAgentHandoffTask(ctx, t, params, mt, ref)
	case "llm":
		msg := strings.TrimSpace(fmt.Sprintf("%v", params["message"]))
		if msg == "" {
			msg = "继续按上下文回复"
		}
		out, _, err := m.Dispatch(ctx, msg, params, mt)
		if err != nil {
			return nil, err
		}
		return out, nil
	}
	return &aschema.ExecutionResult{
		Success: true,
		Data: flowschema.Result{
			"id":         fmt.Sprintf("id-%s", ref),
			"node_kind":  kind,
			"node_ref":   ref,
			"params":     t.Params,
			"depends_on": t.DependsOn,
		},
		Metadata: flowschema.Result{
			"is_final":     false,
			"node_kind":    kind,
			"node_ref":     ref,
			"planner_mode": "unified",
		},
	}, nil
}

func (m *Manager) executeAgentHandoffTask(ctx context.Context, t flowschema.PlanTask, params flowschema.Context, mt aschema.ExecutionMeta, ref string) (*aschema.ExecutionResult, error) {
	childAgentKey := firstNonEmpty(strings.TrimSpace(t.AgentID), asString(params["child_agent_key"]), asString(t.Params["child_agent_key"]))
	childAgentID := firstPositiveUint64(asUint64(params["child_agent_id"]), asUint64(t.Params["child_agent_id"]))
	if childAgentKey == "" && childAgentID == 0 {
		return nil, errors.New("agent_handoff missing child_agent identifier")
	}
	contextRefID := firstNonEmpty(t.ContextRefID, asString(params["context_ref_id"]), asString(t.Params["context_ref_id"]))
	m.mu.RLock()
	authz := m.contextRefAuthz
	inv := m.handoffInvoker
	m.mu.RUnlock()
	if childAgentID == 0 && childAgentKey != "" {
		childAgentID = asUint64(childAgentKey)
	}
	if authz != nil && contextRefID != "" {
		if err := authz(ctx, firstNonEmpty(mt.TenantUUID, asString(params["tenant_uuid"])), childAgentID, contextRefID); err != nil {
			return nil, err
		}
	}

	msg := strings.TrimSpace(asString(params["message"]))
	if msg == "" {
		msg = strings.TrimSpace(asString(params["query"]))
	}
	if msg == "" {
		msg = "继续处理当前任务"
	}
	flowID := firstNonEmpty(asString(t.Params["child_flow_id"]), asString(params["child_flow_id"]), t.FlowID)
	if flowID == "" {
		flowID = ref
	}
	taskID := firstNonEmpty(t.HandoffTaskID, t.TaskID)
	failurePolicy := firstNonEmpty(strings.ToLower(strings.TrimSpace(t.FailurePolicy)), asString(t.Params["failure_policy"]), asString(params["failure_policy"]), "continue")
	teamID := firstNonEmpty(t.TeamID, asString(t.Params["team_id"]), asString(params["team_id"]))
	teamName := firstNonEmpty(asString(t.Params["team_name"]), asString(params["team_name"]))
	sessionID := firstPositiveUint64(asUint64(params["session_id"]), asUint64(t.Params["session_id"]))
	handoffTraceID := firstNonEmpty(asString(mt.Metadata["trace_id"]), mt.TraceID, fmt.Sprintf("handoff_%d", time.Now().UnixNano()))

	payload := payloadFromTaskParams(t, params)
	contextMap := contextFromTaskParams(t, params)
	contextMap["context_ref_id"] = contextRefID
	contextMap["team_id"] = teamID
	contextMap["team_name"] = teamName
	contextMap["parent_agent_id"] = firstPositiveUint64(asUint64(mt.Metadata["agent_id"]), asUint64(params["parent_agent_id"]))
	contextMap["child_agent_id"] = childAgentID
	contextMap["child_agent_key"] = childAgentKey

	in := AgentHandoffInput{
		TenantUUID:     firstNonEmpty(mt.TenantUUID, asString(params["tenant_uuid"])),
		ParentAgentID:  firstPositiveUint64(asUint64(mt.Metadata["agent_id"]), asUint64(params["parent_agent_id"])),
		ChildAgentID:   childAgentID,
		TeamID:         asUint64(teamID),
		TaskID:         taskID,
		PlanID:         firstNonEmpty(asString(mt.Metadata["plan_id"]), asString(t.Params["plan_id"]), asString(params["plan_id"])),
		NodeID:         firstNonEmpty(t.TaskID, taskID),
		SessionID:      sessionID,
		FailurePolicy:  failurePolicy,
		ContextRefID:   contextRefID,
		HandoffTraceID: handoffTraceID,
		RunID:          asString(mt.Metadata["run_id"]),
		FlowID:         flowID,
		Message:        msg,
		Payload:        payload,
		Context:        contextMap,
	}
	if inv != nil {
		out, err := inv(ctx, in)
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, errors.New("agent_handoff invoker returns nil output")
		}
		return &aschema.ExecutionResult{
			Success: !strings.EqualFold(strings.TrimSpace(out.Status), "failed"),
			Data: map[string]any{
				"task_id":          out.TaskID,
				"handoff_trace_id": out.HandoffTraceID,
				"status":           out.Status,
				"result":           out.Result,
				"content":          asString(out.Result["content"]),
			},
			Metadata: flowschema.Result{
				"is_final":        false,
				"node_kind":       "agent_handoff",
				"node_ref":        firstNonEmpty(ref, flowID),
				"team_id":         teamID,
				"team_name":       teamName,
				"parent_agent_id": in.ParentAgentID,
				"child_agent_id":  in.ChildAgentID,
				"child_agent_key": childAgentKey,
				"handoff_task_id": out.TaskID,
				"failure_policy":  failurePolicy,
				"context_ref_id":  contextRefID,
				"trace_id":        out.HandoffTraceID,
				"planner_mode":    "unified",
			},
		}, nil
	}

	// fallback: no explicit handoff invoker configured, directly dispatch to child agent route.
	childTask := flowschema.PlanTask{
		TaskID:   taskID,
		FlowID:   flowID,
		AgentID:  childAgentKey,
		NodeKind: "workflow",
	}
	if childTask.AgentID == "" && childAgentID > 0 {
		childTask.AgentID = strconv.FormatUint(childAgentID, 10)
	}
	ag, _, err := m.resolveAgentForTask(childTask)
	if err != nil {
		return nil, err
	}
	out, err := ag.Invoke(ctx, flowID, flowschema.Context{
		"message": msg,
		"payload": payload,
		"context": contextMap,
	}, mt)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, errors.New("agent_handoff fallback output is nil")
	}
	if out.Metadata == nil {
		out.Metadata = flowschema.Result{}
	}
	out.Metadata["node_kind"] = "agent_handoff"
	out.Metadata["node_ref"] = firstNonEmpty(ref, flowID)
	out.Metadata["team_id"] = teamID
	out.Metadata["team_name"] = teamName
	out.Metadata["parent_agent_id"] = in.ParentAgentID
	out.Metadata["child_agent_id"] = in.ChildAgentID
	out.Metadata["child_agent_key"] = childAgentKey
	out.Metadata["handoff_task_id"] = taskID
	out.Metadata["failure_policy"] = failurePolicy
	out.Metadata["context_ref_id"] = contextRefID
	out.Metadata["trace_id"] = handoffTraceID
	out.Metadata["planner_mode"] = "unified"
	return out, nil
}

func (m *Manager) executeSkillTask(ctx context.Context, t flowschema.PlanTask, params flowschema.Context, mt aschema.ExecutionMeta, ref string) (*aschema.ExecutionResult, error) {
	m.mu.RLock()
	inv := m.skillInvoker
	m.mu.RUnlock()
	if inv == nil {
		return nil, errors.New("skill invoker is not configured")
	}
	in := SkillInvokeInput{
		TenantUUID:   firstNonEmpty(mt.TenantUUID, asString(params["tenant_uuid"])),
		Env:          firstNonEmpty(asString(mt.Metadata["env"]), asString(params["env"])),
		AgentID:      firstPositiveUint64(asUint64(mt.Metadata["agent_id"]), asUint64(params["agent_id"])),
		UserUUID:     firstNonEmpty(asString(mt.Metadata["user_uuid"]), asString(params["user_uuid"]), asString(params["member_uuid"])),
		SessionID:    firstNonEmpty(asString(mt.Metadata["session_id"]), asString(params["session_id"])),
		MessageID:    firstNonEmpty(asString(mt.Metadata["message_id"]), asString(params["message_id"])),
		SkillID:      firstNonEmpty(ref, asString(params["skill_id"])),
		Version:      firstNonEmpty(asString(params["version"]), asString(t.Params["version"])),
		CapabilityID: asString(params["capability_id"]),
		Entrypoint:   asString(params["entrypoint"]),
		TraceID:      firstNonEmpty(mt.TraceID, asString(params["trace_id"])),
		RunID:        asString(mt.Metadata["run_id"]),
		PlanID:       firstNonEmpty(asString(mt.Metadata["plan_id"]), asString(params["plan_id"])),
		NodeID:       firstNonEmpty(t.TaskID, asString(params["node_id"])),
		PluginID:     firstNonEmpty(asString(params["plugin_id"]), asString(params["provider"]), asString(t.Params["plugin_id"])),
		Payload:      payloadFromTaskParams(t, params),
		Context:      contextFromTaskParams(t, params),
		ToolGrantIDs: toStringSlice(params["tool_grant_ids"]),
	}
	in.Context["trace_id"] = in.TraceID
	in.Context["run_id"] = in.RunID
	in.Context["plan_id"] = in.PlanID
	in.Context["node_id"] = in.NodeID
	in.Context["plugin_id"] = in.PluginID
	in.Context["capability_id"] = in.CapabilityID
	logger.InfoF(ctx, "[agent.skill.invoke] skill_id=%s action=%s capability_id=%s plugin_id=%s trace_id=%s payload=%s",
		in.SkillID,
		firstNonEmpty(asString(in.Payload["action"]), asString(in.Payload["operation"]), asString(params["action"]), asString(t.Params["action"])),
		in.CapabilityID,
		in.PluginID,
		in.TraceID,
		utils.J(in.Payload),
	)
	out, err := inv(ctx, in)
	if err != nil {
		return nil, err
	}
	data := map[string]any{
		"trace_id":      out.TraceID,
		"status":        out.Status,
		"protocol_used": out.ProtocolUsed,
		"fallback_used": out.FallbackUsed,
		"skill_id":      out.SkillID,
		"version":       out.Version,
		"result":        out.Result,
	}
	if content := pickSkillVisibleContent(out.Result); content != "" {
		// 统一可展示输出，供 runtime.buildFinalContent 直接透传给用户。
		data["content"] = content
	}
	return &aschema.ExecutionResult{
		Success: true,
		Data:    data,
		Metadata: flowschema.Result{
			"is_final":      false,
			"node_kind":     "skill",
			"node_ref":      ref,
			"trace_id":      out.TraceID,
			"run_id":        in.RunID,
			"plan_id":       in.PlanID,
			"node_id":       in.NodeID,
			"plugin_id":     in.PluginID,
			"capability_id": in.CapabilityID,
			"status":        out.Status,
			"protocol_used": out.ProtocolUsed,
			"fallback_used": out.FallbackUsed,
			"planner_mode":  "unified",
		},
	}, nil
}

func pickSkillVisibleContent(result map[string]any) string {
	if len(result) == 0 {
		return ""
	}
	if s := pickTemplateVisibleContent(result); s != "" {
		return s
	}
	for _, key := range []string{"content", "rendered_text", "text", "output", "answer", "message"} {
		if v, ok := result[key]; ok {
			if s := strings.TrimSpace(fmt.Sprintf("%v", v)); s != "" {
				return s
			}
		}
	}
	return ""
}

func pickTemplateVisibleContent(result map[string]any) string {
	action := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", result["action"])))
	if action == "" {
		return ""
	}
	rawTemplate, ok := result["template"].(map[string]any)
	if !ok {
		return ""
	}
	id := strings.TrimSpace(fmt.Sprintf("%v", firstNonEmptyAny(result["template_id"], rawTemplate["id"])))
	title := strings.TrimSpace(fmt.Sprintf("%v", firstNonEmptyAny(rawTemplate["title"], rawTemplate["name"])))
	detailPath := strings.TrimSpace(fmt.Sprintf("%v", rawTemplate["detail_path"]))
	switch action {
	case "create":
		parts := []string{"模板已创建成功。"}
		if title != "" {
			parts = append(parts, "标题："+title)
		}
		if id != "" {
			parts = append(parts, "ID："+id)
		}
		if detailPath != "" {
			parts = append(parts, "查看："+detailPath)
		}
		return strings.Join(parts, "\n")
	case "update":
		parts := []string{"模板已更新成功。"}
		if title != "" {
			parts = append(parts, "标题："+title)
		}
		if id != "" {
			parts = append(parts, "ID："+id)
		}
		if detailPath != "" {
			parts = append(parts, "查看："+detailPath)
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func firstNonEmptyAny(values ...any) any {
	for _, value := range values {
		if strings.TrimSpace(fmt.Sprintf("%v", value)) != "" && strings.TrimSpace(fmt.Sprintf("%v", value)) != "<nil>" {
			return value
		}
	}
	return ""
}

func (m *Manager) executeToolingTask(ctx context.Context, t flowschema.PlanTask, params flowschema.Context, mt aschema.ExecutionMeta, ref string) (*aschema.ExecutionResult, error) {
	m.mu.RLock()
	inv := m.toolingInvoker
	m.mu.RUnlock()
	if inv == nil {
		return nil, errors.New("tooling invoker is not configured")
	}
	in := ToolingInvokeInput{
		TenantUUID:        firstNonEmpty(mt.TenantUUID, asString(params["tenant_uuid"])),
		Env:               firstNonEmpty(asString(mt.Metadata["env"]), asString(params["env"])),
		CapabilityID:      firstNonEmpty(asString(params["capability_id"]), ref),
		PreferredProtocol: firstNonEmpty(asString(params["preferred_protocol"]), "tooling"),
		TraceID:           firstNonEmpty(mt.TraceID, asString(params["trace_id"])),
		RunID:             asString(mt.Metadata["run_id"]),
		PlanID:            firstNonEmpty(asString(mt.Metadata["plan_id"]), asString(params["plan_id"])),
		NodeID:            firstNonEmpty(t.TaskID, asString(params["node_id"])),
		Payload:           payloadFromTaskParams(t, params),
		Context:           contextFromTaskParams(t, params),
	}
	in.Context["trace_id"] = in.TraceID
	in.Context["run_id"] = in.RunID
	in.Context["plan_id"] = in.PlanID
	in.Context["node_id"] = in.NodeID
	in.Context["capability_id"] = in.CapabilityID
	out, err := inv(ctx, in)
	if err != nil {
		return nil, err
	}
	data := map[string]any{
		"trace_id":      out.TraceID,
		"status":        out.Status,
		"protocol_used": out.ProtocolUsed,
		"fallback_used": out.FallbackUsed,
		"result":        out.Result,
	}
	return &aschema.ExecutionResult{
		Success: true,
		Data:    data,
		Metadata: flowschema.Result{
			"is_final":      false,
			"node_kind":     "tooling",
			"node_ref":      ref,
			"trace_id":      out.TraceID,
			"run_id":        in.RunID,
			"plan_id":       in.PlanID,
			"node_id":       in.NodeID,
			"capability_id": in.CapabilityID,
			"status":        out.Status,
			"protocol_used": out.ProtocolUsed,
			"fallback_used": out.FallbackUsed,
			"planner_mode":  "unified",
		},
	}, nil
}

func payloadFromTaskParams(t flowschema.PlanTask, params flowschema.Context) map[string]any {
	if p, ok := params["payload"].(map[string]any); ok && len(p) > 0 {
		return p
	}
	if p, ok := t.Params["payload"].(map[string]any); ok && len(p) > 0 {
		return p
	}
	out := make(map[string]any, len(t.Params))
	for k, v := range t.Params {
		if strings.HasPrefix(k, "_") {
			continue
		}
		if k == "payload" || k == "context" || k == "tool_grant_ids" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return map[string]any{}
	}
	return out
}

func contextFromTaskParams(t flowschema.PlanTask, params flowschema.Context) map[string]any {
	out := map[string]any{}

	// 1) runtime context wins: explicit params.context (map or string)
	if p, ok := params["context"].(map[string]any); ok && len(p) > 0 {
		for k, v := range p {
			out[k] = v
		}
	} else if s := asString(params["context"]); s != "" {
		out["context"] = s
	}

	// 2) task planned context as fallback: task.params.context (map or string)
	if len(out) == 0 {
		if p, ok := t.Params["context"].(map[string]any); ok && len(p) > 0 {
			for k, v := range p {
				out[k] = v
			}
		} else if s := asString(t.Params["context"]); s != "" {
			out["context"] = s
		}
	}

	// 3) keep dependency snapshots and natural language message as auxiliary context
	if deps, ok := params["_deps"]; ok {
		out["_deps"] = deps
	}
	if msg := asString(params["message"]); msg != "" {
		out["message"] = msg
	}
	if query := asString(params["query"]); query != "" {
		out["query"] = query
	}
	if prompt := asString(params["prompt"]); prompt != "" {
		out["prompt"] = prompt
	}
	return out
}

func toStringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s := strings.TrimSpace(asString(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	}
}

func asUint64(v any) uint64 {
	switch x := v.(type) {
	case uint64:
		return x
	case uint:
		return uint64(x)
	case int:
		if x > 0 {
			return uint64(x)
		}
	case int64:
		if x > 0 {
			return uint64(x)
		}
	case float64:
		if x > 0 {
			return uint64(x)
		}
	case string:
		u, err := strconv.ParseUint(strings.TrimSpace(x), 10, 64)
		if err == nil {
			return u
		}
	}
	return 0
}

func firstPositiveUint64(vals ...uint64) uint64 {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
