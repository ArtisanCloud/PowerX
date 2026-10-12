package customer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidText 按 PostgreSQL varchar(n) 的字符语义计长，拒绝非法 UTF-8 和控制字符。
func ValidText(value string, limit int, required bool) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= limit && strings.IndexFunc(value, unicode.IsControl) < 0 && (!required || strings.TrimSpace(value) != "")
}
