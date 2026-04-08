package agent

import (
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
