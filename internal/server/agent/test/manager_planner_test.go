package test

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/intent"
	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/cloudwego/eino/schema"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// —— 测试准备：注册一个最小可用的 Agent Stub —— //
type stubAgent struct{}

func (s *stubAgent) GetInfo() *aschema.AgentInfo {
	now := time.Now()
	return &aschema.AgentInfo{
		AgentID:    "agent_crm",
		Status:     "ready",
		CreatedAt:  now,
		UpdatedAt:  now,
		LastBeatAt: now,
		Runtime:    &aschema.Runtime{},
	}
}

func (s *stubAgent) ListFlows(ctx context.Context, meta aschema.ExecutionMeta) ([]aschema.FlowRuntimeInfo, error) {
	return nil, nil
}

func (s *stubAgent) GetFlowInfo(ctx context.Context, flowID string, meta aschema.ExecutionMeta) (*aschema.FlowRuntimeInfo, error) {
	return nil, nil
}

func (s *stubAgent) ValidateParams(ctx context.Context, flowID string, params flowschema.Context, meta aschema.ExecutionMeta) error {
	return nil
}

func (s *stubAgent) Invoke(ctx context.Context, flowID string, params flowschema.Context, meta aschema.ExecutionMeta) (*aschema.ExecutionResult, error) {
	return &aschema.ExecutionResult{
		Success:   true,
		Data:      flowschema.Result{"ok": true, "flow": flowID, "params": params},
		Duration:  time.Millisecond,
		StepID:    "stub",
		Timestamp: time.Now().Unix(),
	}, nil
}

func (s *stubAgent) InvokeAsync(ctx context.Context, flowID string, params flowschema.Context, meta aschema.ExecutionMeta) (string, error) {
	return "exec_" + strconv.FormatInt(time.Now().UnixNano(), 10), nil
}

// 流式：测试里一般用不到，返回 nil 即可；需要的话可自造个空 reader
func (s *stubAgent) Stream(ctx context.Context, flowID string, params flowschema.Context, meta aschema.ExecutionMeta) (*schema.StreamReader[*aschema.ExecutionResult], error) {
	return nil, nil
}

func (s *stubAgent) GetExecutionStatus(ctx context.Context, executionID string, meta aschema.ExecutionMeta) (*aschema.ExecutionStatus, error) {
	now := time.Now()
	return &aschema.ExecutionStatus{
		PlanID:      executionID,
		Status:      "completed",
		Progress:    100,
		CurrentStep: "",
		StartedAt:   &now,
		CompletedAt: &now,
		Metadata:    map[string]interface{}{"stub": true},
	}, nil
}

func (s *stubAgent) CancelExecution(ctx context.Context, executionID string, meta aschema.ExecutionMeta) error {
	return nil
}

func (s *stubAgent) GetExecutionResult(ctx context.Context, executionID string, meta aschema.ExecutionMeta) (*aschema.ExecutionResult, error) {
	return &aschema.ExecutionResult{
		Success:   true,
		Data:      flowschema.Result{"ok": true, "exec_id": executionID},
		Duration:  0,
		StepID:    "stub",
		Timestamp: time.Now().Unix(),
	}, nil
}

func (s *stubAgent) GetMetrics(ctx context.Context, meta aschema.ExecutionMeta) (flowschema.Result, error) {
	return flowschema.Result{"qps": 0, "latency_ms_p50": 1}, nil
}

func (s *stubAgent) Health(ctx context.Context) error   { return nil }
func (s *stubAgent) Shutdown(ctx context.Context) error { return nil }

// —— 工具：快速注册一个 flow 的 IntentSpec（含 IO 与 requires） —— //
func registerFlow(t *testing.T, m *agent.Manager, agentID, flowID string, requires []string, inputs, outputs []string, keywords []string) {
	t.Helper()
	spec := &flowschema.IntentSpec{
		FlowID: flowID,
		Name:   flowID,
		Matchers: []flowschema.IntentMatcher{
			{Type: flowschema.IntentMatcherKeyword, Any: keywords},
		},
		Examples: flowschema.IntentExamples{Positive: []string{keywords[0]}},
		Domain:   "toy",
		Version:  "1.0.0",
		Metadata: &flowschema.FlowMetadata{
			IO: &flowschema.IODecl{Inputs: inputs, Outputs: outputs},
			ExtraInfo: map[string]string{
				"requires": strings.Join(requires, ","),
			},
		},
	}
	if err := m.RegisterFlowRoute(agentID, flowID, spec); err != nil {
		t.Fatalf("RegisterFlowRoute(%s): %v", flowID, err)
	}
}

