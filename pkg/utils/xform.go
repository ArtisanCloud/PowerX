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

// 辅助：把 any 转数值（JSON 反序列化 float64 的兼容）
func AsInt16(v any) (int16, bool) {
	switch t := v.(type) {
	case float64:
		return int16(t), true
	case int:
		return int16(t), true
	case int64:
		return int16(t), true
	case int32:
		return int16(t), true
	}
	return 0, false
}
func AsUint64(v any) (uint64, bool) {
	switch t := v.(type) {
	case float64:
		return uint64(t), true
	case int:
		if t < 0 {
			return 0, false
		}
		return uint64(t), true
	case int64:
		if t < 0 {
			return 0, false
		}
		return uint64(t), true
	case uint64:
		return t, true
	}
	return 0, false
}

func IfZeroInt16(v int16, def int16) int16 {
	if v == 0 {
		return def
	}
	return v
}

func IfZeroInt16Ptr(p *int16, def int16) int16 {
	if p == nil || *p == 0 {
		return def
	}
	return *p
}

func TrimLower(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func Trim(s string) string      { return strings.TrimSpace(s) }

func ToStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func UniqUint64(in []uint64) []uint64 {
	m := make(map[uint64]struct{}, len(in))
	out := make([]uint64, 0, len(in))
	for _, v := range in {
		if v == 0 {
			continue
		}
		if _, ok := m[v]; !ok {
			m[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}
