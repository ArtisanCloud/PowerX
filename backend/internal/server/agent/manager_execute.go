package agent

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
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

				// 执行：workflow 走 Agent；其余节点走统一节点执行器（当前为轻量内置实现）
				var out *aschema.ExecutionResult
				var err error
				if taskKind == "workflow" {
					out, err = ag.Invoke(egCtx, task.FlowID, ctxVars, mt)
				} else {
					out, err = m.executeNonWorkflowTask(egCtx, task, ctxVars, mt)
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

func (m *Manager) executeNonWorkflowTask(ctx context.Context, t flowschema.PlanTask, params flowschema.Context, mt aschema.ExecutionMeta) (*aschema.ExecutionResult, error) {
	kind := planTaskKind(t)
	ref := planTaskRef(t)
	switch kind {
	case "skill":
		return m.executeSkillTask(ctx, t, params, mt, ref)
	case "tooling":
		return m.executeToolingTask(ctx, t, params, mt, ref)
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
		SkillID:      firstNonEmpty(ref, asString(params["skill_id"])),
		Version:      firstNonEmpty(asString(params["version"]), asString(t.Params["version"])),
		CapabilityID: asString(params["capability_id"]),
		Entrypoint:   asString(params["entrypoint"]),
		TraceID:      firstNonEmpty(mt.TraceID, asString(params["trace_id"])),
		Payload:      payloadFromTaskParams(t, params),
		Context:      contextFromTaskParams(t, params),
		ToolGrantIDs: toStringSlice(params["tool_grant_ids"]),
	}
	out, err := inv(ctx, in)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "skill record not found") {
			if alt, ok := fallbackSkillIDAlias(in.SkillID); ok {
				in.SkillID = alt
				out, err = inv(ctx, in)
			}
		}
	}
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
	for _, key := range []string{"content", "rendered_text", "text", "output", "answer", "message"} {
		if v, ok := result[key]; ok {
			if s := strings.TrimSpace(fmt.Sprintf("%v", v)); s != "" {
				return s
			}
		}
	}
	return ""
}

func fallbackSkillIDAlias(skillID string) (string, bool) {
	raw := strings.TrimSpace(skillID)
	if raw == "" {
		return "", false
	}
	// 兼容历史别名：skill.thirdparty.<id> -> <id>
	if strings.HasPrefix(strings.ToLower(raw), "skill.thirdparty.") {
		alt := strings.TrimSpace(raw[len("skill.thirdparty."):])
		if alt != "" && !strings.EqualFold(alt, raw) {
			return alt, true
		}
	}
	return "", false
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
		Payload:           payloadFromTaskParams(t, params),
		Context:           contextFromTaskParams(t, params),
	}
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
