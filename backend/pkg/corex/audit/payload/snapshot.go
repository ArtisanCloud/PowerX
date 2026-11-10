// Package payload 只处理审计副本，不修改执行入参、业务结果或授权决定。
package payload

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	MaxPayloadBytes = 16 << 10
	MaxErrorBytes   = 2 << 10
	maxDecodeBytes  = 64 << 10
	maxStringBytes  = 1024
	maxEntries      = 32
	maxNodes        = 512
	maxDepth        = 8
)

var (
	bearer      = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=\-]+`)
	jwt         = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
	urls        = regexp.MustCompile(`(?i)\b(?:https?|postgres(?:ql)?|mysql|redis|mongodb(?:\+srv)?|s3)://[^\s"<>]+`)
	assignments = regexp.MustCompile(`(?i)(["']?(?:password|passwd|secret|token|authorization|cookie|set_cookie|client_secret|access_token|refresh_token|api[_-]?key|access[_-]?key|private[_-]?key|context_ticket|invocation_ticket|ws_revalidation_handle)["']?\s*[:=]\s*)("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^\s,;&}\]]+)`)
	headers     = regexp.MustCompile(`(?i)\b(?:set-cookie|cookie|authorization)\s*:[^\r\n]+`)
)

type report struct {
	Version  int    `json:"version"`
	Mode     string `json:"mode"`
	Redacted int    `json:"redacted"`
	Omitted  int    `json:"omitted"`
	MaxBytes int    `json:"max_bytes"`
}
type walker struct {
	report
	nodes int
}

// Snapshot 保留有界诊断结构；长正文、凭据和大集合仅保留明确省略标记。
func Snapshot(value any) json.RawMessage {
	data, _ := encode(value, MaxPayloadBytes, "bounded")
	return data
}

// Summary 标记经过类型化精简的副本，仍执行与普通载荷相同的保密与大小检查。
func Summary(value any) json.RawMessage {
	data, _ := encode(value, MaxPayloadBytes, "summary")
	return data
}

// JSON 校验已有 JSON 副本；不先解析无限大小输入，max 只能进一步缩小硬上限。
func JSON(raw []byte, max int) ([]byte, bool) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, false
	}
	if len(raw) > maxDecodeBytes {
		data, _ := encode(map[string]any{"detail_omitted": true, "original_bytes": len(raw)}, max, "oversize_json")
		return data, true
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF {
		data, _ := encode(map[string]any{"detail_omitted": true}, max, "invalid_json")
		return data, true
	}
	return encode(value, max, "bounded")
}

func encode(value any, max int, mode string) (json.RawMessage, bool) {
	if max <= 0 || max > MaxPayloadBytes {
		max = MaxPayloadBytes
	}
	if max < 256 {
		max = 256
	}
	w := walker{report: report{Version: 1, Mode: mode, MaxBytes: max}}
	clean := w.walk(reflect.ValueOf(value), 0)
	if clean == nil {
		return json.RawMessage("null"), false
	}
	object, ok := clean.(map[string]any)
	if !ok {
		object = map[string]any{"value": clean}
	}
	if _, exists := object["_audit"]; exists {
		w.Omitted++
	}
	object["_audit"] = w.report
	raw, err := json.Marshal(object)
	if err != nil || len(raw) > max {
		minimal := map[string]any{}
		for _, key := range []string{"operation", "schema", "status", "code", "error_code"} {
			if text, ok := object[key].(string); ok && len(text) <= 128 {
				minimal[key] = text
			}
		}
		w.Mode = "budget_omitted"
		w.Omitted++
		minimal["_audit"] = w.report
		raw, _ = json.Marshal(minimal)
		if len(raw) > max {
			raw, _ = json.Marshal(map[string]any{"_audit": w.report})
		}
	}
	return raw, w.Redacted > 0 || w.Omitted > 0
}

