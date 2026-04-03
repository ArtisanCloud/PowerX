// File: services/agent/drivers/eino/agent.go
package eino

import (
	"context"
	"fmt"
	config2 "github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/config"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/factory/llm"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/loader"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/dto"

	"github.com/cloudwego/eino/schema"
)

/* ================= 类型定义 ================= */
type ctxKey int

const ctxTokenEmit ctxKey = 1001

type TokenEmitter func(delta string)
type tokenEmitterKey struct{}

func withTokenEmitter(ctx context.Context, emit TokenEmitter) context.Context {
	return context.WithValue(ctx, ctxTokenEmit, emit)
}
func tokenEmitterFrom(ctx context.Context) TokenEmitter {
	if v := ctx.Value(ctxTokenEmit); v != nil {
		if f, ok := v.(TokenEmitter); ok {
			return f
		}
	}
	return nil
}

// 每种节点的执行器签名（注意 node 是 *flowschema.Node）
type NodeExec func(
	ctx context.Context,
	a *AgentClient,
	curFlowID string,
	node *flowschema.Node,
	in flowschema.Context,
	meta agentschema.ExecutionMeta,
) (flowschema.Result, error)

var _ contract.AgentClient = (*AgentClient)(nil)

/* ================= Agent ================= */

type AgentClient struct {
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

	deltaMu   sync.RWMutex
	deltaHook func(s string)

	// 异步执行
	asyncMu      sync.RWMutex
	statusByExec map[string]*agentschema.ExecutionStatus
	resultByExec map[string]*agentschema.ExecutionResult
}

/* ================= 构造 / 注册 ================= */

