package runtime

import "testing"

func TestTraceAttrStringOmitsMissingValues(t *testing.T) {
	attrs := map[string]any{"skill_id": "release.report"}

	if got := traceAttrString(attrs, "capability_id"); got != "" {
		t.Fatalf("missing attribute = %q, want empty string", got)
	}
	if got := traceAttrString(attrs, "skill_id"); got != "release.report" {
		t.Fatalf("skill attribute = %q", got)
	}
}
