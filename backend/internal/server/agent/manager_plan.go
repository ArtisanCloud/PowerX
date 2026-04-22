package agent

// services/agent/manager_plan.go

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"sort"
	"strings"
	"time"
)

// 入口：组装执行计划（依赖补齐 + 分层 + 参数连线 + 稳定排序）
func (m *Manager) BuildPlan(tasks []schemas.DetectedTask) schemas.ExecutionPlan {
	plan := schemas.ExecutionPlan{PlanID: fmt.Sprintf("plan_%d", time.Now().UnixNano())}

	// 1) 建索引：flow -> taskID
	idByFlow := m.indexTaskIDs(tasks)

	// 2) 种子集合与队列
	allFlows, queue := m.seedAllFlows(tasks)

	// 3) 构建 requires 图并自动补齐缺失前置（可能会扩充 tasks 与 idByFlow）
	reqGraph := m.expandGraphAndAutofill(&tasks, idByFlow, allFlows, queue)

	// 4) 计算层级（含轻量环保护）
	level := m.computeLevels(reqGraph, allFlows)

	// 5) 组装 PlanTask（依赖与参数连线）
	plan.Tasks = m.assemblePlanTasks(reqGraph, allFlows, tasks, idByFlow, level)

	// 6) 稳定排序：Stage -> priority(desc) -> firstSeen -> flow_id
	m.sortPlanTasks(&plan.Tasks, tasks)

	return plan
}

/* ========== 步骤函数 ========== */

// flow -> taskID
func (m *Manager) indexTaskIDs(tasks []schemas.DetectedTask) map[string]string {
	idByFlow := make(map[string]string, len(tasks))
	for _, t := range tasks {
		idByFlow[t.FlowID] = t.TaskID
	}
	return idByFlow
}

// 初始化 allFlows 与 BFS 队列
func (m *Manager) seedAllFlows(tasks []schemas.DetectedTask) (map[string]struct{}, []string) {
	allFlows := make(map[string]struct{}, len(tasks))
	q := make([]string, 0, len(tasks))
	for _, t := range tasks {
		pushIfNew(allFlows, &q, t.FlowID)
	}
	return allFlows, q
}

func pushIfNew(all map[string]struct{}, q *[]string, fid string) {
	if _, ok := all[fid]; ok {
		return
	}
	all[fid] = struct{}{}
	*q = append(*q, fid)
}

// 扩展 requires 图，并为缺失的前置插入 auto_prereq 任务
func (m *Manager) expandGraphAndAutofill(
	tasks *[]schemas.DetectedTask,
	idByFlow map[string]string,
	allFlows map[string]struct{},
	queue []string,
) map[string][]string {
	reqGraph := map[string][]string{}
	autoIdx := 0

	for len(queue) > 0 {
		fid := queue[0]
		queue = queue[1:]

		sp, _, ok := m.getSpec(fid)
		if !ok || sp == nil {
			if _, ok := reqGraph[fid]; !ok {
				reqGraph[fid] = nil
			}
			continue
		}

		var requires []string
		if sp.Metadata != nil && len(sp.Metadata.Requires) > 0 {
			requires = append([]string(nil), sp.Metadata.Requires...) // 保序 copy
		}
		reqGraph[fid] = requires

		for _, pre := range requires {
			if pre == "" {
				continue
			}
			if _, seen := allFlows[pre]; !seen {
				// 若未在任务集中，自动补一个前置
				if _, has := idByFlow[pre]; !has {
					if preSp, ag, ok2 := m.getSpec(pre); ok2 && preSp != nil {
						autoIdx++
						tid := fmt.Sprintf("t_auto_%d", autoIdx)
						*tasks = append(*tasks, schemas.DetectedTask{
							TaskID:   tid,
							FlowID:   pre,
							AgentID:  ag,
							Score:    m.intentLow,
							Strategy: "auto_prereq",
							Reason:   "auto-added prerequisite",
						})
						idByFlow[pre] = tid
					}
				}
				pushIfNew(allFlows, &queue, pre)
			}
		}
	}
	return reqGraph
}

