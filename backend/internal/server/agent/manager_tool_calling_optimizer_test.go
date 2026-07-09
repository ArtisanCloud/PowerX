package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestSelectPlannerCandidates_TopKAndQuota(t *testing.T) {
	cands := make([]ToolCallCandidate, 0, 40)
	for i := 0; i < 20; i++ {
		cands = append(cands, ToolCallCandidate{
			Name:        "skill.test." + string(rune('a'+(i%26))) + strings.Repeat("x", i/26),
			NodeKind:    "skill",
			NodeRef:     "skill.test.ref",
			Description: "incident analyze and fix",
		})
	}
	for i := 0; i < 10; i++ {
		cands = append(cands, ToolCallCandidate{
			Name:        "tooling.test." + string(rune('a'+(i%26))),
			NodeKind:    "tooling",
			NodeRef:     "tooling.test.ref",
			Description: "incident tooling",
		})
	}
	for i := 0; i < 10; i++ {
		cands = append(cands, ToolCallCandidate{
			Name:        "workflow.test." + string(rune('a'+(i%26))),
			NodeKind:    "workflow",
			NodeRef:     "workflow.test.ref",
			Description: "incident workflow",
		})
	}

	out := selectPlannerCandidates("please fix incident with tooling", cands, PlannerOptimizerConfig{
		Enabled:       true,
		CandidateTopK: 12,
		PerKindQuota: PlannerKindQuota{
			Workflow: 2,
			Skill:    6,
			Tooling:  4,
			LLM:      1,
		},
		PromptSlimMode: "compact",
	})
	if len(out) != 12 {
		t.Fatalf("expected 12 candidates, got %d", len(out))
	}
	counts := map[string]int{}
	for _, c := range out {
		counts[strings.ToLower(strings.TrimSpace(c.NodeKind))]++
	}
	if counts["skill"] < 6 {
		t.Fatalf("expected at least 6 skills, got %d", counts["skill"])
	}
	if counts["tooling"] < 4 {
		t.Fatalf("expected at least 4 toolings, got %d", counts["tooling"])
	}
	if counts["workflow"] < 2 {
		t.Fatalf("expected at least 2 workflows, got %d", counts["workflow"])
	}
}

func TestBuildToolCallingPrompt_CompactShorterThanVerbose(t *testing.T) {
	cands := []ToolCallCandidate{
		{
			Name:         "skill.thirdparty.prompt-template",
			NodeKind:     "skill",
			NodeRef:      "skill.thirdparty.prompt-template",
			SourceScope:  "system",
			Description:  strings.Repeat("Render template with many details. ", 20),
			RequiredArgs: []string{"template"},
			OptionalArgs: []string{"variables"},
		},
		{
			Name:         "skill.thirdparty.hello-echo",
			NodeKind:     "skill",
			NodeRef:      "skill.thirdparty.hello-echo",
			SourceScope:  "system",
			Description:  strings.Repeat("Echo text directly. ", 20),
			OptionalArgs: []string{"text"},
		},
	}
	compact := buildToolCallingPrompt("输出模板结果", cands, PlannerOptimizerConfig{
		Enabled:        true,
		PromptSlimMode: "compact",
	})
	verbose := buildToolCallingPrompt("输出模板结果", cands, PlannerOptimizerConfig{
		Enabled:        true,
		PromptSlimMode: "verbose",
	})
	if len(compact) >= len(verbose) {
		t.Fatalf("expected compact prompt shorter than verbose, compact=%d verbose=%d", len(compact), len(verbose))
	}
	if strings.Contains(compact, "schema.1") {
		t.Fatalf("compact prompt should not contain verbose schema lines")
	}
	if !strings.Contains(verbose, "schema.1") {
		t.Fatalf("verbose prompt should contain schema lines")
	}
}

