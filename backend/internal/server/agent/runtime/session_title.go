package runtime

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var thinkTagRe = regexp.MustCompile(`(?is)<\s*think\s*>.*?<\s*/\s*think\s*>`)
var mdFenceRe = regexp.MustCompile("(?s)```.+?```")

// MakeDefaultSessionTitle 从首条用户消息推个标题。
// - 优先用首行/首句的前 maxRunes 字符；
// - 清理 <think>...</think>、Markdown 代码块、重复空白；
// - 兜底 "新的会话 MM-DD HH:mm".
func MakeDefaultSessionTitle(msg string, maxRunes int) string {
	s := strings.TrimSpace(msg)
	if s == "" {
		return "新的会话 " + time.Now().Format("01-02 15:04")
	}
	// 去掉隐藏推理/代码块
	s = thinkTagRe.ReplaceAllString(s, " ")
	s = mdFenceRe.ReplaceAllString(s, " ")
	// 取首行/句
	for _, sep := range []string{"\n", "。", "!", "！", "?", "？"} {
		if i := strings.Index(s, sep); i >= 0 && i < len(s) {
			s = s[:i]
			break
		}
	}
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "新的会话 " + time.Now().Format("01-02 15:04")
	}
	// 限长（按 rune）
	if maxRunes <= 0 {
		maxRunes = 24
	}
	if utf8.RuneCountInString(s) > maxRunes {
		rs := []rune(s)
		s = string(rs[:maxRunes]) + "…"
	}
	return s
}