func (w *walker) omit(kind string, n int) any {
	w.Omitted++
	return map[string]any{"_omitted": kind, "size": n}
}
func normalized(key string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
}
func sensitive(key string) bool {
	k := normalized(key)
	if k == "token" || k == "tokens" || k == "auth" {
		return true
	}
	for _, s := range []string{"password", "passwd", "secret", "authorization", "cookie", "accesstoken", "refreshtoken", "bearertoken", "apikey", "accesskey", "privatekey", "credential", "contextticket", "invocationticket", "wsrevalidationhandle", "connectionstring"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	return k == "dsn" || k == "signature"
}
func content(key string) bool {
	switch normalized(key) {
	case "prompt", "messages", "content", "text", "html", "base64", "rawbody", "filecontent", "documentcontent", "attachment":
		return true
	}
	return false
}
func contentValue(key string, v reflect.Value) bool {
	if content(key) {
		return true
	}
	if normalized(key) != "body" {
		return false
	}
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	return v.IsValid() && (v.Kind() == reflect.String || v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8)
}

func (w *walker) walk(v reflect.Value, depth int) any {
	w.nodes++
	if w.nodes > maxNodes || depth > maxDepth {
		return w.omit("traversal_limit", 0)
	}
	if !v.IsValid() {
		return nil
	}
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
		depth++
		if depth > maxDepth {
			return w.omit("depth", 0)
		}
	}
	if v.CanInterface() {
		if t, ok := v.Interface().(time.Time); ok {
			return t.UTC().Format(time.RFC3339Nano)
		}
		if n, ok := v.Interface().(json.Number); ok {
			if _, e := n.Float64(); e == nil {
				return n
			}
			return w.omit("invalid_number", 0)
		}
	}
	switch v.Kind() {
	case reflect.String:
		s := v.String()
		if len(s) > maxStringBytes {
			return w.omit("text", len(s))
		}
		trimmed := strings.TrimSpace(s)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var decoded any
			d := json.NewDecoder(strings.NewReader(trimmed))
			d.UseNumber()
			if d.Decode(&decoded) != nil || d.Decode(new(any)) != io.EOF {
				return w.omit("encoded_text", len(s))
			}
			return w.walk(reflect.ValueOf(decoded), depth+1)
		}
		clean := SafeText(s)
		if clean != s {
			w.Redacted++
		}
		return clean
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint()
	case reflect.Float32, reflect.Float64:
		n := v.Float()
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return w.omit("invalid_number", 0)
		}
		return n
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		if v.Type().Key().Kind() != reflect.String || v.Len() > maxEntries {
			return w.omit("object", v.Len())
		}
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		out := map[string]any{}
		for _, key := range keys {
			name := key.String()
			if len(name) > 128 {
				w.Omitted++
				continue
			}
			if sensitive(name) {
				out[name] = "[REDACTED]"
				w.Redacted++
			} else if contentValue(name, v.MapIndex(key)) {
				out[name] = w.omit("content", 0)
			} else {
				out[name] = w.walk(v.MapIndex(key), depth+1)
			}
		}
		return out
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}
		if v.Type().Elem().Kind() == reflect.Uint8 {
			if v.Len() > maxDecodeBytes {
				return w.omit("bytes", v.Len())
			}
			data := make([]byte, v.Len())
			// uint8 的命名类型同样允许；reflect.Copy 会因元素类型不同而 panic。
			for i := range data {
				data[i] = byte(v.Index(i).Uint())
			}
			var decoded any
			d := json.NewDecoder(bytes.NewReader(data))
			d.UseNumber()
			if d.Decode(&decoded) != nil || d.Decode(new(any)) != io.EOF {
				return w.omit("bytes", v.Len())
			}
			return w.walk(reflect.ValueOf(decoded), depth+1)
		}
		if v.Len() > maxEntries {
			return w.omit("array", v.Len())
		}
		out := make([]any, v.Len())
		for i := range out {
			out[i] = w.walk(v.Index(i), depth+1)
		}
		return out
	case reflect.Struct:
		if v.NumField() > maxEntries {
			return w.omit("struct", v.NumField())
		}
		out := map[string]any{}
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if f.PkgPath != "" {
				continue
			}
			tag := f.Tag.Get("json")
			name := strings.Split(tag, ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			if strings.Contains(tag, ",omitempty") && v.Field(i).IsZero() {
				continue
			}
			if sensitive(name) {
				out[name] = "[REDACTED]"
				w.Redacted++
			} else if contentValue(name, v.Field(i)) {
				out[name] = w.omit("content", 0)
			} else {
				out[name] = w.walk(v.Field(i), depth+1)
			}
		}
		return out
	default:
		return w.omit("unsupported", 0)
	}
}

// SafeText 仅用于审计错误副本；不改业务返回错误。超长内容和多行正文不写入数据库。
func SafeText(raw string) string {
	if strings.Contains(raw, "PRIVATE KEY-----") {
		return "[REDACTED PRIVATE KEY]"
	}
	if len(raw) > MaxErrorBytes {
		return "[error detail omitted: exceeds byte limit]"
	}
	text := strings.SplitN(raw, "\n", 2)[0]
	text = headers.ReplaceAllString(text, "[REDACTED HEADER]")
	text = bearer.ReplaceAllString(text, "$1 [REDACTED]")
	text = jwt.ReplaceAllString(text, "[REDACTED JWT]")
	text = urls.ReplaceAllString(text, "[REDACTED URL]")
	text = assignments.ReplaceAllString(text, "${1}[REDACTED]")
	if text != raw && strings.Contains(raw, "\n") {
		text += " [additional lines omitted]"
	}
	if len(text) > MaxErrorBytes {
		return "[error detail omitted: redacted byte limit]"
	}
	return text
}
