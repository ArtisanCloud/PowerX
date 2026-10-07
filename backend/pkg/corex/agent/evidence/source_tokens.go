package evidence

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NumericToken 是原始输入的词面证据，不推断业务含义，也不进行单位换算。
type NumericToken struct {
	Key           string `json:"key"`
	Unit          string `json:"unit"`
	Source        Source `json:"source"`
	LiteralOffset int    `json:"literal_offset"`
}

type SelectedValue struct {
	Scope    string `json:"scope"`
	TokenRef string `json:"token_ref"`
}
type SourceSelection struct {
	Schema string                   `json:"schema"`
	Data   map[string]SelectedValue `json:"data"`
}

var sourceNumberPattern = regexp.MustCompile(`[+-]?[0-9]+(?:,[0-9]{3})*(?:\.[0-9]+)?`)

func TokenizeSources(payload map[string]any, sources []string, policy CalculationPolicy) ([]NumericToken, error) {
	units := []string{}
	seen := map[string]bool{}
	for _, f := range policy.InputFields {
		for _, u := range f.UnitTokens {
			if u != "" && !seen[u] {
				units = append(units, u)
				seen[u] = true
			}
		}
	}
	return tokenizeSourcesWithUnits(payload, sources, units, false)
}

// TokenizeGenericSources preserves numeric statements before a Skill decides
// whether any of them are eligible for a declared business calculation.  The
// list is deliberately lexical: it does not contain marketing field names or
// turn an unrecognised number into a metric.
func TokenizeGenericSources(payload map[string]any, sources []string) ([]NumericToken, error) {
	return tokenizeSourcesWithUnits(payload, sources, []string{
		"万元", "亿元", "美元", "人民币", "元", "万", "亿", "%", "％",
		"seconds", "minutes", "hours", "months", "weeks", "years",
		"小时", "分钟", "个月", "季度", "星期", "秒", "天", "周", "年",
		"人", "次", "单", "条", "个",
	}, true)
}

func tokenizeSourcesWithUnits(payload map[string]any, sources []string, units []string, preserveUnknownUnit bool) ([]NumericToken, error) {
	units = append([]string(nil), units...)
	sort.Slice(units, func(i, j int) bool {
		if len(units[i]) == len(units[j]) {
			return units[i] < units[j]
		}
		return len(units[i]) > len(units[j])
	})
	out := []NumericToken{}
	for _, source := range sources {
		raw, err := pointer(payload, source)
		if err != nil {
			return nil, err
		}
		text, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("evidence.source_text_required")
		}
		if len(text) > 128*1024 {
			return nil, fmt.Errorf("evidence.source_limit_exceeded")
		}
		for _, span := range sourceNumberPattern.FindAllStringIndex(text, -1) {
			literal := text[span[0]:span[1]]
			if !literalPattern.MatchString(literal) {
				continue
			}
			after := text[span[1]:]
			trimmed := strings.TrimLeft(after, " \t")
			unit := ""
			for _, u := range units {
				if strings.HasPrefix(trimmed, u) {
					// A count unit is not allowed to consume the prefix of a
					// compound duration such as "6个月". This is a generic
					// numeric-lexing rule, not a business-field special case;
					// authors may declare the complete duration unit when it is
					// actually a supported input unit.
					if isCompoundDurationUnit(u, strings.TrimPrefix(trimmed, u)) {
						continue
					}
					unit = u
					break
				}
			}
			// 未声明的单位不被猜成无单位数量，日期/编号也不提升为计数。
			if unit == "" && len(trimmed) > 0 {
				r, _ := utf8.DecodeRuneInString(trimmed)
				if unicode.IsLetter(r) || r == '%' || r == '％' {
					if !preserveUnknownUnit || !unicode.IsLetter(r) {
						continue
					}
					unit = genericUnknownUnit(trimmed)
				}
			}
			end := span[1]
			if unit != "" {
				end += len(after) - len(trimmed) + len(unit)
			}
			quote := text[span[0]:end]
			if !containsQuotedNumericToken(text, quote, literal+unit) && !strings.Contains(quote, " ") && !strings.Contains(quote, "\t") {
				continue
			}
			// 上下文只用于映射和审计，保持为原文连续片段。
			left, right := []rune(text[:span[0]]), []rune(text[end:])
			if len(left) > 24 {
				left = left[len(left)-24:]
			}
			if len(right) > 24 {
				right = right[:24]
			}
			prefix, suffix := string(left), string(right)
			if at := strings.LastIndexAny(prefix, "，,。；;\n"); at >= 0 {
				_, size := utf8.DecodeRuneInString(prefix[at:])
				prefix = prefix[at+size:]
			}
			if at := strings.IndexAny(suffix, "，,。；;\n"); at >= 0 {
				suffix = suffix[:at]
			}
			out = append(out, NumericToken{Key: fmt.Sprintf("token_%d", len(out)), Unit: unit, LiteralOffset: len(prefix), Source: Source{Pointer: source, Quote: prefix + quote + suffix, Literal: literal}})
			if len(out) > 512 {
				return nil, fmt.Errorf("evidence.source_token_limit_exceeded")
			}
		}
	}
	return out, nil
}

func genericUnknownUnit(text string) string {
	end := 0
	for end < len(text) {
		r, size := utf8.DecodeRuneInString(text[end:])
		if !unicode.IsLetter(r) && r != '/' {
			break
		}
		end += size
		if end >= 24 {
			break
		}
	}
	return text[:end]
}

