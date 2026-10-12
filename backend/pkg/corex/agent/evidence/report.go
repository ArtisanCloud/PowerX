package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const DraftSchema = "powerx.agent.response-draft/v1"
const ReportSchema = "powerx.agent.response/v4"

type Source struct {
	Pointer string `json:"pointer"`
	Quote   string `json:"quote"`
	Literal string `json:"literal"`
}
type Datum struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Unit   string `json:"unit"`
	Scope  string `json:"scope"`
	Kind   string `json:"kind"`
	Source Source `json:"source"`
}

// DraftDatum has only one numeric source of truth. The platform derives Value.
type DraftDatum struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Unit   string `json:"unit"`
	Scope  string `json:"scope"`
	Kind   string `json:"kind"`
	Source Source `json:"source"`
}
type Calculation struct {
	Key        string            `json:"key"`
	Label      string            `json:"label"`
	Expression string            `json:"expression"`
	Bindings   map[string]string `json:"bindings"`
	Precision  int               `json:"precision"`
	Percent    bool              `json:"percent"`
	CompareTo  string            `json:"compare_to"`
}
type Draft struct {
	Schema       string        `json:"schema"`
	Kind         string        `json:"kind"`
	Data         []DraftDatum  `json:"data"`
	Calculations []Calculation `json:"calculations"`
	Hypotheses   []string      `json:"hypotheses"`
	Gaps         []string      `json:"gaps"`
	Actions      []string      `json:"actions"`
}
type Execution struct {
	UUID         string           `json:"uuid"`
	ToolKey      string           `json:"tool_key"`
	ToolVersion  string           `json:"tool_version"`
	TenantUUID   string           `json:"tenant_uuid"`
	RevisionUUID string           `json:"revision_uuid"`
	TraceID      string           `json:"trace_id"`
	SourceDigest string           `json:"source_digest"`
	Request      Calculation      `json:"request"`
	Operands     map[string]Datum `json:"operands"`
	DisplayValue string           `json:"display_value"`
	StartedAt    time.Time        `json:"started_at"`
	EndedAt      time.Time        `json:"ended_at"`
}
type Conflict struct {
	CalculationKey  string `json:"calculation_key"`
	ReportedKey     string `json:"reported_key"`
	ReportedValue   string `json:"reported_value"`
	CalculatedValue string `json:"calculated_value"`
}
type Report struct {
	Schema       string       `json:"schema"`
	Kind         string       `json:"kind"`
	Outcome      string       `json:"outcome"`
	Presentation Presentation `json:"presentation"`
	SourceDigest string       `json:"source_digest"`
	TenantUUID   string       `json:"tenant_uuid"`
	RevisionUUID string       `json:"revision_uuid"`
	TraceID      string       `json:"trace_id"`
}
type Presentation struct {
	Reported   []Datum     `json:"reported"`
	Computed   []Execution `json:"computed"`
	Conflicts  []Conflict  `json:"conflicts"`
	Hypotheses []string    `json:"hypotheses"`
	Gaps       []string    `json:"gaps"`
	Actions    []string    `json:"actions"`
}

// Ledger 只用于同次执行的结果真实性核验；完整证据随报告持久化到已有历史及 Trace。
// 权限和输入始终由调用上下文决定，不从 Ledger 恢复权限。
type Ledger struct {
	mu      sync.Mutex
	reports map[string][]byte
}
type ledgerKey struct{}

func WithLedger(ctx context.Context) context.Context {
	return context.WithValue(ctx, ledgerKey{}, &Ledger{reports: map[string][]byte{}})
}

