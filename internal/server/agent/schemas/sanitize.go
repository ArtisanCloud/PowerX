package schemas

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"gorm.io/datatypes"
)

// SanitizePolicy 定义统一的“去循环/裁剪/摘要”策略
type SanitizePolicy struct {
	// 通用
	MaxDepth         int      // 递归最大深度（默认10）
	StringLimit      int      // 字符串最大长度（默认0=不限）
	MaxMapKeys       int      // Map最多保留多少个键（0=不限）
	MaxArrayLen      int      // Slice/Array最多保留多少项（0=不限）
	DropKeys         []string // 全局丢弃的键（比如 _deps/context 等）
	DropKeyPrefix    string   // 丢弃此前缀的键（默认 "_"）
	KeepMetaKeys     []string // Metadata 允许保留的键（其余丢弃）
	StepsSummarize   bool     // 是否对 data.steps 做摘要
	StepsKeepFields  []string // 摘要时保留的字段（默认 use/tag/kind/error）
	StepsInputSample int      // steps.input 采样键数量（默认3）
}

// 一些默认策略
func DefaultSanitizePolicy() SanitizePolicy {
	return SanitizePolicy{
		MaxDepth:         10,
		StringLimit:      0,
		MaxMapKeys:       0,
		MaxArrayLen:      0,
		DropKeys:         []string{"_deps", "_prev_stage", "context", "gin", "writer", "password", "secret", "token", "api_key"},
		DropKeyPrefix:    "_",
		KeepMetaKeys:     []string{"task_id", "flow_id", "plan_id", "is_final", "trace_id"},
		StepsSummarize:   false,
		StepsKeepFields:  []string{"use", "tag", "kind", "error"},
		StepsInputSample: 3,
	}
}

// 专用于“日志入参”的轻量策略
func LogInputPolicy() SanitizePolicy {
	p := DefaultSanitizePolicy()
	p.StringLimit = 256
	p.MaxMapKeys = 10
	p.MaxArrayLen = 10
	return p
}

// 专用于“结果摘要”的轻量策略（对 steps 做摘要）
func ResultSummaryPolicy() SanitizePolicy {
	p := DefaultSanitizePolicy()
	p.StringLimit = 256
	p.MaxMapKeys = 50
	p.MaxArrayLen = 50
	p.StepsSummarize = true
	return p
}

// Sanitizer：带缓存/去循环的执行器
type Sanitizer struct {
	policy SanitizePolicy
}

func NewSanitizer(p SanitizePolicy) *Sanitizer { return &Sanitizer{policy: p} }

// JSON：对任意对象按策略净化后序列化为 datatypes.JSON
func (s *Sanitizer) JSON(v any) datatypes.JSON {
	b, _ := json.Marshal(s.SanitizeAny(v))
	return datatypes.JSON(b)
}

// JSONResult：对 ExecutionResult 做净化（含 metadata 过滤、steps 摘要等）
func (s *Sanitizer) JSONResult(in *ExecutionResult) datatypes.JSON {
	out := s.SanitizeResult(in)
	b, _ := json.Marshal(out)
	return datatypes.JSON(b)
}

// SanitizeResult：对 ExecutionResult 做净化
func (s *Sanitizer) SanitizeResult(in *ExecutionResult) *ExecutionResult {
	if in == nil {
		return nil
	}
	cp := *in
	// Metadata 先按策略收敛
	if cp.Metadata != nil && len(s.policy.KeepMetaKeys) > 0 {
		keep := setFromSlice(s.policy.KeepMetaKeys)
		m := make(map[string]any, len(keep))
		for k, v := range cp.Metadata {
			if _, ok := keep[k]; ok {
				m[k] = v
			}
		}
		cp.Metadata = m
	}
	// Data 走通用 Sanitize，并在需要时摘要 steps
	cp.Data = s.sanitizeData(cp.Data)
	return &cp
}

func (s *Sanitizer) sanitizeData(d map[string]any) map[string]any {
	if d == nil {
		return nil
	}
	// 先通用清洗
	out, _ := s.sanitizeAnyInternal(d, make(map[uintptr]bool), 0)
	if !s.policy.StepsSummarize {
		return out.(map[string]any)
	}

	// 针对 steps 做摘要（仅保留 use/tag/kind/error 和少量 input 片段）
	if raw, ok := out.(map[string]any)["steps"]; ok {
		if mm, ok2 := raw.(map[string]any); ok2 {
			slim := map[string]any{}
			keep := setFromSlice(s.policy.StepsKeepFields)
			for sid, sv := range mm {
				if msv, ok := sv.(map[string]any); ok {
					item := map[string]any{}
					for k, v := range msv {
						if _, ok := keep[k]; ok {
							item[k] = v
						}
					}
					if inp, ok := msv["input"].(map[string]any); ok {
						sample := map[string]any{}
						i := 0
						for ik, iv := range inp {
							sample[ik] = s.limitString(iv)
							i++
							if s.policy.StepsInputSample > 0 && i >= s.policy.StepsInputSample {
								break
							}
						}
						item["input"] = sample
					}
					slim[sid] = item
				}
			}
			out.(map[string]any)["steps"] = slim
		}
	}
	return out.(map[string]any)
}

