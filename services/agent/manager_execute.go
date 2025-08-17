package agent

import (
	"context"
	"errors"
	"fmt"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/services/agent/contract"
	aschema "github.com/ArtisanCloud/PowerX/services/agent/schemas"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"
)

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
	return tasks
}

/****************
 * ExecutePlan
 ****************/

func (m *Manager) ExecutePlan(ctx context.Context, plan flowschema.ExecutionPlan, mt aschema.ExecutionMeta) (*aschema.ExecutionResult, error) {
	if len(plan.Tasks) == 0 {
		return nil, errors.New("empty plan")
	}

	// 计划级超时
	var cancel context.CancelFunc
	if mt.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, mt.Timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	// 结果存储（既可按 taskID 取，也能按 flowID 取“最近一次”）
	results := aschema.NewResultStore()

	// Stage 分组并升序执行
	stageMap := map[int][]flowschema.PlanTask{}
	var stages []int
	for _, t := range plan.Tasks {
		stageMap[t.Stage] = append(stageMap[t.Stage], t)
	}
	for s := range stageMap {
		stages = append(stages, s)
	}
	sort.Ints(stages)

	var last *aschema.ExecutionResult

	for _, s := range stages {
		eg, egCtx := errgroup.WithContext(ctx)
		stageTasks := stageMap[s]

		for i := range stageTasks {
			task := stageTasks[i] // 避免闭包变量问题

			eg.Go(func() error {
				// 1) 路由到 Agent
				ag, _, err := m.resolveAgentForTask(task)
				if err != nil {
					return fmt.Errorf("resolve agent for task(%s/%s) failed: %w", task.TaskID, task.FlowID, err)
				}

				// 2) 构造最终入参：Params + ParamRefs(模板) 物化
				finalParams := make(map[string]interface{}, len(task.Params))
				for k, v := range task.Params {
					finalParams[k] = v
				}

				// 注入依赖输出，便于 flow 内直接取用
				depMap := results.GetMany(task.DependsOn) // map[taskID]*ExecutionResult
				ctxVars := make(flowschema.Context, len(finalParams)+2)
				ctxVars["_deps"] = depMap

				// 解析 ParamRefs：如 {"lead_id": "{{task.s1.output.id}}"} 或 {"lead_id": "{{task.lead_create.output.id}}"}
				for pk, ref := range task.ParamRefs {
					val, ok, rerr := aschema.ResolveParamRef(ref, results, task)
					if rerr != nil {
						return fmt.Errorf("param_ref resolve error (%s:%s): %w", task.TaskID, pk, rerr)
					}
					if !ok {
						return fmt.Errorf("param_ref not found (%s:%s) => %s", task.TaskID, pk, ref)
					}
					finalParams[pk] = val
				}

				// 把最终参数放进 ctxVars（flow 侧按键取用）
				for k, v := range finalParams {
					ctxVars[k] = v
				}

				// 3) 执行
				start := time.Now()
				out, err := ag.Invoke(egCtx, task.FlowID, ctxVars, mt)
				if err != nil {
					return fmt.Errorf("invoke flow(%s) failed: %w", task.FlowID, err)
				}
				if out != nil {
					// 补充一些常用 metadata
					if out.Metadata == nil {
						out.Metadata = flowschema.Result{}
					}
					out.Metadata["task_id"] = task.TaskID
					out.Metadata["flow_id"] = task.FlowID
					out.Duration = time.Since(start)

					results.Put(task.TaskID, task.FlowID, s, out)
					last = out
				}
				return nil
			})
		}

		if err := eg.Wait(); err != nil {
			return nil, err
		}
	}

	return last, nil
}

/*************************
 * helpers: route & store
 *************************/

func (m *Manager) resolveAgentForTask(t flowschema.PlanTask) (contract.Agent, string, error) {
	// 1) task.AgentID 优先
	if t.AgentID != "" {
		m.mu.RLock()
		ag := m.agents[t.AgentID]
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
		ag := m.agents[rec.AgentID]
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

func (m *Manager) GetDefaultRoute() (contract.Agent, string, error) {
	if m.defaultAgID == "" || m.defaultFlowID == "" {
		return nil, "", errors.New("default agent/flow is not set")
	}
	m.mu.RLock()
	ag := m.agents[m.defaultAgID]
	m.mu.RUnlock()
	if ag == nil {
		return nil, "", fmt.Errorf("default agent not found: %s", m.defaultAgID)
	}
	return ag, m.defaultFlowID, nil
}
