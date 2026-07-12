package agent

import (
	"strings"
	"testing"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
)

func TestRenderAgentProfilePromptPrioritizesAgentIdentity(t *testing.T) {
	prompt := renderAgentProfilePrompt(&dbmodel.Agent{
		Name:        "模板智能体",
		Description: "面向插件开发者和管理员的 PowerXPlugin 基础模板对象管理智能体。",
		Persona:     "你是 PowerXPlugin 基础模板对象管理助手。",
		PromptSeed:  "当用户询问你是谁或能做什么时，请以基础模板对象管理助手身份回答。",
	})

	for _, want := range []string{
		"[AGENT_PROFILE]",
		"你正在扮演当前数据库 Agent",
		"不得回答“我是通义千问”",
		"模板智能体",
		"PowerXPlugin 基础模板对象管理助手",
		"基础模板对象管理助手身份回答",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("profile prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestJoinPromptSectionsSkipsEmptySections(t *testing.T) {
	got := joinPromptSections(" base ", "", " profile ")
	if got != "base\n\nprofile" {
		t.Fatalf("unexpected joined prompt: %q", got)
	}
}
