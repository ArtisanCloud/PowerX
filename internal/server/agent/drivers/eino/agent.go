// File: services/agent/drivers/eino/agent.go
package eino

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/llm"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/loader"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"

	"github.com/cloudwego/eino/schema"
)

/* ================= 类型定义 ================= */

// 每种节点的执行器签名（注意 node 是 *flowschema.Node）
type NodeExec func(
	ctx context.Context,
	a *Agent,
	curFlowID string,
	node *flowschema.Node,
	in flowschema.Context,
	meta agentschema.ExecutionMeta,
) (flowschema.Result, error)

var _ contract.Agent = (*Agent)(nil)

/* ================= Agent ================= */

type Agent struct {
	config    *config.AgentConfig
	agentInfo *agentschema.AgentInfo

	// 蓝图解析缓存
	flowMu    sync.RWMutex
	flowCache map[string]*flowschema.Flow // flowID -> Flow

	// resolver
	resolver    *loader.Resolver
	flowAliases map[string]string

	// 节点执行器
	execMu    sync.RWMutex
	kindExecs map[string]NodeExec // kind(小写) -> exec
	useExecs  map[string]NodeExec // use(原样)  -> exec（系统内部函数）

	// 异步执行
	asyncMu      sync.RWMutex
	statusByExec map[string]*agentschema.ExecutionStatus
	resultByExec map[string]*agentschema.ExecutionResult
}

/* ================= 构造 / 注册 ================= */

func NewAgent(cfg *config.AgentConfig) (contract.Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不能为空")
	}
	r := loader.NewResolver(cfg.FlowSpec.BusinessDir)
	if err := r.BuildIndex(); err != nil {
		return nil, fmt.Errorf("build blueprint index failed: %w", err)
	}
	now := time.Now().UTC()

	a := &Agent{
		config:       cfg,
		agentInfo:    &agentschema.AgentInfo{Name: "eino", Status: string(agentschema.StatusInit), CreatedAt: now, UpdatedAt: now},
		resolver:     r,
		flowCache:    make(map[string]*flowschema.Flow),
		kindExecs:    make(map[string]NodeExec),
		useExecs:     make(map[string]NodeExec),
		statusByExec: make(map[string]*agentschema.ExecutionStatus),
		resultByExec: make(map[string]*agentschema.ExecutionResult),
		flowAliases:  make(map[string]string),
	}
	a.registerBuiltins()
	return a, nil
}

// 注册系统内部函数（按 use 精确分派）
func (a *Agent) RegisterFunc(use string, exec NodeExec) {
	a.execMu.Lock()
	a.useExecs[strings.TrimSpace(use)] = exec
	a.execMu.Unlock()
}

// 注册内置 kind 的最小可用执行器（全部使用常量，统一小写 key）
func (a *Agent) registerBuiltins() {
	set := func(kind flowschema.NodeKind, exec NodeExec) {
		a.kindExecs[strings.ToLower(string(kind))] = exec
	}
	// 基础
	set(flowschema.KindStart, execNoop("start"))
	set(flowschema.KindEnd, execNoop("end"))
	set(flowschema.KindIOInput, execNoop("input"))
	set(flowschema.KindIOOutput, execNoop("output"))
	// 核心
	set(flowschema.KindSelector, execSelector()) // 本次补齐的实现
	set(flowschema.KindLLM, execLLM(a))
	set(flowschema.KindWorkflow, execWorkflow(a))
	// 其它先用回显占位（后续你可替换/注册系统 use）
	for _, k := range []flowschema.NodeKind{
		flowschema.KindBizLogic, flowschema.KindPlugin, flowschema.KindCode, flowschema.KindHTTP,
		flowschema.KindDB, flowschema.KindQA, flowschema.KindTextProc, flowschema.KindJSONSerDe,
		flowschema.KindLoop, flowschema.KindBatch, flowschema.KindAggVars, flowschema.KindTrigger,
		flowschema.KindSession, flowschema.KindMessage, flowschema.KindKnowledge, flowschema.KindMemory,
		flowschema.KindImage, flowschema.KindVideo, flowschema.KindComponent, flowschema.KindIntent,
	} {
		lk := strings.ToLower(string(k))
		if _, ok := a.kindExecs[lk]; !ok {
			a.kindExecs[lk] = execEcho(lk)
		}
	}
}

