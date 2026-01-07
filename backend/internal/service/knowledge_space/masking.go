package knowledge_space

import (
	"regexp"
	"strings"
)

type Masker struct {
	strict bool
}

func NewMasker(profile string) Masker {
	p := strings.ToLower(strings.TrimSpace(profile))
	return Masker{
		strict: strings.Contains(p, "strict") || strings.Contains(p, "required"),
	}
}

func (m Masker) Apply(chunks []IngestionChunk) ([]IngestionChunk, float64, bool) {
	if len(chunks) == 0 {
		return nil, 0, false
	}
	out := make([]IngestionChunk, 0, len(chunks))

	email := regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}`)
	for _, chunk := range chunks {
		c := chunk
		if m.strict && strings.Contains(strings.ToUpper(c.Content), "UNMASKABLE") {
			return out, 0, true
		}
		c.Content = email.ReplaceAllString(c.Content, "[REDACTED_EMAIL]")
		c.Masked = true
		out = append(out, c)
	}
	return out, 100.0, false
}