func isCompoundDurationUnit(unit, suffix string) bool {
	if unit != "个" {
		return false
	}
	for _, duration := range []string{"月", "季度", "星期", "周", "年", "小时", "分钟", "秒"} {
		if strings.HasPrefix(suffix, duration) {
			return true
		}
	}
	return false
}

func SelectionJSONSchema(policy CalculationPolicy, tokens []NumericToken, activeProfiles []string, locale string) map[string]any {
	properties := map[string]any{}
	for _, f := range policy.InputFields {
		if !matchesProfile(f.AppliesTo, activeProfiles) {
			continue
		}
		refs := []string{}
		for _, token := range tokens {
			if !fieldMatchesPolicyTokenEvidence(f, token, policy, activeProfiles, locale) {
				continue
			}
			for _, unit := range f.UnitTokens {
				if unit == token.Unit {
					refs = append(refs, token.Key)
					break
				}
			}
		}
		if len(refs) == 0 {
			continue
		}
		properties[f.Key] = map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"scope": map[string]any{"type": "string"}, "token_ref": map[string]any{"type": "string", "enum": refs}}, "required": []string{"scope", "token_ref"}}
	}
	// data is an object keyed by the declared calculation-policy fields. An
	// array permits a provider to select the same field twice; this shape makes
	// duplicate keys impossible before platform calculation begins.
	required := []string{}
	for _, field := range policy.InputFields {
		if _, ok := properties[field.Key]; ok {
			required = append(required, field.Key)
		}
	}
	data := map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"schema": map[string]any{"const": ExtractionSchema}, "data": data}, "required": []string{"schema", "data"}}
}

func ResolveSelection(selection SourceSelection, tokens []NumericToken, policy CalculationPolicy, activeProfiles []string, locale string) (PolicyExtraction, error) {
	out := PolicyExtraction{Schema: selection.Schema, Data: []InputValue{}}
	if selection.Schema != ExtractionSchema || selection.Data == nil {
		return out, fmt.Errorf("evidence.source_schema_invalid")
	}
	fields := map[string]InputField{}
	for _, field := range policy.InputFields {
		if matchesProfile(field.AppliesTo, activeProfiles) {
			fields[field.Key] = field
		}
	}
	for key := range selection.Data {
		if _, ok := fields[key]; !ok {
			return out, fmt.Errorf("evidence.source_input_key_unknown: %s", key)
		}
	}
	byKey := map[string]NumericToken{}
	for _, token := range tokens {
		byKey[token.Key] = token
	}
	// Preserve the Skill-declared field order in the execution plan.
	for _, field := range policy.InputFields {
		if !matchesProfile(field.AppliesTo, activeProfiles) {
			continue
		}
		selected, selectedOK := selection.Data[field.Key]
		if !selectedOK {
			continue
		}
		token, ok := byKey[selected.TokenRef]
		if !ok {
			return out, fmt.Errorf("evidence.source_token_missing")
		}
		if !fieldMatchesPolicyTokenEvidence(field, token, policy, activeProfiles, locale) {
			return out, fmt.Errorf("evidence.source_context_invalid: %s", field.Key)
		}
		out.Data = append(out.Data, InputValue{Key: field.Key, Scope: selected.Scope, Unit: token.Unit, Source: token.Source})
	}
	return out, nil
}

// fieldMatchesTokenEvidence is a generic policy interpreter. The business words
// come only from the published Skill field declaration, never from Core code.
func fieldMatchesTokenEvidence(field InputField, token NumericToken, locale string) bool {
	return fieldTokenSpecificity(field, token, locale) > 0
}

func fieldTokenSpecificity(field InputField, token NumericToken, locale string) int {
	best := 0
	for _, term := range field.EvidenceTermsI18n[locale] {
		for _, label := range genericFactLabels(token) {
			if normalized := normalizeEvidenceText(term); normalized != "" && strings.Contains(normalizeEvidenceText(label), normalized) && len(normalized) > best {
				best = len(normalized)
			}
		}
	}
	return best
}

// 业务词由 Skill 声明；只匹配数值所在短句，长词优先，避免相邻字段
// 或包含关系（如 GMV 与增量 GMV）把同一个操作数串到不同口径。
func fieldMatchesPolicyTokenEvidence(field InputField, token NumericToken, policy CalculationPolicy, profiles []string, locale string) bool {
	if !slices.Contains(field.UnitTokens, token.Unit) {
		return false
	}
	match := fieldTokenSpecificity(field, token, locale)
	if match == 0 {
		return false
	}
	for _, other := range policy.InputFields {
		if matchesProfile(other.AppliesTo, profiles) && slices.Contains(other.UnitTokens, token.Unit) && fieldTokenSpecificity(other, token, locale) > match {
			return false
		}
	}
	return true
}

func ValidateSelectionCoverage(selection SourceSelection, tokens []NumericToken, policy CalculationPolicy, profiles []string, locale string) error {
	for _, field := range policy.InputFields {
		if !matchesProfile(field.AppliesTo, profiles) {
			continue
		}
		for _, token := range tokens {
			if fieldMatchesPolicyTokenEvidence(field, token, policy, profiles, locale) {
				if _, ok := selection.Data[field.Key]; !ok {
					return fmt.Errorf("evidence.source_selection_incomplete: %s", field.Key)
				}
				break
			}
		}
	}
	return nil
}
