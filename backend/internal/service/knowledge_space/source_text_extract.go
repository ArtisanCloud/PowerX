package knowledge_space

import (
	"strings"
)

// extractPlainText walks arbitrary JSON-like objects and concatenates texty fields.
// It is used as a minimal normalizer for API connectors.
func extractPlainText(v any, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 4000
	}
	var parts []string
	walkText(v, func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		parts = append(parts, s)
	})
	out := strings.Join(parts, "\n")
	out = strings.TrimSpace(out)
	if len(out) > maxLen {
		out = out[:maxLen]
	}
	return out
}

func walkText(v any, emit func(string)) {
	switch t := v.(type) {
	case string:
		emit(t)
	case []any:
		for _, it := range t {
			walkText(it, emit)
		}
	case map[string]any:
		// Prefer well-known keys first to reduce noise.
		for _, k := range []string{"plain_text", "content", "title", "name"} {
			if val, ok := t[k]; ok {
				walkText(val, emit)
			}
		}
		for k, val := range t {
			if k == "plain_text" || k == "content" || k == "title" || k == "name" {
				continue
			}
			walkText(val, emit)
		}
	default:
		// ignore other scalar types
	}
}

