package knowledge_space

import "testing"

func TestSplitByRuneWindowPreferSeparators_PrefersSeparatorBoundary(t *testing.T) {
	text := "一句话很长很长很长很长很长很长。下一句也很长很长很长很长很长很长。最后一句。"
	seps := []string{"。"}

	// Force window small enough to require splitting, and ensure it prefers ending at "。"
	window := 18
	parts := splitByRuneWindowPreferSeparators(text, window, 4, seps)
	if len(parts) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(parts))
	}
	minLen := int(float64(window) * 0.6)
	if minLen < 1 {
		minLen = 1
	}
	for i, p := range parts[:len(parts)-1] {
		if p == "" {
			t.Fatalf("chunk %d empty", i)
		}
		if p[len(p)-len("。"):] != "。" {
			if got := len([]rune(p)); got < minLen {
				t.Fatalf("chunk %d expected separator or min length %d, got %d: %q", i, minLen, got, p)
			}
		}
	}
}
