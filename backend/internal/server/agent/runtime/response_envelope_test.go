package runtime

import "testing"

func validResponseEnvelope() map[string]any {
	return map[string]any{
		"schema": AgentResponseEnvelopeSchema, "kind": "multi_agent_summary", "outcome": "needs_action",
		"presentation": map[string]any{
			"facts":      []any{map[string]any{"statement": "已收到活动原始数据。", "source": map[string]any{"type": "input", "ref": "input:message"}}},
			"metrics":    []any{map[string]any{"label": "点击率", "numerator": "36000", "denominator": "1200000", "formula": "36000/1200000", "display_value": "3%", "source": map[string]any{"type": "input", "ref": "input:message"}}},
			"hypotheses": []any{"直播停留时长可能影响转化，需要补充分场次数据验证。"},
			"gaps":       []any{"缺少直播停留时长的原始数据。"},
			"actions":    []any{"补充各场直播的停留时长和成交数据后再验证。"},
		},
	}
}

func TestValidateAgentResponseEnvelope(t *testing.T) {
	if _, err := ValidateAgentResponseEnvelope(validResponseEnvelope()); err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}
}

func TestValidateAgentResponseEnvelopeRejectsLegacyAndFreeTextPresentation(t *testing.T) {
	invalid := validResponseEnvelope()
	invalid["summary_refs"] = []any{"fact:1"}
	if _, err := ValidateAgentResponseEnvelope(invalid); err == nil {
		t.Fatal("legacy summary_refs must be rejected")
	}
	invalid = validResponseEnvelope()
	presentation := invalid["presentation"].(map[string]any)
	presentation["acceptance"] = []any{"自定义验收阈值"}
	if _, err := ValidateAgentResponseEnvelope(invalid); err == nil {
		t.Fatal("skill-owned acceptance must be rejected")
	}
	invalid = validResponseEnvelope()
	facts := invalid["presentation"].(map[string]any)["facts"].([]any)
	facts[0].(map[string]any)["id"] = "fact:1"
	if _, err := ValidateAgentResponseEnvelope(invalid); err == nil {
		t.Fatal("legacy item ids must be rejected")
	}
}

func TestValidateAgentResponseEnvelopeRejectsIncorrectPercentage(t *testing.T) {
	invalid := validResponseEnvelope()
	metrics := invalid["presentation"].(map[string]any)["metrics"].([]any)
	metrics[0].(map[string]any)["display_value"] = "2.5%"
	if _, err := ValidateAgentResponseEnvelope(invalid); err == nil {
		t.Fatal("incorrect percentage must be rejected")
	}
}

func TestValidateAgentResponseEnvelopeRejectsCompletedWithOpenWork(t *testing.T) {
	invalid := validResponseEnvelope()
	invalid["outcome"] = "completed"
	if _, err := ValidateAgentResponseEnvelope(invalid); err == nil {
		t.Fatal("completed result with hypotheses or gaps must be rejected")
	}
}

func TestResponseEnvelopeFromExecutionResultRequiresExplicitEnvelope(t *testing.T) {
	got, err := responseEnvelopeFromExecutionResult(map[string]any{"result": map[string]any{"content": "raw markdown"}})
	if err != nil {
		t.Fatalf("absence is distinguished from invalid envelope: %v", err)
	}
	if got != nil {
		t.Fatalf("raw markdown must not be auto-wrapped: %#v", got)
	}
}