/* ================= Flow 获取（缓存） ================= */

func (a *Agent) getOrBuildFlow(flowID string) (*flowschema.Flow, error) {
	// 命中缓存
	a.flowMu.RLock()
	if f := a.flowCache[flowID]; f != nil {
		a.flowMu.RUnlock()
		fmt.Printf("[agent.eino] getOrBuildFlow: cache hit id=%s nodes=%d\n", flowID, len(f.Nodes))
		return f, nil
	}
	a.flowMu.RUnlock()

	fmt.Printf("[agent.eino] getOrBuildFlow: resolving id=%s\n", flowID)

	// 解析
	f, err := a.resolver.Resolve(flowID)
	if err != nil {
		fmt.Printf("[agent.eino] getOrBuildFlow: resolve FAILED id=%s err=%v\n", flowID, err)
		return nil, fmt.Errorf("resolve flow(%s) failed: %w", flowID, err)
	}
	fmt.Printf("[agent.eino] getOrBuildFlow: resolve OK id=%s nodes=%d\n", f.FlowID, len(f.Nodes))

	// 缓存
	a.flowMu.Lock()
	a.flowCache[flowID] = f
	a.flowMu.Unlock()
	return f, nil
}

/* ================= contract.Agent ================= */

func (a *Agent) GetInfo() *agentschema.AgentInfo { return a.agentInfo }

func (a *Agent) ListFlows(ctx context.Context, meta agentschema.ExecutionMeta) ([]agentschema.FlowRuntimeInfo, error) {
	// 如果 resolver 支持 ListIDs()，就列出来
	type idLister interface{ ListIDs() []string }
	out := []agentschema.FlowRuntimeInfo{}
	if l, ok := any(a.resolver).(idLister); ok {
		now := time.Now().UTC()
		for _, id := range l.ListIDs() {
			out = append(out, agentschema.FlowRuntimeInfo{
				Flow:      flowschema.Flow{FlowID: id},
				Status:    "ready",
				CreatedAt: now, UpdatedAt: now,
			})
		}
	}
	return out, nil
}