// 计算 Stage 层级（DFS + 轻量环保护）
func (m *Manager) computeLevels(reqGraph map[string][]string, allFlows map[string]struct{}) map[string]int {
	level := map[string]int{}
	temp := map[string]bool{}

	var dfs func(string) int
	dfs = func(fid string) int {
		if v, ok := level[fid]; ok {
			return v
		}
		if temp[fid] {
			// 环：就近置 1，避免递归爆栈
			level[fid] = 1
			return 1
		}
		temp[fid] = true
		maxPre := 0
		for _, pre := range reqGraph[fid] {
			if pre == "" {
				continue
			}
			v := dfs(pre)
			if v > maxPre {
				maxPre = v
			}
		}
		temp[fid] = false
		level[fid] = maxPre + 1
		return level[fid]
	}

	for fid := range allFlows {
		_ = dfs(fid)
	}
	return level
}

// 生成 PlanTask：依赖与参数连线（同名 + upstream_id 兜底）
func (m *Manager) assemblePlanTasks(
	reqGraph map[string][]string,
	allFlows map[string]struct{},
	tasks []schemas.DetectedTask,
	idByFlow map[string]string,
	level map[string]int,
) []schemas.PlanTask {

	byFlow := make(map[string]schemas.DetectedTask, len(tasks))
	for _, t := range tasks {
		byFlow[t.FlowID] = t
	}

	out := make([]schemas.PlanTask, 0, len(allFlows))
	for fid := range allFlows {
		t := byFlow[fid]
		params := map[string]any{}
		for k, v := range t.Params {
			params[k] = v
		}
		nodeKind := strings.TrimSpace(readTaskNodeKind(t))
		if nodeKind == "" {
			nodeKind = "workflow"
		}
		nodeRef := strings.TrimSpace(readTaskNodeRef(t))
		if nodeRef == "" {
			nodeRef = t.FlowID
		}
		sourceScope := strings.TrimSpace(readTaskSourceScope(t))
		if sourceScope == "" {
			sourceScope = "system"
		}
		pt := schemas.PlanTask{
			TaskID:        t.TaskID,
			FlowID:        t.FlowID,
			NodeKind:      nodeKind,
			NodeRef:       nodeRef,
			SourceScope:   sourceScope,
			AgentID:       t.AgentID,
			TeamID:        readTaskTeamID(t),
			HandoffTaskID: readTaskHandoffTaskID(t),
			FailurePolicy: readTaskFailurePolicy(t),
			ContextRefID:  readTaskContextRefID(t),
			Params:        params,
			ParamRefs:     map[string]string{},
			Stage:         level[fid],
		}

		// 依赖（按 requires 顺序）
		for _, pre := range reqGraph[fid] {
			if pre == "" {
				continue
			}
			if tid, ok := idByFlow[pre]; ok {
				pt.DependsOn = append(pt.DependsOn, tid)
			}
		}

		// 参数连线
		spTo, _, _ := m.getSpec(fid)
		inSet, _ := m.getIOSets(spTo)

		wiredAny := false
		for _, pre := range reqGraph[fid] {
			if pre == "" {
				continue
			}
			preTid, ok := idByFlow[pre]
			if !ok {
				continue
			}
			spFrom, _, _ := m.getSpec(pre)
			_, outSet := m.getIOSets(spFrom)

			for name := range outSet {
				if _, need := inSet[name]; need {
					if _, already := pt.ParamRefs[name]; !already {
						pt.ParamRefs[name] = fmt.Sprintf("{{task.%s.output.%s}}", preTid, name)
						wiredAny = true
					}
				}
			}
		}

		// 兜底：upstream_id <- 最后一个前置的 id
		if _, needUp := inSet["upstream_id"]; needUp {
			if _, already := pt.ParamRefs["upstream_id"]; !already && len(reqGraph[fid]) > 0 {
				lastPre := reqGraph[fid][len(reqGraph[fid])-1]
				if lastTid, ok := idByFlow[lastPre]; ok {
					pt.ParamRefs["upstream_id"] = fmt.Sprintf("{{task.%s.output.id}}", lastTid)
					wiredAny = true
				}
			}
		}
		// 兜底：*_id <- 最后一个前置的 id
		if !wiredAny && len(reqGraph[fid]) > 0 {
			lastPre := reqGraph[fid][len(reqGraph[fid])-1]
			if lastTid, ok := idByFlow[lastPre]; ok {
				for inName := range inSet {
					if strings.HasSuffix(inName, "_id") && pt.ParamRefs[inName] == "" {
						pt.ParamRefs[inName] = fmt.Sprintf("{{task.%s.output.id}}", lastTid)
					}
				}
			}
		}

		out = append(out, pt)
	}
	return out
}

