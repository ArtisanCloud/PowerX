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