func EnsureLedger(ctx context.Context) context.Context {
	if _, ok := ctx.Value(ledgerKey{}).(*Ledger); ok {
		return ctx
	}
	return WithLedger(ctx)
}
func Verify(ctx context.Context, value any) error {
	l, ok := ctx.Value(ledgerKey{}).(*Ledger)
	if !ok {
		return fmt.Errorf("agent.response_evidence_ledger_missing")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(raw))
	l.mu.Lock()
	defer l.mu.Unlock()
	if !bytes.Equal(l.reports[digest], raw) {
		return fmt.Errorf("agent.response_evidence_untrusted")
	}
	return nil
}
func Decode(raw []byte, target any) error {
	if len(raw) > 1024*1024 {
		return fmt.Errorf("agent.response_contract_limit_exceeded")
	}
	var shape any
	if err := json.Unmarshal(raw, &shape); err != nil {
		return err
	}
	if err := requireFields(shape, reflect.TypeOf(target)); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return fmt.Errorf("agent.response_contract_invalid: %w", err)
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return fmt.Errorf("agent.response_contract_invalid: trailing_json")
	}
	return nil
}

// Required fields cannot silently turn into Go zero values (notably precision and percent).
func requireFields(value any, typ reflect.Type) error {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == reflect.TypeOf(time.Time{}) {
		return nil
	}
	switch typ.Kind() {
	case reflect.Struct:
		m, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("agent.response_contract_invalid: object_required")
		}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			key := strings.Split(f.Tag.Get("json"), ",")[0]
			if key == "" || key == "-" {
				continue
			}
			v, ok := m[key]
			if !ok && strings.Contains(f.Tag.Get("json"), ",omitempty") {
				continue
			}
			if !ok || v == nil {
				return fmt.Errorf("agent.response_contract_invalid: required_field %s", key)
			}
			if err := requireFields(v, f.Type); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if arr, ok := value.([]any); ok {
			for _, v := range arr {
				if err := requireFields(v, typ.Elem()); err != nil {
					return err
				}
			}
		}
	case reflect.Map:
		if m, ok := value.(map[string]any); ok {
			for _, v := range m {
				if err := requireFields(v, typ.Elem()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func pointer(root map[string]any, ref string) (any, error) {
	if !strings.HasPrefix(ref, "/") {
		return nil, fmt.Errorf("evidence.source_pointer_invalid")
	}
	var current any = root
	for _, key := range strings.Split(ref[1:], "/") {
		key = strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("evidence.source_missing")
		}
		current, ok = object[key]
		if !ok {
			return nil, fmt.Errorf("evidence.source_missing")
		}
	}
	return current, nil
}

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var literalPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+|[0-9]{1,3}(?:,[0-9]{3})+)(?:\.[0-9]+)?$`)

// Compile 是显式 response draft 执行流程，不从自由文本猜测结构化结果。
// 每个数值必须带声明的词面值、单位和原文定位；所有展示结果由工具计算生成。
func Compile(ctx context.Context, draft Draft, payload map[string]any, allowedSources []string, tenantUUID, revisionUUID, traceID string) (map[string]any, error) {
	return compileDraft(ctx, draft, payload, allowedSources, tenantUUID, revisionUUID, traceID, false)
}

func compileDraft(ctx context.Context, draft Draft, payload map[string]any, allowedSources []string, tenantUUID, revisionUUID, traceID string, preparing bool) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if draft.Schema != DraftSchema || draft.Kind == "" || draft.Data == nil || draft.Calculations == nil || draft.Hypotheses == nil || draft.Gaps == nil || draft.Actions == nil || len(draft.Data) > 64 || len(draft.Calculations) > 32 {
		return nil, fmt.Errorf("agent.response_draft_invalid")
	}
	if _, err := uuid.Parse(tenantUUID); err != nil {
		return nil, fmt.Errorf("evidence.tenant_uuid_required")
	}
	if _, err := uuid.Parse(revisionUUID); err != nil {
		return nil, fmt.Errorf("evidence.revision_uuid_required")
	}
	if traceID == "" {
		return nil, fmt.Errorf("evidence.trace_required")
	}
	if _, ok := ctx.Value(ledgerKey{}).(*Ledger); !ok {
		return nil, fmt.Errorf("agent.response_evidence_ledger_missing")
	}
	allowed := map[string]bool{}
	for _, p := range allowedSources {
		allowed[p] = true
	}
	sources := map[string]any{}
	data := map[string]Datum{}
	reported := []Datum{}
	for _, extracted := range draft.Data {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d := Datum{Key: extracted.Key, Label: extracted.Label, Value: strings.ReplaceAll(extracted.Source.Literal, ",", ""), Unit: extracted.Unit, Scope: extracted.Scope, Kind: extracted.Kind, Source: extracted.Source}
		if !keyPattern.MatchString(d.Key) || d.Label == "" || d.Scope == "" || (d.Kind != "quantity" && d.Kind != "reported") || !allowed[d.Source.Pointer] {
			return nil, fmt.Errorf("evidence.datum_invalid: %s", d.Key)
		}
		if _, ok := data[d.Key]; ok {
			return nil, fmt.Errorf("evidence.duplicate_key: %s", d.Key)
		}
		if !literalPattern.MatchString(d.Source.Literal) {
			return nil, fmt.Errorf("evidence.literal_mismatch: %s", d.Key)
		}
		if _, err := decimal(d.Value); err != nil {
			return nil, err
		}
		raw, err := pointer(payload, d.Source.Pointer)
		if err != nil {
			return nil, err
		}
		text, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("evidence.source_text_required")
		}
		// 必须提交含单位的精确引用，不能把百分号截掉当数量。
		lexeme := d.Source.Literal + d.Unit
		validQuote := d.Source.Quote != "" && containsQuotedNumericToken(text, d.Source.Quote, lexeme)
		if !validQuote && d.Unit != "" {
			// Whitespace between an explicitly supplied numeric token and unit is lexical formatting.
			pattern := regexp.MustCompile(regexp.QuoteMeta(d.Source.Literal) + `[ \t]+` + regexp.QuoteMeta(d.Unit))
			for _, literalWithUnit := range pattern.FindAllString(d.Source.Quote, -1) {
				if containsQuotedNumericToken(text, d.Source.Quote, literalWithUnit) {
					validQuote = true
					break
				}
			}
		}
		if !validQuote {
			return nil, fmt.Errorf("evidence.source_quote_mismatch: %s", d.Key)
		}
		if d.Kind == "quantity" && (d.Unit == "%" || d.Unit == "％") {
			return nil, fmt.Errorf("evidence.rate_not_quantity: %s", d.Key)
		}
		data[d.Key] = d
		reported = append(reported, d)
		sources[d.Source.Pointer] = raw
	}
	raw, _ := json.Marshal(sources)
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(raw))
	p := Presentation{Reported: reported, Computed: []Execution{}, Conflicts: []Conflict{}, Hypotheses: draft.Hypotheses, Gaps: draft.Gaps, Actions: draft.Actions}
	keys := map[string]bool{}
	for _, c := range draft.Calculations {
		if !keyPattern.MatchString(c.Key) || keys[c.Key] || c.Label == "" || len(c.Bindings) == 0 {
			return nil, fmt.Errorf("evidence.calculation_invalid")
		}
		keys[c.Key] = true
		operands := map[string]Datum{}
		bindings := map[string]string{}
		scope, unit := "", ""
		for variable, ref := range c.Bindings {
			d, ok := data[ref]
			if !ok || d.Kind != "quantity" {
				return nil, fmt.Errorf("evidence.quantity_reference_required: %s", ref)
			}
			if scope != "" && (scope != d.Scope || unit != d.Unit) {
				return nil, fmt.Errorf("evidence.operand_scope_or_unit_mismatch")
			}
			scope, unit = d.Scope, d.Unit
			operands[variable] = d
			bindings[variable] = d.Value
		}
		// 报告计算不允许数值字面量：比例换算使用 percent，业务数据必须引用。
		if err := validateBoundExpression(c.Expression, c.Bindings); err != nil {
			return nil, err
		}
		start := time.Now().UTC()
		result, err := Calculate(ctx, c.Expression, bindings, c.Precision, c.Percent)
		if err != nil {
			return nil, err
		}
		record := Execution{UUID: uuid.NewString(), ToolKey: CalculatorKey, ToolVersion: CalculatorVersion, TenantUUID: tenantUUID, RevisionUUID: revisionUUID, TraceID: traceID, SourceDigest: digest, Request: c, Operands: operands, DisplayValue: result, StartedAt: start, EndedAt: time.Now().UTC()}
		p.Computed = append(p.Computed, record)
		if c.CompareTo != "" {
			d, ok := data[c.CompareTo]
			if !ok || d.Kind != "reported" || d.Scope != scope {
				return nil, fmt.Errorf("evidence.comparison_reference_invalid")
			}
			if (d.Unit == "%" || d.Unit == "％") != c.Percent {
				return nil, fmt.Errorf("evidence.comparison_unit_mismatch")
			}
			claimed, err := Calculate(ctx, "a", map[string]string{"a": d.Value}, c.Precision, false)
			if err != nil {
				return nil, err
			}
			if c.Percent {
				claimed += "%"
			}
			if claimed != result {
				p.Conflicts = append(p.Conflicts, Conflict{c.Key, d.Key, claimed, result})
			}
		}
	}
	return sealReport(ctx, Report{Schema: ReportSchema, Kind: draft.Kind, Presentation: p, SourceDigest: digest, TenantUUID: tenantUUID, RevisionUUID: revisionUUID, TraceID: traceID}, preparing)
}

func sealReport(ctx context.Context, report Report, preparing bool) (map[string]any, error) {
	l, ok := ctx.Value(ledgerKey{}).(*Ledger)
	if !ok {
		return nil, fmt.Errorf("agent.response_evidence_ledger_missing")
	}
	p := report.Presentation
	for _, list := range [][]string{p.Hypotheses, p.Gaps, p.Actions} {
		if len(list) > 6 {
			return nil, fmt.Errorf("evidence.statement_limit_exceeded")
		}
		for _, v := range list {
			if len([]rune(v)) > 300 {
				return nil, fmt.Errorf("evidence.statement_limit_exceeded")
			}
			if strings.TrimSpace(v) == "" {
				return nil, fmt.Errorf("evidence.empty_statement")
			}
			if strings.ContainsAny(v, "0123456789") {
				return nil, fmt.Errorf("evidence.numeric_narrative_forbidden")
			}
		}
	}
	outcome := "completed"
	if len(p.Conflicts) > 0 || len(p.Gaps) > 0 || len(p.Hypotheses) > 0 {
		outcome = "needs_action"
	}
	if !preparing && len(p.Reported) == 0 && len(p.Computed) == 0 && len(p.Gaps) == 0 {
		return nil, fmt.Errorf("evidence.empty_report")
	}
	report.Outcome = outcome
	raw, _ := json.Marshal(report)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	canonical, _ := json.Marshal(out)
	key := fmt.Sprintf("%x", sha256.Sum256(canonical))
	l.mu.Lock()
	l.reports[key] = canonical
	l.mu.Unlock()
	return out, nil
}

// Validate boundaries in the actual input, not just a model-selected substring.
func containsQuotedNumericToken(text, quote, lexeme string) bool {
	if !containsNumericToken(quote, lexeme) {
		return false
	}
	for offset := 0; offset < len(text); {
		i := strings.Index(text[offset:], quote)
		if i < 0 {
			return false
		}
		i += offset
		start, end := i, i+len(quote)
		if start > 0 {
			start--
		}
		if end < len(text) {
			end++
		}
		if strings.HasPrefix(text[i+len(quote):], "％") {
			end = i + len(quote) + len("％")
		}
		if containsNumericToken(text[start:end], lexeme) {
			return true
		}
		offset = i + len(quote)
	}
	return false
}

func containsNumericToken(quote, lexeme string) bool {
	for offset := 0; offset < len(quote); {
		idx := strings.Index(quote[offset:], lexeme)
		if idx < 0 {
			return false
		}
		idx += offset
		end := idx + len(lexeme)
		badBefore := idx > 0 && strings.ContainsRune("0123456789.,+-", rune(quote[idx-1]))
		badAfter := end < len(quote) && strings.ContainsRune("0123456789.,%", rune(quote[end]))
		if !badBefore && !badAfter && !strings.HasPrefix(quote[end:], "％") {
			return true
		}
		offset = end
	}
	return false
}
