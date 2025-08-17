package eino

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/loader"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/services/agent"
	"github.com/ArtisanCloud/PowerX/services/agent/config"
	"github.com/ArtisanCloud/PowerX/services/agent/contract"
	"github.com/ArtisanCloud/PowerX/services/agent/drivers/eino/llm"
	agentschema "github.com/ArtisanCloud/PowerX/services/agent/schemas"

	"sync"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

var _ contract.Agent = (*Agent)(nil)

type Agent struct {
	config    *config.AgentConfig
	agentInfo *agentschema.AgentInfo
	Graph     compose.AnyGraph
	Prompt    string
	Flow      *flowschema.Flow

	// resolver 缓存
	resOnce  sync.Once
	resolver *loader.Resolver
	resErr   error

	// 异步执行（按 PlanID 作为主键）
	execMu       sync.RWMutex
	statusByPlan map[string]*agentschema.ExecutionStatus
	resultByPlan map[string]*agentschema.ExecutionResult
}

func NewAgent(cfg *config.AgentConfig) (contract.Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不能为空")
	}
	g := compose.NewGraph[map[string]any, *schema.Message]()
	now := time.Now().UTC()
	return &Agent{
		config: cfg,
		agentInfo: &agentschema.AgentInfo{ // 名称不从 cfg 取，给默认
			Name:      "eino",
			Status:    string(agentschema.StatusInit),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Graph:        g,
		Prompt:       "",
		Flow:         &flowschema.Flow{},
		statusByPlan: make(map[string]*agentschema.ExecutionStatus),
		resultByPlan: make(map[string]*agentschema.ExecutionResult),
	}, nil
}

/* ------------ internal: resolver ------------ */

func (a *Agent) ensureResolver() error {
	a.resOnce.Do(func() {
		r := loader.NewResolver(a.config.FlowSpec.BusinessDir)
		if err := r.BuildIndex(); err != nil {
			a.resErr = fmt.Errorf("build blueprint index failed: %w", err)
			return
		}
		a.resolver = r
	})
	return a.resErr
}

/* ------------ contract.Agent ------------ */

func (a *Agent) GetInfo() *agentschema.AgentInfo { return a.agentInfo }

func (a *Agent) ListFlows(ctx context.Context, meta agentschema.ExecutionMeta) ([]agentschema.FlowRuntimeInfo, error) {
	// TODO: 如果需要，可在 resolver 暴露 List() 返回所有 flowID，然后逐个 Resolve
	return []agentschema.FlowRuntimeInfo{}, nil
}

