package utils

import (
	"encoding/json"
	"regexp"
	"strings"
)

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func SanitizeID(s string) string {
	s = strings.ToLower(s)
	s = nonAlnum.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func MapToStruct(src map[string]any, dst any) error {
	bs, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(bs, dst)
}

func ToJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// 统一把结果转 map[string]any，便于 OutMap
func ToMap(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case nil:
		return map[string]any{}
	default:
		b, _ := json.Marshal(v)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return m
	}
}

// 从 any 提取 string（不是 string 则返回空串）
func StrFrom(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// 从 any 提取 string（带默认值）
func StrFromOr(v any, def string) string {
	s := StrFrom(v)
	if s != "" {
		return s
	}
	return def
}
