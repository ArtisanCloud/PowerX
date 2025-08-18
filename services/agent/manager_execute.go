package agent

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
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

	// ---- 运行日志：PlanStart ----
	m.log().PlanStart(ctx, flow.AgentTaskEvent{
		PlanID:     plan.PlanID,
		Kind:       "plan.start",
		TenantID:   mt.TenantID,
		UserID:     mt.UserID,
		CustomerID: mt.CustomerID,
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
				// 路由到 Agent
				ag, agID, err := m.resolveAgentForTask(task)
				if err != nil {
					// 任务级错误日志
					m.log().TaskErr(egCtx, flow.AgentTaskEvent{
						PlanID: plan.PlanID, TaskID: task.TaskID, FlowID: task.FlowID, Stage: task.Stage,
						AgentID: agID, Kind: "task.err", TenantID: mt.TenantID, UserID: mt.UserID, CustomerID: mt.CustomerID,
						Ts: time.Now(), Error: err.Error(),
					})
					return fmt.Errorf("resolve agent for task(%s/%s) failed: %w", task.TaskID, task.FlowID, err)
				}

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
							AgentID: agID, Kind: "task.err", TenantID: mt.TenantID, UserID: mt.UserID, CustomerID: mt.CustomerID,
							Ts: time.Now(), Error: fmt.Sprintf("param_ref(%s): %v", pk, rerr),
							Input: inSan.JSON(finalParams),
						})
						return fmt.Errorf("param_ref resolve error (%s:%s): %w", task.TaskID, pk, rerr)
					}
					if !ok {
						m.log().TaskErr(egCtx, flow.AgentTaskEvent{
							PlanID: plan.PlanID, TaskID: task.TaskID, FlowID: task.FlowID, Stage: task.Stage,
							AgentID: agID, Kind: "task.err", TenantID: mt.TenantID, UserID: mt.UserID, CustomerID: mt.CustomerID,
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

				// 任务开始日志
				start := time.Now()
				m.log().TaskStart(egCtx, flow.AgentTaskEvent{
					PlanID: plan.PlanID, TaskID: task.TaskID, FlowID: task.FlowID, Stage: task.Stage,
					AgentID: agID, Kind: "task.start", TenantID: mt.TenantID, UserID: mt.UserID, CustomerID: mt.CustomerID,
					Ts: start, Input: inSan.JSON(finalParams),
				})

				// 执行
				out, err := ag.Invoke(egCtx, task.FlowID, ctxVars, mt)
				dur := time.Since(start).Milliseconds()
				if err != nil {
					m.log().TaskErr(egCtx, flow.AgentTaskEvent{
						PlanID: plan.PlanID, TaskID: task.TaskID, FlowID: task.FlowID, Stage: task.Stage,
						AgentID: agID, Kind: "task.err", TenantID: mt.TenantID, UserID: mt.UserID, CustomerID: mt.CustomerID,
						Ts: time.Now(), DurationMS: dur, Error: err.Error(),
					})
					return fmt.Errorf("invoke flow(%s) failed: %w", task.FlowID, err)
				}
				if out != nil {
					if out.Metadata == nil {
						out.Metadata = flowschema.Result{}
					}
					out.Metadata["task_id"] = task.TaskID
					out.Metadata["flow_id"] = task.FlowID
					out.Duration = time.Duration(dur) * time.Millisecond

					results.Put(task.TaskID, task.FlowID, s, out)
					stageOut[i] = out

					// 任务成功日志（输出做摘要&去循环）
					safeOut := outSan.SanitizeResult(out) // 得到瘦身后的 ExecutionResult
					m.log().TaskOK(egCtx, flow.AgentTaskEvent{
						PlanID: plan.PlanID, TaskID: task.TaskID, FlowID: task.FlowID, Stage: task.Stage,
						AgentID: agID, Kind: "task.ok", TenantID: mt.TenantID, UserID: mt.UserID, CustomerID: mt.CustomerID,
						Ts: time.Now(), DurationMS: dur,
						Output: outSan.JSON(safeOut.Data),
						Meta:   outSan.JSON(safeOut.Metadata),
					})
				}
				return nil
			})
		}

		if err := eg.Wait(); err != nil {
			// 计划结束（失败）
			m.log().PlanEnd(ctx, flow.AgentTaskEvent{
				PlanID: plan.PlanID, Kind: "plan.end", TenantID: mt.TenantID, UserID: mt.UserID, CustomerID: mt.CustomerID,
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
		PlanID: plan.PlanID, Kind: "plan.end", TenantID: mt.TenantID, UserID: mt.UserID, CustomerID: mt.CustomerID,
		Ts: time.Now(), Meta: outSan.JSON(map[string]any{"status": "completed"}),
	})

	return finalOut, nil
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