func (a *Agent) GetFlowInfo(ctx context.Context, flowID string, meta agentschema.ExecutionMeta) (*agentschema.FlowRuntimeInfo, error) {
	if flowID == "" {
		return nil, fmt.Errorf("flowID 不能为空")
	}
	if err := a.ensureResolver(); err != nil {
		return nil, err
	}
	bp, err := a.resolver.Resolve(flowID) // 合并 base + child
	if err != nil {
		return nil, err
	}

	// 将合并后的 blueprint 转成 Flow（这里直接填主要字段；按你项目结构完善）
	f := flowschema.Flow{
		FlowID: bp.FlowID,
		Name:   bp.Name,
		Nodes:  bp.Nodes,
		// 其它需要的字段按你的 Flow 定义补齐
	}

	return &agentschema.FlowRuntimeInfo{
		Flow:      f,
		Status:    "ready",
		Tags:      nil,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (a *Agent) ValidateParams(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) error {
	// TODO: 结合 JSONSchema 校验；先透传
	return nil
}

func (a *Agent) Invoke(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	if flowID == "" {
		return nil, fmt.Errorf("flowID 不能为空")
	}
	inp, _ := params["_inputs"].(map[string]any)
	vars, _ := params["_vars"].(map[string]any)
	hist := params["_history"] // 任意类型
	_ = inp
	_ = vars
	_ = hist

	start := time.Now()
	out := flowschema.Result{"flow": flowID}
	if debug, _ := params["debug"].(bool); debug {
		out["echo"] = params
	}

	// ===== 1) 兜底聊天 flow（走 LLM，各厂商通用） =====
	// 确保这里的 flowID 与你 SetDefaultAgent(...) 的 defaultFlowID 一致；
	// 我这里宽松兼容三种常见命名：
	if flowID == config.BaseFlowKey {
		// 用户消息
		msg, _ := params["message"].(string)

		// 运行时模型配置（来自 /chat 的 req.Config）
		var rt llm.RuntimeConfig
		if raw, ok := params["model_config"]; ok {
			if m, ok := raw.(map[string]any); ok {
				rt = m
			}
		}

		// 融合默认配置(a.config.LLM) 与 运行时覆盖(rt)
		mc := llm.MergeConfig(a.config.LLMConfig, rt)

		// 创建对应厂商的客户端并调用一次性对话
		if msg != "" {
			cli, err := llm.NewClient(mc.Provider)
			if err == nil {
				if content, callErr := cli.ChatOnce(ctx, mc, msg); callErr == nil && content != "" {
					out["content"] = content
				} else {
					// 调用失败的兜底提示
					out["content"] = "（LLM调用失败，已返回占位文本）" + msg
					if callErr != nil {
						out["llm_error"] = callErr.Error()
					}
				}
			} else {
				out["content"] = "（未知 LLM 提供商）" + msg
				out["llm_error"] = err.Error()
			}
		} else {
			out["content"] = "（没有可回复的用户消息）"
		}

		// 返回兜底对话结果
		return &agentschema.ExecutionResult{
			Success:   true,
			Data:      out,
			Duration:  time.Since(start),
			StepID:    "final",
			Timestamp: time.Now().UTC().Unix(),
			Metadata: flowschema.Result{
				"flow_id":  flowID,
				"plan_id":  meta.RequestID,
				"is_final": true,
			},
		}, nil
	}
	// ===== 2) 正常任务 flow（生成 output.id，供下游布线） =====

	// 如需校验/读蓝图，放在这里
	if err := a.ensureResolver(); err != nil {
		return nil, err
	}
	if _, err := a.resolver.Resolve(flowID); err != nil {
		return nil, fmt.Errorf("resolve flow(%s) failed: %w", flowID, err)
	}

	// 产出一个可被下游引用的 output.id
	var outID string
	if v, ok := params["upstream_id"].(string); ok && v != "" {
		outID = v // 继承上游
	} else {
		outID = fmt.Sprintf("ID_%s_%d", flowID, time.Now().UnixNano())
	}

	out["id"] = outID

	return &agentschema.ExecutionResult{
		Success:   true,
		Data:      out,
		Duration:  time.Since(start),
		StepID:    "final",
		Timestamp: time.Now().UTC().Unix(),
		Metadata: flowschema.Result{
			"flow_id":  flowID,
			"plan_id":  meta.RequestID,
			"is_final": true,
		},
	}, nil
}

func (a *Agent) InvokeAsync(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (string, error) {
	if flowID == "" {
		return "", fmt.Errorf("flowID 不能为空")
	}
	// planID：以 RequestID + 时间戳 生成
	planID := fmt.Sprintf("%s-%d", meta.RequestID, time.Now().UTC().UnixNano())

	now := time.Now().UTC()
	a.setStatus(planID, &agentschema.ExecutionStatus{
		PlanID:      planID,
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
			a.setStatus(planID, &agentschema.ExecutionStatus{
				PlanID:      planID,
				Status:      "failed",
				Progress:    100,
				CurrentStep: "final",
				StartedAt:   a.getStart(planID),
				CompletedAt: &fin,
				Error:       errMsg(err, res),
				Metadata:    map[string]any{"flow_id": flowID},
			})
			return
		}
		a.setResult(planID, res)
		a.setStatus(planID, &agentschema.ExecutionStatus{
			PlanID:      planID,
			Status:      "completed",
			Progress:    100,
			CurrentStep: "final",
			StartedAt:   a.getStart(planID),
			CompletedAt: &fin,
			Metadata:    map[string]any{"flow_id": flowID},
		})
	}()

	return planID, nil
}

func (a *Agent) Stream(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (*schema.StreamReader[*agentschema.ExecutionResult], error) {
	if flowID == "" {
		return nil, fmt.Errorf("flowID 不能为空")
	}
	if err := a.ensureResolver(); err != nil {
		return nil, err
	}
	bp, err := a.resolver.Resolve(flowID)
	if err != nil {
		return nil, fmt.Errorf("resolve flow(%s) failed: %w", flowID, err)
	}

	planID := fmt.Sprintf("%s-%d", meta.RequestID, time.Now().UTC().UnixNano())
	now := time.Now().UTC()
	a.setStatus(planID, &agentschema.ExecutionStatus{
		PlanID:      planID,
		Status:      "running",
		Progress:    0,
		CurrentStep: "start",
		StartedAt:   &now,
		Metadata:    map[string]any{"flow_id": flowID},
	})

	sr, sw := agent.NewResultPipe(16)
	go func() {
		defer sw.Close()

		// start 帧
		start := &agentschema.ExecutionResult{
			Success:   true,
			Data:      flowschema.Result{"message": "start"},
			Duration:  0,
			StepID:    "start",
			Timestamp: time.Now().UTC().Unix(),
			Metadata:  flowschema.Result{"flow_id": flowID, "plan_id": planID, "is_final": false},
		}
		if sw.Send(start, nil) {
			return
		}

		for i, st := range bp.Nodes {
			select {
			case <-ctx.Done():
				// 通知前端错误
				_ = sw.Send(nil, ctx.Err())
				// 更新状态
				fin := time.Now().UTC()
				a.setStatus(planID, &agentschema.ExecutionStatus{
					PlanID:      planID,
					Status:      "cancelled",
					Progress:    float64(i) / float64(max1(len(bp.Nodes))) * 100,
					CurrentStep: fmt.Sprintf("step-%d", i+1),
					StartedAt:   a.getStart(planID),
					CompletedAt: &fin,
					Error:       ctx.Err().Error(),
					Metadata:    map[string]any{"flow_id": flowID},
				})
				return
			default:
			}

			time.Sleep(200 * time.Millisecond)

			isFinal := (i == len(bp.Nodes)-1)
			ch := &agentschema.ExecutionResult{
				Success:   true,
				Data:      flowschema.Result{"step_use": st.Use, "params": st.Params},
				Duration:  200 * time.Millisecond,
				StepID:    fmt.Sprintf("step-%d", i+1),
				Timestamp: time.Now().UTC().Unix(),
				Metadata:  flowschema.Result{"flow_id": flowID, "plan_id": planID, "is_final": isFinal},
			}
			if sw.Send(ch, nil) {
				return
			}

			// 更新进度
			a.execMu.Lock()
			if stt, ok := a.statusByPlan[planID]; ok {
				stt.Progress = float64(i+1) / float64(max1(len(bp.Nodes))) * 100
				stt.CurrentStep = ch.StepID
			}
			a.execMu.Unlock()

			if isFinal {
				fin := time.Now().UTC()
				a.setStatus(planID, &agentschema.ExecutionStatus{
					PlanID:      planID,
					Status:      "completed",
					Progress:    100,
					CurrentStep: ch.StepID,
					StartedAt:   a.getStart(planID),
					CompletedAt: &fin,
					Metadata:    map[string]any{"flow_id": flowID},
				})
				return
			}
		}
	}()

	return sr, nil
}

func (a *Agent) GetExecutionStatus(ctx context.Context, planID string, meta agentschema.ExecutionMeta) (*agentschema.ExecutionStatus, error) {
	a.execMu.RLock()
	defer a.execMu.RUnlock()
	if st, ok := a.statusByPlan[planID]; ok {
		cp := *st
		return &cp, nil
	}
	return nil, fmt.Errorf("plan not found: %s", planID)
}

func (a *Agent) CancelExecution(ctx context.Context, planID string, meta agentschema.ExecutionMeta) error {
	a.execMu.Lock()
	defer a.execMu.Unlock()
	if st, ok := a.statusByPlan[planID]; ok {
		now := time.Now().UTC()
		st.Status = "cancelled"
		st.CompletedAt = &now
		return nil
	}
	return fmt.Errorf("plan not found: %s", planID)
}

func (a *Agent) GetExecutionResult(ctx context.Context, planID string, meta agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	a.execMu.RLock()
	defer a.execMu.RUnlock()
	if r, ok := a.resultByPlan[planID]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("plan result not found: %s", planID)
}

func (a *Agent) GetMetrics(ctx context.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
	uptime := time.Since(a.agentInfo.CreatedAt)
	return flowschema.Result{"uptime_sec": int(uptime.Seconds())}, nil
}

func (a *Agent) Health(ctx context.Context) error   { return nil }
func (a *Agent) Shutdown(ctx context.Context) error { return nil }

/* ------------ helpers ------------ */

func (a *Agent) setStatus(id string, st *agentschema.ExecutionStatus) {
	a.execMu.Lock()
	defer a.execMu.Unlock()
	cp := *st
	a.statusByPlan[id] = &cp
}

func (a *Agent) setResult(id string, r *agentschema.ExecutionResult) {
	a.execMu.Lock()
	defer a.execMu.Unlock()
	a.resultByPlan[id] = r
}

func (a *Agent) getStart(id string) *time.Time {
	a.execMu.RLock()
	defer a.execMu.RUnlock()
	if st, ok := a.statusByPlan[id]; ok {
		return st.StartedAt
	}
	return nil
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

func max1(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}
