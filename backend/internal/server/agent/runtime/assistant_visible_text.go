package runtime

import (
	"regexp"
	"strings"
)

var (
	thinkBlockTagRe   = regexp.MustCompile(`(?is)<\s*(think|thinking)\s*>.*?<\s*/\s*(think|thinking)\s*>`)
	thinkSingleTagRe  = regexp.MustCompile(`(?is)<\s*(think|thinking)\s*/\s*>`)
	multiBlankLineRe  = regexp.MustCompile(`\n{3,}`)
	reasoningPrefixes = []string{"思考过程：", "思考过程:", "推理过程：", "推理过程:", "Thought process:", "Reasoning:"}
)

// SanitizeAssistantVisibleText 清理不应展示给用户的推理内容，仅保留可见答案。
func SanitizeAssistantVisibleText(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return ""
	}
	t = thinkBlockTagRe.ReplaceAllString(t, "\n")
	t = thinkSingleTagRe.ReplaceAllString(t, " ")
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	for _, prefix := range reasoningPrefixes {
		if strings.HasPrefix(t, prefix) {
			t = strings.TrimSpace(strings.TrimPrefix(t, prefix))
			break
		}
	}
	t = multiBlankLineRe.ReplaceAllString(t, "\n\n")
	return strings.TrimSpace(t)
}