// —— 工具：断言 A 在 B 之前（拓扑必要顺序） —— //
func assertBefore(t *testing.T, ordered []string, a, b string) {
	t.Helper()
	pos := map[string]int{}
	for i, id := range ordered {
		pos[id] = i
	}
	ia, oka := pos[a]
	ib, okb := pos[b]
	if !oka || !okb {
		t.Fatalf("ids not found in plan: %s (%v), %s (%v)", a, oka, b, okb)
	}
	if !(ia < ib) {
		t.Fatalf("expected %s before %s, but order is %v", a, b, ordered)
	}
}

// —— 工具：从 plan 取 Flow 顺序（线性） —— //
func planFlowOrder(plan flowschema.ExecutionPlan) []string {
	out := make([]string, 0, len(plan.Tasks))
	for _, t := range plan.Tasks {
		out = append(out, t.FlowID)
	}
	return out
}

// —— 构建一个带 6 个任务的 Manager —— //
// —— 构建一个带 6 个任务的 Manager（规则识别 + Planner布线规则）—— //
func newManagerWith6Flows(t *testing.T) *agent.Manager {
	t.Helper()

	m := agent.GetAgentManager()

	// 为避免单例残留，先清空该 agent 的所有路由（多次跑测不会脏）
	const agentID = "agent_crm"
	m.ClearFlowRoutesByAgent(agentID)

	// 注册一个 stub agent（重复注册直接忽略错误）
	_ = m.Register(agentID, &stubAgent{}, &aschema.AgentMeta{})

	// 1) 安装“意图识别策略”：仅规则策略（关键词/正则）
	//    这样测试不依赖外部向量和 LLM
	// 需要 import:  "github.com/ArtisanCloud/PowerX/internal/server/agent/intent"
	m.SetIntentStrategies([]contract.IntentStrategy{
		&intent.RuleStrategy{M: m},
	}, 0.5, 0.85) // low=0.5(多标签最低分), high=0.85(可留作强匹配阈值)

	// 2) 配置 Planner 布线规则（可选）
	//    - 规则A：显式映射，把上一步输出 id -> 当前输入 upstream_id（高优先级）
	//    - 规则B：自动同名映射（inputs/outputs 名称一致时自动连线，次优先级）
	m.SetPlannerRules([]flowschema.WireRule{
		{
			// FromFlow/ToFlow 留空表示“相邻两任务通用”
			Map:      map[string]string{"id": "upstream_id"},
			Priority: 100, // 高于 AutoByName
		},
		{
			AutoByName: true,
			Priority:   10,
		},
	})

	// 3) 注册 6 个 flow 的 IntentSpec（含 IO 与 requires）
	// 依赖图：
	// t1: 无
	// t2: t1
	// t3: t2
	// t4: t2
	// t5: t3,t4
	// t6: t1（独立支线）
	registerFlow(t, m, agentID, "task_1", nil, []string{}, []string{"id"},
		[]string{"执行任务1", "任务1", "t1"})
	registerFlow(t, m, agentID, "task_2", []string{"task_1"}, []string{"upstream_id"}, []string{"id"},
		[]string{"执行任务2", "任务2", "t2"})
	registerFlow(t, m, agentID, "task_3", []string{"task_2"}, []string{"upstream_id"}, []string{"id"},
		[]string{"执行任务3", "任务3", "t3"})
	registerFlow(t, m, agentID, "task_4", []string{"task_2"}, []string{"upstream_id"}, []string{"id"},
		[]string{"执行任务4", "任务4", "t4"})
	registerFlow(t, m, agentID, "task_5", []string{"task_3", "task_4"}, []string{"upstream_id"}, []string{"id"},
		[]string{"执行任务5", "任务5", "t5"})
	registerFlow(t, m, agentID, "task_6", []string{"task_1"}, []string{"upstream_id"}, []string{"id"},
		[]string{"执行任务6", "任务6", "t6"})

	return m
}