func NewAgentClient(cfg *config.AgentConfig) (contract.AgentClient, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不能为空")
	}
	r := loader.NewResolver(cfg.FlowSpec.BusinessDir)
	if err := r.BuildIndex(); err != nil {
		return nil, fmt.Errorf("build blueprint index failed: %w", err)
	}
	now := time.Now().UTC()

	a := &AgentClient{
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
func (a *AgentClient) RegisterFunc(use string, exec NodeExec) {
	a.execMu.Lock()
	a.useExecs[strings.TrimSpace(use)] = exec
	a.execMu.Unlock()
}

// 注册内置 kind 的最小可用执行器（全部使用常量，统一小写 key）
func (a *AgentClient) registerBuiltins() {
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

func (a *AgentClient) getOrBuildFlow(flowID string) (*flowschema.Flow, error) {
	// 命中缓存
	a.flowMu.RLock()
	if f := a.flowCache[flowID]; f != nil {
		a.flowMu.RUnlock()
		fmt.Printf("[agent.eino] getOrBuildFlow: cache hit id=%s nodes=%d\n", flowID, len(f.Nodes))
		return f, nil
	}
	a.flowMu.RUnlock()

	// 先应用别名（如有）
	if alt, ok := a.aliasOf(flowID); ok && strings.TrimSpace(alt) != "" {
		flowID = alt
	}

	fmt.Printf("[agent.eino] getOrBuildFlow: resolving id=%s\n", flowID)

	// 尝试从蓝图解析
	f, err := a.resolver.Resolve(flowID)
	if err != nil || f == nil || len(f.Nodes) == 0 {
		// 如果是兜底 flow，则动态构建一个“单 LLM 节点”的最小可用流程
		if strings.EqualFold(flowID, config.BaseFlowKey) {
			f = a.buildFallbackBaseFlow(flowID)
			fmt.Printf("[agent.eino] getOrBuildFlow: build FALLBACK base_flow id=%s nodes=%d\n", flowID, len(f.Nodes))
		} else {
			if err == nil {
				err = fmt.Errorf("resolve flow(%s) failed: empty", flowID)
			}
			fmt.Printf("[agent.eino] getOrBuildFlow: resolve FAILED id=%s err=%v\n", flowID, err)
			return nil, fmt.Errorf("resolve flow(%s) failed: %w", flowID, err)
		}
	} else {
		fmt.Printf("[agent.eino] getOrBuildFlow: resolve OK id=%s nodes=%d\n", f.FlowID, len(f.Nodes))
	}

	// 缓存
	a.flowMu.Lock()
	a.flowCache[flowID] = f
	a.flowMu.Unlock()
	return f, nil
}

// 动态构建兜底 base_flow（只有 1 个 LLM 节点）
func (a *AgentClient) buildFallbackBaseFlow(flowID string) *flowschema.Flow {
	return &flowschema.Flow{
		FlowID: flowID,
		Nodes: []*flowschema.Node{
			{
				Kind: flowschema.KindLLM,
				Use:  "chat",
				Params: flowschema.Context{
					// 可选：把全局 message 映射为该节点的 prompt；也可不配（execLLM 会兜底用 in.message）
					"_io_in":  map[string]string{"prompt": "message"},
					"_io_out": map[string]string{"content": "_last.answer"}, // 可选：把回答写回上下文
					// 也可以在此放默认模型/温度（留空则 execLLM 用全局 cfg）
					// "model": a.config.LLMConfig.Model,
					// "temperature": a.config.LLMConfig.Temperature,
				},
			},
		},
	}
}

/* ================= contract.Agent ================= */

func (a *AgentClient) GetInfo() *agentschema.AgentInfo { return a.agentInfo }

func (a *AgentClient) ListFlows(ctx context.Context, meta agentschema.ExecutionMeta) ([]agentschema.FlowRuntimeInfo, error) {
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

func (a *AgentClient) GetFlowInfo(ctx context.Context, flowID string, meta agentschema.ExecutionMeta) (*agentschema.FlowRuntimeInfo, error) {
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

func (a *AgentClient) ValidateParams(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) error {
	// TODO：如需 JSONSchema 校验在此实现
	return nil
}

func (a *AgentClient) Invoke(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
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

func (a *AgentClient) InvokeAsync(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (string, error) {
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

func (a *AgentClient) Stream(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (*schema.StreamReader[*agentschema.ExecutionResult], error) {
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
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("panic in stream: %v", r)
				_ = sw.Send(nil, err)
				fin := time.Now().UTC()
				a.setStatus(execID, &agentschema.ExecutionStatus{
					PlanID:      execID,
					Status:      "failed",
					Progress:    0,
					CurrentStep: "panic",
					StartedAt:   a.getStart(execID),
					CompletedAt: &fin,
					Error:       err.Error(),
					Metadata: map[string]any{
						"flow_id": flowID,
						"stack":   string(debug.Stack()),
					},
				})
			}
		}()

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

			// 👇 注入 token 发射器，让 execLLM 可以实时上报
			ctxWithEmit := withTokenEmitter(ctx, func(delta string) {
				if strings.TrimSpace(delta) == "" {
					return
				}
				_ =
					sw.Send(&agentschema.ExecutionResult{
						Success:   true,
						Data:      nil, // token 走 metadata，不占 data
						Duration:  0,
						StepID:    stepKey,
						Timestamp: time.Now().UTC().Unix(),
						Metadata: flowschema.Result{
							"flow_id":    flowID,
							"plan_id":    execID,
							"is_final":   false,
							"delta_text": delta, // 👈 Engine 看到这个就会 emit "token"
						},
					}, nil)
			})

			exec := a.pickExecutor(n)
			res, err := exec(ctxWithEmit, a, flow.FlowID, n, in, meta)
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
					if val, ok := getByPathMap(res, fromKey); ok {
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

func (a *AgentClient) GetExecutionStatus(ctx context.Context, executionID string, meta agentschema.ExecutionMeta) (*agentschema.ExecutionStatus, error) {
	a.asyncMu.RLock()
	defer a.asyncMu.RUnlock()
	if st, ok := a.statusByExec[executionID]; ok {
		cp := *st
		return &cp, nil
	}
	return nil, fmt.Errorf("plan not found: %s", executionID)
}

func (a *AgentClient) CancelExecution(ctx context.Context, executionID string, meta agentschema.ExecutionMeta) error {
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

func (a *AgentClient) GetExecutionResult(ctx context.Context, executionID string, meta agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	a.asyncMu.RLock()
	defer a.asyncMu.RUnlock()
	if r, ok := a.resultByExec[executionID]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("plan result not found: %s", executionID)
}

func (a *AgentClient) GetMetrics(ctx context.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
	uptime := time.Since(a.agentInfo.CreatedAt)
	return flowschema.Result{"uptime_sec": int(uptime.Seconds())}, nil
}

func (a *AgentClient) Health(ctx context.Context) error   { return nil }
func (a *AgentClient) Shutdown(ctx context.Context) error { return nil }

/* ================= 分派 & 内置执行器 ================= */

func (a *AgentClient) pickExecutor(n *flowschema.Node) NodeExec {
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
	key := strings.ToLower(strings.TrimSpace(string(n.Kind)))
	a.execMu.RLock()
	fn := a.kindExecs[key]
	a.execMu.RUnlock()
	if fn != nil {
		return fn
	}
	return execEcho("default")
}

func execNoop(tag string) NodeExec {
	return func(ctx context.Context, a *AgentClient, curFlowID string, node *flowschema.Node, in flowschema.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
		return flowschema.Result{"tag": tag, "kind": node.Kind, "use": node.Use, "params": node.Params}, nil
	}
}

// 最小 selector：params.cond(in.cond) 为真取 then，否则 else
func execSelector() NodeExec {
	return func(ctx context.Context, a *AgentClient, curFlowID string, node *flowschema.Node, in flowschema.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
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
	return func(ctx context.Context, a *AgentClient, curFlowID string, node *flowschema.Node, in flowschema.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
		return flowschema.Result{"tag": tag, "kind": node.Kind, "use": node.Use, "params": node.Params, "input": in}, nil
	}
}

func execLLM(a *AgentClient) NodeExec {
	return func(ctx context.Context, _ *AgentClient, curFlowID string, node *flowschema.Node, in flowschema.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
		// 1) prompt
		var prompt string
		if s, ok := node.Params["prompt"].(string); ok && strings.TrimSpace(s) != "" {
			prompt = s
		} else if s, ok := in["message"].(string); ok {
			prompt = s
		}
		if strings.TrimSpace(prompt) == "" {
			return flowschema.Result{"content": ""}, nil
		}

		// 2) client & config
		if a == nil || a.config == nil {
			return nil, fmt.Errorf("llm config missing: agent client not initialized")
		}
		mc, err := resolveLLMConfig(node, in, a)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(mc.Provider) == "" {
			return nil, fmt.Errorf("llm config missing provider")
		}
		if strings.TrimSpace(mc.Model) == "" {
			return nil, fmt.Errorf("llm config missing model")
		}
		applyLLMCacheStrategy(mc.Provider, mc)
		cli, err := llm.NewClient(mc.Provider)
		if err != nil {
			return nil, fmt.Errorf("llm provider init failed: %w", err)
		}

		// 3) 是否需要流式：从上下文拿 emitter（注意要用 tokenEmitterFrom，而不是 tokenEmitterKey{}）
		emitter := tokenEmitterFrom(ctx)

		// 4) 若需要流式，优先尝试 provider 原生流
		if emitter != nil {
			if final, err := cli.Stream(ctx, mc, prompt, func(delta string) {
				if strings.TrimSpace(delta) != "" {
					emitter(delta)
				}
			}); err == nil {
				return flowschema.Result{"content": final}, nil
			}

			// 5) 原生流不可用 -> 回退到同步 + 本地分块回放
			invokeResult, invErr := cli.Invoke(ctx, mc, prompt)
			if invErr != nil {
				return nil, fmt.Errorf("llm invoke failed: %w", invErr)
			}
			content := ""
			if invokeResult != nil {
				content = invokeResult.Text
			}

			// 分块配置：优先节点参数，可不配就用默认
			chunkSize := 8
			if v, ok := node.Params["stream_chunk"].(int); ok && v > 0 {
				chunkSize = v
			}
			delayMs := 20
			if v, ok := node.Params["stream_delay_ms"].(int); ok && v >= 0 {
				delayMs = v
			}

			rs := []rune(content)
			for i := 0; i < len(rs); i += chunkSize {
				select {
				case <-ctx.Done():
					return flowschema.Result{"content": content}, ctx.Err()
				default:
				}
				j := i + chunkSize
				if j > len(rs) {
					j = len(rs)
				}
				emitter(string(rs[i:j]))
				if delayMs > 0 {
					select {
					case <-ctx.Done():
						return flowschema.Result{"content": content}, ctx.Err()
					case <-time.After(time.Duration(delayMs) * time.Millisecond):
					}
				}
			}
			return flowschema.Result{"content": content}, nil
		}

		// 6) 上层不需要流式 -> 直接同步调用
		invokeResult, err := cli.Invoke(ctx, mc, prompt)
		if err != nil {
			return nil, fmt.Errorf("llm invoke failed: %w", err)
		}
		content := ""
		if invokeResult != nil {
			content = invokeResult.Text
		}
		return flowschema.Result{"content": content}, nil
	}
}

func resolveLLMConfig(node *flowschema.Node, in flowschema.Context, a *AgentClient) (*config2.ModelConfig, error) {
	// Priority (low -> high):
	// 1) driver-level fallback: agent.llm (config.yaml) —— 仅用于“调用方未注入 config”的兜底
	// 2) runtime request: dto.ChatConfig (in["config"]) —— 通常已包含「AI settings 默认 + Agent 自身配置 + 本次请求覆盖」
	// 3) node params overrides —— 节点级最高优先级
	var mc *config2.ModelConfig

	// 2) runtime request config (ChatConfig)
	if in != nil {
		if v := in["config"]; v != nil {
			switch vv := v.(type) {
			case *dto.ChatConfig:
				mc = config2.MergeConfig(mc, modelConfigFromChatConfig(vv))
			case dto.ChatConfig:
				tmp := vv
				mc = config2.MergeConfig(mc, modelConfigFromChatConfig(&tmp))
			case map[string]any:
				mc = config2.MergeConfig(mc, modelConfigFromChatConfig(mapToChatConfig(vv)))
			}
		}
	}

	// 1) driver-level fallback (only when caller didn't inject config)
	if mc == nil && a != nil && a.config != nil {
		mc = config2.MergeConfig(mc, a.config.LLMConfig)
	}

	// 3) node params overrides
	if node != nil && len(node.Params) > 0 {
		mc = config2.MergeConfig(mc, modelConfigFromNodeParams(node.Params))
	}

	if mc == nil {
		return nil, fmt.Errorf("llm config missing")
	}
	return mc, nil
}

func modelConfigFromChatConfig(c *dto.ChatConfig) *config2.ModelConfig {
	if c == nil {
		return nil
	}
	var extra map[string]any
	if mode := strings.ToLower(strings.TrimSpace(c.CacheMode)); mode != "" {
		extra = map[string]any{
			"cache_mode": mode,
		}
	}
	return &config2.ModelConfig{
		Provider:     strings.TrimSpace(c.Provider),
		Endpoint:     strings.TrimSpace(c.Endpoint),
		APIKey:       strings.TrimSpace(c.APIKey),
		Model:        strings.TrimSpace(c.ModelName),
		SystemPrompt: strings.TrimSpace(c.SystemPrompt),
		Temperature:  c.Temperature,
		MaxTokens:    c.MaxTokens,
		Extra:        extra,
	}
}

func mapToChatConfig(m map[string]any) *dto.ChatConfig {
	if m == nil {
		return nil
	}
	getStr := func(key string) string {
		if v, ok := m[key]; ok {
			if s, ok2 := v.(string); ok2 {
				return s
			}
		}
		return ""
	}
	getF := func(key string) float64 {
		if v, ok := m[key]; ok {
			switch vv := v.(type) {
			case float64:
				return vv
			case float32:
				return float64(vv)
			case int:
				return float64(vv)
			case int64:
				return float64(vv)
			}
		}
		return 0
	}
	getI := func(key string) int {
		if v, ok := m[key]; ok {
			switch vv := v.(type) {
			case int:
				return vv
			case int64:
				return int(vv)
			case float64:
				return int(vv)
			}
		}
		return 0
	}
	return &dto.ChatConfig{
		ModelName:    strings.TrimSpace(getStr("model_name")),
		Provider:     strings.TrimSpace(getStr("provider")),
		Endpoint:     strings.TrimSpace(getStr("endpoint")),
		APIKey:       strings.TrimSpace(getStr("api_key")),
		Temperature:  getF("temperature"),
		MaxTokens:    getI("max_tokens"),
		SystemPrompt: strings.TrimSpace(getStr("system_prompt")),
		CacheMode:    strings.TrimSpace(getStr("cache_mode")),
	}
}

func supportsPromptCache(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai", "anthropic", "google", "gemini":
		return true
	default:
		return false
	}
}

type PromptCacheStrategy struct {
	Mode      string `json:"mode"`
	Supported bool   `json:"supported"`
	Enabled   bool   `json:"enabled"`
}

func ResolvePromptCacheStrategy(provider string, mode string) PromptCacheStrategy {
	normalized := normalizeCacheMode(mode)
	supported := supportsPromptCache(provider)
	enabled := false
	switch normalized {
	case "force_on":
		enabled = true
	case "force_off":
		enabled = false
	default:
		enabled = supported
	}
	return PromptCacheStrategy{
		Mode:      normalized,
		Supported: supported,
		Enabled:   enabled,
	}
}

func normalizeCacheMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "auto", "force_off", "force_on":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return "auto"
	}
}

func applyLLMCacheStrategy(provider string, mc *config2.ModelConfig) {
	if mc == nil {
		return
	}
	if mc.Extra == nil {
		mc.Extra = map[string]any{}
	}
	decision := ResolvePromptCacheStrategy(provider, fmt.Sprintf("%v", mc.Extra["cache_mode"]))
	mc.Extra["cache_mode"] = decision.Mode
	mc.Extra["cache_supported"] = decision.Supported
	mc.Extra["cache_enabled"] = decision.Enabled
}

func modelConfigFromNodeParams(params map[string]any) *config2.ModelConfig {
	if params == nil {
		return nil
	}
	out := &config2.ModelConfig{}

	setStr := func(dst *string, keys ...string) {
		for _, k := range keys {
			if v, ok := params[k]; ok {
				if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
					*dst = strings.TrimSpace(s)
					return
				}
			}
		}
	}
	setFloat := func(dst *float64, keys ...string) {
		for _, k := range keys {
			if v, ok := params[k]; ok {
				switch vv := v.(type) {
				case float64:
					*dst = vv
					return
				case float32:
					*dst = float64(vv)
					return
				case int:
					*dst = float64(vv)
					return
				case int64:
					*dst = float64(vv)
					return
				}
			}
		}
	}
	setInt := func(dst *int, keys ...string) {
		for _, k := range keys {
			if v, ok := params[k]; ok {
				switch vv := v.(type) {
				case int:
					*dst = vv
					return
				case int64:
					*dst = int(vv)
					return
				case float64:
					*dst = int(vv)
					return
				}
			}
		}
	}

	setStr(&out.Provider, "provider", "llm_provider")
	setStr(&out.Endpoint, "endpoint", "base_url", "llm_endpoint")
	setStr(&out.APIKey, "api_key", "apikey", "llm_api_key")
	setStr(&out.Model, "model", "model_name", "llm_model")
	setStr(&out.SystemPrompt, "system_prompt", "prompt_system")
	setFloat(&out.Temperature, "temperature", "temp")
	setInt(&out.MaxTokens, "max_tokens", "maxTokens")

	return out
}

// 子工作流：use 或 params.flow_id 指定子 flow
func execWorkflow(a *AgentClient) NodeExec {
	return func(ctx context.Context, _ *AgentClient, curFlowID string, node *flowschema.Node, in flowschema.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
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