// SanitizeAny：对任意对象做去循环/限深/删键/限长/采样
func (s *Sanitizer) SanitizeAny(v any) any {
	out, _ := s.sanitizeAnyInternal(v, make(map[uintptr]bool), 0)
	return out
}

func (s *Sanitizer) sanitizeAnyInternal(v any, seen map[uintptr]bool, depth int) (any, bool) {
	if depth > s.policy.MaxDepth && s.policy.MaxDepth > 0 {
		return "[[max-depth]]", true
	}
	if v == nil {
		return nil, true
	}

	switch x := v.(type) {
	case map[string]any:
		rv := reflect.ValueOf(x)
		ptr := rv.Pointer()
		if seen[ptr] {
			return "[[circular]]", true
		}
		seen[ptr] = true

		out := make(map[string]any)
		count := 0
		for k, val := range x {
			if s.dropKey(k) {
				continue
			}
			if s.policy.MaxMapKeys > 0 && count >= s.policy.MaxMapKeys {
				break
			}
			clean, _ := s.sanitizeAnyInternal(val, seen, depth+1)
			out[k] = clean
			count++
		}
		return out, true

	case []any:
		rv := reflect.ValueOf(x)
		ptr := rv.Pointer()
		if seen[ptr] {
			return "[[circular]]", true
		}
		seen[ptr] = true

		n := len(x)
		if s.policy.MaxArrayLen > 0 && n > s.policy.MaxArrayLen {
			n = s.policy.MaxArrayLen
		}
		out := make([]any, n)
		for i := 0; i < n; i++ {
			clean, _ := s.sanitizeAnyInternal(x[i], seen, depth+1)
			out[i] = clean
		}
		return out, true

	case time.Time:
		return x.UTC().Format(time.RFC3339Nano), true
	}

	// 反射兜底
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return fmt.Sprintf("[[%s]]", rv.Kind()), true
	case reflect.Pointer:
		if rv.IsNil() {
			return nil, true
		}
		p := rv.Pointer()
		if seen[p] {
			return "[[circular]]", true
		}
		seen[p] = true
		return s.sanitizeAnyInternal(rv.Elem().Interface(), seen, depth+1)
	case reflect.Map:
		iter := rv.MapRange()
		tmp := make(map[string]any)
		count := 0
		for iter.Next() {
			k := iter.Key()
			val := iter.Value()
			keyStr := k.String()
			if k.Kind() != reflect.String {
				keyStr = fmt.Sprintf("%v", k.Interface())
			}
			if s.dropKey(keyStr) {
				continue
			}
			if s.policy.MaxMapKeys > 0 && count >= s.policy.MaxMapKeys {
				break
			}
			clean, _ := s.sanitizeAnyInternal(val.Interface(), seen, depth+1)
			tmp[keyStr] = clean
			count++
		}
		return tmp, true
	case reflect.Slice, reflect.Array:
		n := rv.Len()
		if s.policy.MaxArrayLen > 0 && n > s.policy.MaxArrayLen {
			n = s.policy.MaxArrayLen
		}
		tmp := make([]any, n)
		for i := 0; i < n; i++ {
			clean, _ := s.sanitizeAnyInternal(rv.Index(i).Interface(), seen, depth+1)
			tmp[i] = clean
		}
		return tmp, true
	case reflect.Struct:
		// 除 time.Time 以外，结构体不展开，避免不可控增长
		if t, ok := v.(time.Time); ok {
			return t.UTC().Format(time.RFC3339Nano), true
		}
		return fmt.Sprintf("[[%T]]", v), true
	default:
		return s.limitString(v), true
	}
}

func (s *Sanitizer) dropKey(k string) bool {
	if s.policy.DropKeyPrefix != "" && strings.HasPrefix(k, s.policy.DropKeyPrefix) {
		return true
	}
	for _, d := range s.policy.DropKeys {
		if d == k {
			return true
		}
	}
	return false
}

func (s *Sanitizer) limitString(v any) any {
	str, ok := v.(string)
	if !ok || s.policy.StringLimit <= 0 {
		return v
	}
	if len(str) <= s.policy.StringLimit {
		return str
	}
	return str[:s.policy.StringLimit] + "…"
}

func setFromSlice(ss []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}
