package utils

import (
	"gorm.io/datatypes"
	"strings"
)

func FirstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func FirstJSONNonEmpty(m datatypes.JSONMap, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok2 := v.(string); ok2 {
				if t := strings.TrimSpace(s); t != "" {
					return t
				}
			}
		}
	}
	return ""
}

func Keys(m map[uint64]struct{}) []uint64 {
	out := make([]uint64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func GetAny(v any, key string) any {
	if m, ok := v.(map[string]any); ok {
		return m[key]
	}
	return nil
}

func GetMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[key]; ok {
		if mv, ok2 := v.(map[string]any); ok2 {
			return mv
		}
	}
	return nil
}

func GetString(v any, key string) string {
	if m, ok := v.(map[string]any); ok {
		if s, ok2 := m[key].(string); ok2 {
			return s
		}
	}
	return ""
}

func GetInt64(v any, key string) int64 {
	if m, ok := v.(map[string]any); ok {
		if i, ok2 := m[key].(int64); ok2 {
			return i
		}
	}
	return 0
}
