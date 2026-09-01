package bootstrap

import "testing"

func TestExtractManifestResponseGuidanceKeepsModeLabels(t *testing.T) {
	manifest := map[string]interface{}{
		"response_guidance": map[string]interface{}{
			"capability_intro": []interface{}{"介绍当前 Agent 已绑定能力"},
			"general":          "不要输出内部字段",
		},
		"prompt_spec": map[string]interface{}{
			"response_guidance": map[string]interface{}{
				"clarify_params": []interface{}{"只追问缺失字段"},
			},
		},
	}

	got := extractManifestResponseGuidance(manifest)
	want := []string{
		"不要输出内部字段",
		"capability_intro: 介绍当前 Agent 已绑定能力",
		"clarify_params: 只追问缺失字段",
	}
	if len(got) != len(want) {
		t.Fatalf("guidance length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("guidance[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func TestAsStringFromMapTreatsMissingOrNilAsEmpty(t *testing.T) {
	values := map[string]any{
		"nil_value": nil,
		"protocol":  " rest ",
	}

	if got := asStringFromMap(values, "missing"); got != "" {
		t.Fatalf("missing key = %q, want empty", got)
	}
	if got := asStringFromMap(values, "nil_value"); got != "" {
		t.Fatalf("nil value = %q, want empty", got)
	}
	if got := asStringFromMap(values, "protocol"); got != "rest" {
		t.Fatalf("protocol = %q, want rest", got)
	}
}

func TestResolveSkillPreferredProtocolDoesNotDefaultToREST(t *testing.T) {
	if got := resolveSkillPreferredProtocol(nil, nil); got != "" {
		t.Fatalf("preferred protocol = %q, want empty", got)
	}
	if got := resolveSkillPreferredProtocol(map[string]any{"preferred_protocol": " core_internal "}, nil); got != "core_internal" {
		t.Fatalf("preferred protocol from payload = %q, want core_internal", got)
	}
	if got := resolveSkillPreferredProtocol(nil, map[string]any{"preferred_protocol": " grpc "}); got != "grpc" {
		t.Fatalf("preferred protocol from context = %q, want grpc", got)
	}
}
