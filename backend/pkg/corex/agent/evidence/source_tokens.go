package evidence

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NumericToken 是原始输入的词面证据，不推断业务含义，也不进行单位换算。
type NumericToken struct {
	Key    string `json:"key"`
	Unit   string `json:"unit"`
	Source Source `json:"source"`
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
					unit = u
					break
				}
			}
			// 未声明的单位不被猜成无单位数量，日期/编号也不提升为计数。
			if unit == "" && len(trimmed) > 0 {
				r, _ := utf8.DecodeRuneInString(trimmed)
				if unicode.IsLetter(r) || r == '%' || r == '％' {
					continue
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
			out = append(out, NumericToken{Key: fmt.Sprintf("token_%d", len(out)), Unit: unit, Source: Source{Pointer: source, Quote: string(left) + quote + string(right), Literal: literal}})
			if len(out) > 512 {
				return nil, fmt.Errorf("evidence.source_token_limit_exceeded")
			}
		}
	}
	return out, nil
}

func SelectionJSONSchema(policy CalculationPolicy, tokens []NumericToken) map[string]any {
	properties := map[string]any{}
	for _, f := range policy.InputFields {
		refs := []string{}
		for _, token := range tokens {
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
	data := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"schema": map[string]any{"const": ExtractionSchema}, "data": data}, "required": []string{"schema", "data"}}
}

func ResolveSelection(selection SourceSelection, tokens []NumericToken, policy CalculationPolicy) (PolicyExtraction, error) {
	out := PolicyExtraction{Schema: selection.Schema, Data: []InputValue{}}
	if selection.Schema != ExtractionSchema || selection.Data == nil {
		return out, fmt.Errorf("evidence.source_schema_invalid")
	}
	fields := map[string]bool{}
	for _, field := range policy.InputFields {
		fields[field.Key] = true
	}
	for key := range selection.Data {
		if !fields[key] {
			return out, fmt.Errorf("evidence.source_input_key_unknown: %s", key)
		}
	}
	byKey := map[string]NumericToken{}
	for _, token := range tokens {
		byKey[token.Key] = token
	}
	// Preserve the Skill-declared field order in the execution plan.
	for _, field := range policy.InputFields {
		selected, selectedOK := selection.Data[field.Key]
		if !selectedOK {
			continue
		}
		token, ok := byKey[selected.TokenRef]
		if !ok {
			return out, fmt.Errorf("evidence.source_token_missing")
		}
		out.Data = append(out.Data, InputValue{Key: field.Key, Scope: selected.Scope, Unit: token.Unit, Source: token.Source})
	}
	return out, nil
}