// —— 主测试 —— //
func TestPlanner_MultiIntentAndPrereqs(t *testing.T) {
	m := newManagerWith6Flows(t)

	type TC struct {
		name       string
		message    string
		expectSet  []string    // 至少应包含的 flow 集合（不关心绝对顺序）
		mustBefore [][2]string // 必须满足的部分顺序（拓扑）
	}

	cases := []TC{
		{
			name:      "only t5 -> auto-complete chain",
			message:   "执行任务5",
			expectSet: []string{"task_1", "task_2", "task_3", "task_4", "task_5"},
			mustBefore: [][2]string{
				{"task_1", "task_2"},
				{"task_2", "task_3"},
				{"task_2", "task_4"},
				{"task_3", "task_5"},
				{"task_4", "task_5"},
			},
		},
		{
			name:      "t3 then t5 and t6",
			message:   "我要执行任务3，在执行任务5，执行任务6",
			expectSet: []string{"task_1", "task_2", "task_3", "task_4", "task_5", "task_6"},
			mustBefore: [][2]string{
				{"task_1", "task_2"},
				{"task_2", "task_3"},
				{"task_2", "task_4"},
				{"task_3", "task_5"},
				{"task_4", "task_5"},
				{"task_1", "task_6"},
			},
		},
		{
			name:      "multi in one clause (no punctuation)",
			message:   "执行任务3和任务4以及任务5",
			expectSet: []string{"task_1", "task_2", "task_3", "task_4", "task_5"},
			mustBefore: [][2]string{
				{"task_1", "task_2"},
				{"task_2", "task_3"},
				{"task_2", "task_4"},
				{"task_3", "task_5"},
				{"task_4", "task_5"},
			},
		},
		{
			name:      "duplicates collapsed",
			message:   "执行任务3，然后任务3，再来一次任务3",
			expectSet: []string{"task_1", "task_2", "task_3"},
			mustBefore: [][2]string{
				{"task_1", "task_2"},
				{"task_2", "task_3"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 只做识别 + 依赖补全 + 计划，不执行
			tasks, err := m.DetectTasks(context.Background(), tc.message)
			if err != nil {
				t.Fatalf("DetectTasks error: %v", err)
			}
			if len(tasks) == 0 {
				t.Fatalf("no tasks detected for input: %s", tc.message)
			}
			// 自动补全依赖
			tasks = m.ExpandWithPrereqs(tasks)
			plan := m.BuildPlan(tasks)

			gotOrder := planFlowOrder(plan)

			// 断言包含期望集合
			set := map[string]bool{}
			for _, id := range gotOrder {
				set[id] = true
			}
			for _, want := range tc.expectSet {
				if !set[want] {
					t.Fatalf("plan missing flow %s, order=%v", want, gotOrder)
				}
			}
			// 断言拓扑必要顺序
			for _, pair := range tc.mustBefore {
				assertBefore(t, gotOrder, pair[0], pair[1])
			}
		})
	}
}

// 如果你不愿意暴露内部成员，可以把上面两个“for test”方法删掉，
// 直接把 expandWithPrereqs 的实现复制进测试里（就像上面一样）即可。

// —— 可选：验证“同句多标签”的检测直接输出（不经补全） —— //
func TestDetectTasks_MultiLabelSingleClause(t *testing.T) {
	m := newManagerWith6Flows(t)
	msg := "执行任务3和任务4以及任务5"
	tasks, err := m.DetectTasks(context.Background(), msg)
	if err != nil {
		t.Fatalf("DetectTasks error: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatalf("no tasks detected")
	}
	flows := make([]string, 0, len(tasks))
	for _, tk := range tasks {
		flows = append(flows, tk.FlowID)
	}
	sort.Strings(flows)
	// 这里只要求至少包含 3/4/5，本测试不校验补全
	want := []string{"task_3", "task_4", "task_5"}
	for _, w := range want {
		found := false
		for _, f := range flows {
			if f == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("want flows to include %v, got %v", w, flows)
		}
	}
}
