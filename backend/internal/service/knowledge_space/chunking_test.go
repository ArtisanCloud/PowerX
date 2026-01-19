package knowledge_space

import "testing"

func TestSplitByRuneWindowPreferSeparators_PrefersSeparatorBoundary(t *testing.T) {
	text := "一句话很长很长很长很长很长很长。下一句也很长很长很长很长很长很长。最后一句。"
	seps := []string{"。"}

	// Force window small enough to require splitting, and ensure it prefers ending at "。"
	parts := splitByRuneWindowPreferSeparators(text, 18, 4, seps)
	if len(parts) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(parts))
	}
	for i, p := range parts[:len(parts)-1] {
		if p == "" {
			t.Fatalf("chunk %d empty", i)
		}
		if p[len(p)-len("。"):] != "。" {
			t.Fatalf("chunk %d should end with separator '。', got %q", i, p)
		}
	}
}
