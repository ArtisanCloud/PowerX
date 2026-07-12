package agent

import (
	"strings"
	"testing"
)

func TestRenderBriefCapabilityContextKeepsCapabilityFacts(t *testing.T) {
	raw := strings.Join([]string{
		"- 能力 A",
		"  说明: 处理当前 Agent 已绑定的领域能力。",
		"  可用动作: action_a, action_b",
		"  必要参数: field_a, field_b",
		"  可选参数: q, page, page_size",
		"  示例问法: 帮我执行能力 A",
		"  回复规范: 根据当前 Agent 的表达风格回答，不输出内部实现字段",
		"  ref: capability.a",
	}, "\n")

	out := renderBriefCapabilityContext(raw)
	for _, want := range []string{"能力 A", "说明:", "可用动作:", "必要参数:", "示例问法:", "回复规范:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in brief context:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ref:") || strings.Contains(out, "可选参数:") {
		t.Fatalf("brief context leaked hidden fields:\n%s", out)
	}
}