func readTaskNodeKind(t schemas.DetectedTask) string {
	if t.Params == nil {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t.Strategy)), "tool_calling:") {
			return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(t.Strategy)), "tool_calling:")
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t.Strategy)), "candidate_recall:") {
			return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(t.Strategy)), "candidate_recall:")
		}
		return ""
	}
	if v, ok := t.Params["_node_kind"]; ok {
		if s := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v))); s != "" {
			return s
		}
	}
	return ""
}

func readTaskNodeRef(t schemas.DetectedTask) string {
	if t.Params == nil {
		return ""
	}
	if v, ok := t.Params["_node_ref"]; ok {
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return ""
}

func readTaskSourceScope(t schemas.DetectedTask) string {
	if t.Params == nil {
		return ""
	}
	if v, ok := t.Params["_source_scope"]; ok {
		return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
	}
	return ""
}

func readTaskTeamID(t schemas.DetectedTask) string {
	if t.Params == nil {
		return ""
	}
	if v, ok := t.Params["_team_id"]; ok {
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return ""
}

func readTaskHandoffTaskID(t schemas.DetectedTask) string {
	if t.Params == nil {
		return ""
	}
	if v, ok := t.Params["_handoff_task_id"]; ok {
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return ""
}

func readTaskFailurePolicy(t schemas.DetectedTask) string {
	if t.Params == nil {
		return ""
	}
	if v, ok := t.Params["_failure_policy"]; ok {
		return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
	}
	return ""
}

func readTaskContextRefID(t schemas.DetectedTask) string {
	if t.Params == nil {
		return ""
	}
	if v, ok := t.Params["_context_ref_id"]; ok {
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return ""
}

// 排序：Stage → priority(desc) → firstSeen → flow_id
func (m *Manager) sortPlanTasks(planTasks *[]schemas.PlanTask, detected []schemas.DetectedTask) {
	ts := *planTasks

	// 首次出现顺序（auto_prereq 视为很后）
	firstSeen := map[string]int{}
	for i, t := range detected {
		if _, ok := firstSeen[t.FlowID]; !ok {
			firstSeen[t.FlowID] = i
		}
	}
	const autoBase = 1_000_000
	for i := range ts {
		fid := ts[i].FlowID
		if _, ok := firstSeen[fid]; !ok {
			firstSeen[fid] = autoBase + i
		}
	}

	getPrio := func(flowID string) int {
		sp, _, ok := m.getSpec(flowID)
		if !ok || sp == nil || sp.Metadata == nil {
			return 0
		}
		return sp.Metadata.Priority
	}

	sort.SliceStable(ts, func(i, j int) bool {
		ai, aj := ts[i], ts[j]
		if ai.Stage != aj.Stage {
			return ai.Stage < aj.Stage
		}
		pi, pj := getPrio(ai.FlowID), getPrio(aj.FlowID)
		if pi != pj {
			return pi > pj
		}
		si, sj := firstSeen[ai.FlowID], firstSeen[aj.FlowID]
		if si != sj {
			return si < sj
		}
		return ai.FlowID < aj.FlowID
	})

	*planTasks = ts
}