func TestScoreCandidateForQuery_AliasMatch(t *testing.T) {
	c := ToolCallCandidate{
		Name:        "skill.thirdparty.hello-echo",
		NodeKind:    "skill",
		NodeRef:     "skill.thirdparty.hello-echo",
		Description: "Echo text directly",
	}
	score := scoreCandidateForQuery("请调用 hello-echo，把文本原样返回", c)
	if score <= 0 {
		t.Fatalf("expected alias hit score > 0, got %d", score)
	}
}

func TestBuildToolCallCandidatesWithContextRestrictsToAgentBindings(t *testing.T) {
	m := NewAgentManager()
	m.UpsertUnifiedCandidate(ToolCallCandidate{
		Name:        "powerxplugin.template.basic",
		NodeKind:    "skill",
		NodeRef:     "powerxplugin.template.basic",
		SourceScope: "system",
		Source:      "plugin",
		Visibility:  "public",
	})
	m.UpsertUnifiedCandidate(ToolCallCandidate{
		Name:        "global.unbound.skill",
		NodeKind:    "skill",
		NodeRef:     "global.unbound.skill",
		SourceScope: "system",
		Source:      "plugin",
		Visibility:  "public",
	})
	m.UpsertUnifiedCandidate(ToolCallCandidate{
		Name:        "global.unbound.tool",
		NodeKind:    "tooling",
		NodeRef:     "global.unbound.tool",
		SourceScope: "system",
		Source:      "builtin",
		Visibility:  "public",
	})

	out := m.BuildToolCallCandidatesWithContext(CandidateBuildContext{
		AgentID:       "8",
		BoundSkillIDs: []string{"powerxplugin.template.basic"},
	}, 0)
	if len(out) != 1 {
		t.Fatalf("expected only one bound candidate, got %d: %+v", len(out), out)
	}
	if out[0].NodeKind != "skill" || out[0].NodeRef != "powerxplugin.template.basic" {
		t.Fatalf("expected bound template skill only, got %+v", out[0])
	}
}

func TestDetectTasksFromUnifiedCandidatesRecallsBoundTemplateSkill(t *testing.T) {
	m := NewAgentManager()
	m.SetIntentStrategies(nil, 0.1, 0.6)
	m.UpsertUnifiedCandidate(ToolCallCandidate{
		Name:        "powerxplugin.template.basic.local",
		DisplayName: "模板对象基础能力",
		NodeKind:    "skill",
		NodeRef:     "powerxplugin.template.basic.local",
		FlowID:      "powerxplugin.template.basic.local",
		SourceScope: "agent",
		Source:      "plugin",
		Description: "管理 PowerXPlugin 的基础模板对象。该对象仅包含标题、描述和内容，用于开发者验证插件侧 CRUD、能力注册和 Agent 调用链路。",
		IntentHints: []string{
			"帮我创建一个标题为测试模板的模板，描述是用于验证插件 CRUD，内容是这是一条测试内容",
			"列出所有模板",
		},
		Actions: []string{"create", "get", "update", "delete", "list"},
		ActionRequiredArgs: map[string][]string{
			"create": {"template.title", "template.description", "template.content"},
		},
	})

	ctx := contextWithBoundSkillIDs([]string{"powerxplugin.template.basic.local"})
	tasks := m.detectTasksFromUnifiedCandidates(ctx, "生成一个标题为活动公告的模板")
	if len(tasks) != 1 {
		t.Fatalf("expected one recalled task, got %d: %#v", len(tasks), tasks)
	}
	if tasks[0].FlowID != "powerxplugin.template.basic.local" {
		t.Fatalf("unexpected flow id: %#v", tasks[0])
	}
	if got := strings.TrimSpace(tasks[0].Params["action"].(string)); got != "create" {
		t.Fatalf("expected create action, got %q params=%#v", got, tasks[0].Params)
	}
	if got := strings.TrimSpace(fmt.Sprint(tasks[0].Params["user_message"])); got != "生成一个标题为活动公告的模板" {
		t.Fatalf("expected user_message to be preserved, got %q params=%#v", got, tasks[0].Params)
	}
}

func contextWithBoundSkillIDs(ids []string) context.Context {
	return context.WithValue(context.Background(), "agent_bound_skill_ids", ids)
}