func (a *Agent) GetFlowInfo(ctx context.Context, flowID string, meta agentschema.ExecutionMeta) (*agentschema.FlowRuntimeInfo, error) {
	if flowID == "" {
		return nil, fmt.Errorf("flowID 不能为空")
	}
	f, err := a.getOrBuildFlow(flowID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &agentschema.FlowRuntimeInfo{
		Flow:      *f,
		Status:    "ready",
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (a *Agent) ValidateParams(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) error {
	// TODO：如需 JSONSchema 校验在此实现
	return nil
}

func (a *Agent) Invoke(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	if flowID == "" {
		return nil, fmt.Errorf("flowID 不能为空")
	}
	start := time.Now()

	flow, err := a.getOrBuildFlow(flowID)
	if err != nil {
		return nil, err
	}

	// 运行上下文
	if params == nil {
		params = flowschema.Context{}
	}
	steps := map[string]any{}
	params["_vars"] = steps // 供节点/ParamRefs取值
	// params["_last"] 将在循环内更新

	for i := range flow.Nodes {
		n := flow.Nodes[i] // *flowschema.Node
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		stepKey := fmt.Sprintf("step_%d", i+1)

		// ---------- 1) 合成节点输入：全局 params + 节点 params + IO.InMap ----------
		in := mergeCtx(params, n.Params)
		if m := n.IOInMap(); len(m) > 0 {
			for toKey, fromPath := range m {
				if v, ok := getByPathCtx(in, fromPath); ok {
					in[toKey] = v
				}
			}
		}

		// ---------- 2) 选执行器（use 优先；否则按 kind 常量） ----------
		exec := a.pickExecutor(n)

		// ---------- 3) 执行 ----------
		res, err := exec(ctx, a, flow.FlowID, n, in, meta)
		if err != nil {
			return nil, fmt.Errorf("node(kind=%s use=%s) failed at %s: %w", n.Kind, n.Use, stepKey, err)
		}

		// ---------- 4) IO.OutMap：把输出写回上下文，供后续节点直接使用 ----------
		if om := n.IOOutMap(); len(om) > 0 && res != nil {
			for fromKey, toPath := range om {
				if val, ok := getByPathMap(map[string]any(res), fromKey); ok {
					setByPathCtx(params, toPath, val)
				}
			}
		}

		// ---------- 5) 记录步骤输出 ----------
		steps[stepKey] = res
		params["_last"] = res
	}

	// 汇总输出
	out := flowschema.Result{
		"flow":  flowID,
		"steps": steps,
	}
	if v, ok := params["upstream_id"].(string); ok && v != "" {
		out["id"] = v
	} else {
		out["id"] = fmt.Sprintf("ID_%s_%d", flowID, time.Now().UnixNano())
	}

	return &agentschema.ExecutionResult{
		Success:   true,
		Data:      out,
		Duration:  time.Since(start),
		StepID:    "final",
		Timestamp: time.Now().UTC().Unix(),
		Metadata:  flowschema.Result{"flow_id": flowID, "plan_id": meta.RequestID, "is_final": true},
	}, nil
}

func (a *Agent) InvokeAsync(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (string, error) {
	if flowID == "" {
		return "", fmt.Errorf("flowID 不能为空")
	}
	execID := fmt.Sprintf("%s-%d", meta.RequestID, time.Now().UTC().UnixNano())

	now := time.Now().UTC()
	a.setStatus(execID, &agentschema.ExecutionStatus{
		PlanID:      execID,
		Status:      "running",
		Progress:    0,
		CurrentStep: "",
		StartedAt:   &now,
		Metadata:    map[string]any{"flow_id": flowID},
	})

	go func() {
		res, err := a.Invoke(ctx, flowID, params, meta)
		fin := time.Now().UTC()
		if err != nil || (res != nil && !res.Success) {
			a.setStatus(execID, &agentschema.ExecutionStatus{
				PlanID:      execID,
				Status:      "failed",
				Progress:    100,
				CurrentStep: "final",
				StartedAt:   a.getStart(execID),
				CompletedAt: &fin,
				Error:       errMsg(err, res),
				Metadata:    map[string]any{"flow_id": flowID},
			})
			return
		}
		a.setResult(execID, res)
		a.setStatus(execID, &agentschema.ExecutionStatus{
			PlanID:      execID,
			Status:      "completed",
			Progress:    100,
			CurrentStep: "final",
			StartedAt:   a.getStart(execID),
			CompletedAt: &fin,
			Metadata:    map[string]any{"flow_id": flowID},
		})
	}()

	return execID, nil
}

func (a *Agent) Stream(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (*schema.StreamReader[*agentschema.ExecutionResult], error) {
	if flowID == "" {
		return nil, fmt.Errorf("flowID 不能为空")
	}
	flow, err := a.getOrBuildFlow(flowID)
	if err != nil {
		return nil, err
	}
	if params == nil {
		params = flowschema.Context{}
	}
	steps := map[string]any{}
	params["_vars"] = steps

	execID := fmt.Sprintf("%s-%d", meta.RequestID, time.Now().UTC().UnixNano())
	now := time.Now().UTC()
	a.setStatus(execID, &agentschema.ExecutionStatus{
		PlanID:      execID,
		Status:      "running",
		Progress:    0,
		CurrentStep: "start",
		StartedAt:   &now,
		Metadata:    map[string]any{"flow_id": flowID},
	})

	sr, sw := agent.NewResultPipe(16)
	go func() {
		defer sw.Close()

		_ = sw.Send(&agentschema.ExecutionResult{
			Success:   true,
			Data:      flowschema.Result{"message": "start"},
			Duration:  0,
			StepID:    "start",
			Timestamp: time.Now().UTC().Unix(),
			Metadata:  flowschema.Result{"flow_id": flowID, "plan_id": execID, "is_final": false},
		}, nil)

		for i := range flow.Nodes {
			n := flow.Nodes[i]
			select {
			case <-ctx.Done():
				_ = sw.Send(nil, ctx.Err())
				fin := time.Now().UTC()
				a.setStatus(execID, &agentschema.ExecutionStatus{
					PlanID:      execID,
					Status:      "cancelled",
					Progress:    float64(i) / float64(utils.Max1(len(flow.Nodes))) * 100,
					CurrentStep: fmt.Sprintf("step-%d", i+1),
					StartedAt:   a.getStart(execID),
					CompletedAt: &fin,
					Error:       ctx.Err().Error(),
					Metadata:    map[string]any{"flow_id": flowID},
				})
				return
			default:
			}

			stepKey := fmt.Sprintf("step_%d", i+1)
			in := mergeCtx(params, n.Params)
			if m := n.IOInMap(); len(m) > 0 {
				for toKey, fromPath := range m {
					if v, ok := getByPathCtx(in, fromPath); ok {
						in[toKey] = v
					}
				}
			}

			exec := a.pickExecutor(n)
			res, err := exec(ctx, a, flow.FlowID, n, in, meta)
			if err != nil {
				_ = sw.Send(nil, err)
				fin := time.Now().UTC()
				a.setStatus(execID, &agentschema.ExecutionStatus{
					PlanID:      execID,
					Status:      "failed",
					Progress:    float64(i) / float64(utils.Max1(len(flow.Nodes))) * 100,
					CurrentStep: stepKey,
					StartedAt:   a.getStart(execID),
					CompletedAt: &fin,
					Error:       err.Error(),
					Metadata:    map[string]any{"flow_id": flowID},
				})
				return
			}

			if om := n.IOOutMap(); len(om) > 0 && res != nil {
				for fromKey, toPath := range om {
					if val, ok := getByPathMap(map[string]any(res), fromKey); ok {
						setByPathCtx(params, toPath, val)
					}
				}
			}

			steps[stepKey] = res
			params["_last"] = res

			isFinal := (i == len(flow.Nodes)-1)
			_ = sw.Send(&agentschema.ExecutionResult{
				Success:   true,
				Data:      flowschema.Result{"step_kind": n.Kind, "step_use": n.Use, "result": res},
				Duration:  0,
				StepID:    stepKey,
				Timestamp: time.Now().UTC().Unix(),
				Metadata:  flowschema.Result{"flow_id": flowID, "plan_id": execID, "is_final": isFinal},
			}, nil)

			a.asyncMu.Lock()
			if stt, ok := a.statusByExec[execID]; ok {
				stt.Progress = float64(i+1) / float64(utils.Max1(len(flow.Nodes))) * 100
				stt.CurrentStep = stepKey
			}
			a.asyncMu.Unlock()

			if isFinal {
				fin := time.Now().UTC()
				a.setStatus(execID, &agentschema.ExecutionStatus{
					PlanID:      execID,
					Status:      "completed",
					Progress:    100,
					CurrentStep: stepKey,
					StartedAt:   a.getStart(execID),
					CompletedAt: &fin,
					Metadata:    map[string]any{"flow_id": flowID},
				})
				return
			}
		}
	}()
	return sr, nil
}

func (a *Agent) GetExecutionStatus(ctx context.Context, executionID string, meta agentschema.ExecutionMeta) (*agentschema.ExecutionStatus, error) {
	a.asyncMu.RLock()
	defer a.asyncMu.RUnlock()
	if st, ok := a.statusByExec[executionID]; ok {
		cp := *st
		return &cp, nil
	}
	return nil, fmt.Errorf("plan not found: %s", executionID)
}

func (a *Agent) CancelExecution(ctx context.Context, executionID string, meta agentschema.ExecutionMeta) error {
	a.asyncMu.Lock()
	defer a.asyncMu.Unlock()
	if st, ok := a.statusByExec[executionID]; ok {
		now := time.Now().UTC()
		st.Status = "cancelled"
		st.CompletedAt = &now
		return nil
	}
	return fmt.Errorf("plan not found: %s", executionID)
}

func (a *Agent) GetExecutionResult(ctx context.Context, executionID string, meta agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	a.asyncMu.RLock()
	defer a.asyncMu.RUnlock()
	if r, ok := a.resultByExec[executionID]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("plan result not found: %s", executionID)
}

func (a *Agent) GetMetrics(ctx context.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
	uptime := time.Since(a.agentInfo.CreatedAt)
	return flowschema.Result{"uptime_sec": int(uptime.Seconds())}, nil
}

func (a *Agent) Health(ctx context.Context) error   { return nil }
func (a *Agent) Shutdown(ctx context.Context) error { return nil }

/* ================= 分派 & 内置执行器 ================= */

func (a *Agent) pickExecutor(n *flowschema.Node) NodeExec {
	// 先按 use（系统内部函数）
	if u := strings.TrimSpace(n.Use); u != "" {
		a.execMu.RLock()
		fn := a.useExecs[u]
		a.execMu.RUnlock()
		if fn != nil {
			return fn
		}
	}
	// 再按 kind（小写）
	key := strings.ToLower(strings.TrimSpace(n.Kind))
	a.execMu.RLock()
	fn := a.kindExecs[key]
	a.execMu.RUnlock()
	if fn != nil {
		return fn
	}
	return execEcho("default")
}

func execNoop(tag string) NodeExec {
	return func(ctx context.Context, a *Agent, curFlowID string, node *flowschema.Node, in flowschema.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
		return flowschema.Result{"tag": tag, "kind": node.Kind, "use": node.Use, "params": node.Params}, nil
	}
}

// 最小 selector：params.cond(in.cond) 为真取 then，否则 else
func execSelector() NodeExec {
	return func(ctx context.Context, a *Agent, curFlowID string, node *flowschema.Node, in flowschema.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
		var cond bool
		if v, ok := node.Params["cond"]; ok {
			switch vv := v.(type) {
			case bool:
				cond = vv
			case string:
				cond = strings.EqualFold(vv, "true") || vv == "1" || strings.EqualFold(vv, "yes")
			default:
				cond = vv != nil
			}
		} else if v, ok := in["cond"]; ok {
			if b, okb := v.(bool); okb {
				cond = b
			}
		}
		thenV, _ := node.Params["then"]
		elseV, _ := node.Params["else"]
		selected := elseV
		branch := "else"
		if cond {
			selected = thenV
			branch = "then"
		}
		return flowschema.Result{
			"branch":   branch,
			"selected": selected,
			"cond":     cond,
		}, nil
	}
}

func execEcho(tag string) NodeExec {
	return func(ctx context.Context, a *Agent, curFlowID string, node *flowschema.Node, in flowschema.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
		return flowschema.Result{"tag": tag, "kind": node.Kind, "use": node.Use, "params": node.Params, "input": in}, nil
	}
}

func execLLM(a *Agent) NodeExec {
	return func(ctx context.Context, _ *Agent, curFlowID string, node *flowschema.Node, in flowschema.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
		// prompt 优先级：node.params.prompt > in.message
		var prompt string
		if s, ok := node.Params["prompt"].(string); ok && strings.TrimSpace(s) != "" {
			prompt = s
		} else if s, ok := in["message"].(string); ok {
			prompt = s
		}
		if strings.TrimSpace(prompt) == "" {
			return flowschema.Result{"content": ""}, nil
		}
		mc := llm.MergeConfig(a.config.LLMConfig, nil)
		cli, err := llm.NewClient(mc.Provider)
		if err != nil {
			return nil, fmt.Errorf("llm provider init failed: %w", err)
		}
		content, callErr := cli.ChatOnce(ctx, mc, prompt)
		if callErr != nil {
			return flowschema.Result{"content": "", "llm_error": callErr.Error()}, nil
		}
		return flowschema.Result{"content": content}, nil
	}
}

// 子工作流：use 或 params.flow_id 指定子 flow
func execWorkflow(a *Agent) NodeExec {
	return func(ctx context.Context, _ *Agent, curFlowID string, node *flowschema.Node, in flowschema.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
		childID := strings.TrimSpace(node.Use)
		if childID == "" {
			if v, ok := node.Params["flow_id"].(string); ok {
				childID = strings.TrimSpace(v)
			}
		}
		if childID == "" {
			return nil, fmt.Errorf("workflow node missing flow_id (use or params.flow_id)")
		}
		// 仅应用显式别名；不做任何字符串替换
		if alt, ok := a.aliasOf(childID); ok {
			childID = alt
		}

		// 子流入参 = 父 in + 节点 params（节点覆盖）
		childIn := mergeCtx(in, node.Params)
		childIn["_caller_flow"] = curFlowID

		res, err := a.Invoke(ctx, childID, childIn, meta)
		if err != nil {
			return nil, fmt.Errorf("invoke subflow(%s) failed: %w", childID, err)
		}
		out := flowschema.Result{
			"child_flow": childID,
			"child_out":  res.Data,
		}
		if id, ok := res.Data["id"]; ok {
			out["child_id"] = id
		}
		return out, nil
	}
}

/* ================= async helpers ================= */

func (a *Agent) setStatus(id string, st *agentschema.ExecutionStatus) {
	a.asyncMu.Lock()
	defer a.asyncMu.Unlock()
	cp := *st
	a.statusByExec[id] = &cp
}

func (a *Agent) setResult(id string, r *agentschema.ExecutionResult) {
	a.asyncMu.Lock()
	defer a.asyncMu.Unlock()
	a.resultByExec[id] = r
}

func (a *Agent) getStart(id string) *time.Time {
	a.asyncMu.RLock()
	defer a.asyncMu.RUnlock()
	if st, ok := a.statusByExec[id]; ok {
		return st.StartedAt
	}
	return nil
}

/* ================= 小工具（支持 IO.InMap/OutMap 点路径） ================= */

func mergeCtx(base flowschema.Context, overlay map[string]any) flowschema.Context {
	out := flowschema.Context{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func getByPathCtx(ctx flowschema.Context, path string) (any, bool) {
	// 允许从 ctx 根取，也允许从 _vars（步骤结果）取
	if v, ok := getByPathMap(map[string]any(ctx), path); ok {
		return v, true
	}
	// 兼容简写：当 path 以 "step_" 开头时，自动从 _vars 下取
	if strings.HasPrefix(path, "step_") {
		if vars, _ := ctx["_vars"].(map[string]any); vars != nil {
			return getByPathMap(vars, path)
		}
	}
	return nil, false
}

func getByPathMap(m map[string]any, path string) (any, bool) {
	cur := any(m)
	for _, p := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		nxt, ok := obj[p]
		if !ok {
			return nil, false
		}
		cur = nxt
	}
	return cur, true
}

func setByPathCtx(ctx flowschema.Context, path string, val any) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return
	}
	cur := map[string]any(ctx)
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = val
			return
		}
		nxt, ok := cur[p]
		if !ok {
			nm := map[string]any{}
			cur[p] = nm
			cur = nm
			continue
		}
		asMap, ok := nxt.(map[string]any)
		if !ok {
			nm := map[string]any{}
			cur[p] = nm
			cur = nm
			continue
		}
		cur = asMap
	}
}

func errMsg(err error, res *agentschema.ExecutionResult) string {
	if err != nil {
		return err.Error()
	}
	if res != nil && !res.Success && res.Error != "" {
		return res.Error
	}
	return ""
}

// LoadFlowAliases 从 JSON 或 YAML 文件加载别名映射：{"from":"to", ...}
// 需要：YAML 需要 gopkg.in/yaml.v3（如果项目里已有）
func (a *Agent) LoadFlowAliases(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read alias file failed: %w", err)
	}
	var m map[string]string

	switch {
	case strings.HasSuffix(strings.ToLower(path), ".json"):
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("parse json failed: %w", err)
		}
	case strings.HasSuffix(strings.ToLower(path), ".yaml"), strings.HasSuffix(strings.ToLower(path), ".yml"):
		type YAML map[string]string
		var y YAML
		if err := yaml.Unmarshal(b, &y); err != nil {
			return fmt.Errorf("parse yaml failed: %w", err)
		}
		m = map[string]string(y)
	default:
		return fmt.Errorf("unsupported alias file ext (use .json/.yaml)")
	}
	a.SetFlowAliases(m)
	return nil
}
