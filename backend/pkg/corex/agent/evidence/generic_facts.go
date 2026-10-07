package evidence

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// GenericFactsSchema is a source-preserving extraction protocol.  It records
// an article's own numeric business statements without granting them formula
// eligibility.  Formula eligibility remains exclusively in CalculationPolicy.
const GenericFactsSchema = "powerx.agent.evidence-facts/v1"

// GenericFactInventorySchema 要求逐项处理所有可命名的原文数值，防止
// 合法但不完整的模型子集被当作完整报告。
const GenericFactInventorySchema = "powerx.agent.evidence-facts/v2"

type GenericFactInventory struct {
	Schema string `json:"schema"`
	Facts  map[string]struct {
		Label string `json:"label"`
		Scope string `json:"scope"`
	} `json:"facts"`
}

func GenericFactInventoryJSONSchema(tokens []NumericToken) map[string]any {
	properties := map[string]any{}
	required := []string{}
	for _, token := range tokens {
		labels := genericFactLabels(token)
		if len(labels) == 0 {
			continue
		}
		properties[token.Key] = map[string]any{"type": "object", "additionalProperties": false,
			"properties": map[string]any{"label": map[string]any{"type": "string", "enum": labels}, "scope": map[string]any{"type": "string", "minLength": 1, "maxLength": 120}},
			"required":   []string{"label", "scope"}}
		required = append(required, token.Key)
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
		"schema": map[string]any{"const": GenericFactInventorySchema},
		"facts":  map[string]any{"type": "object", "additionalProperties": false, "maxProperties": 64, "properties": properties, "required": required}}, "required": []string{"schema", "facts"}}
}

// ValidateGenericFactInventoryTokens 在调用模型前检查现有报告的事实上限，
// 超限不得靠截断输入或漏选候选来伪造完整性。
func ValidateGenericFactInventoryTokens(tokens []NumericToken) error {
	count := 0
	for _, token := range tokens {
		if len(genericFactLabels(token)) > 0 {
			count++
		}
	}
	if count > 64 {
		return fmt.Errorf("evidence.generic_fact_limit_exceeded")
	}
	return nil
}

func ResolveGenericFactInventory(inventory GenericFactInventory, tokens []NumericToken) ([]DraftDatum, error) {
	if err := ValidateGenericFactInventoryTokens(tokens); err != nil {
		return nil, err
	}
	if inventory.Schema != GenericFactInventorySchema || inventory.Facts == nil {
		return nil, fmt.Errorf("evidence.generic_inventory_schema_invalid")
	}
	selection := GenericFactSelection{Schema: GenericFactsSchema, Facts: []GenericFactChoice{}}
	for _, token := range tokens {
		if len(genericFactLabels(token)) == 0 {
			continue
		}
		fact, ok := inventory.Facts[token.Key]
		if !ok {
			return nil, fmt.Errorf("evidence.generic_inventory_incomplete: %s", token.Key)
		}
		selection.Facts = append(selection.Facts, GenericFactChoice{Label: fact.Label, Scope: fact.Scope, TokenRef: token.Key})
	}
	if len(selection.Facts) != len(inventory.Facts) {
		return nil, fmt.Errorf("evidence.generic_inventory_unknown_token")
	}
	return ResolveGenericFacts(selection, tokens)
}

var genericIdentifierContextPattern = regexp.MustCompile(`(?i)\b(?:id|identifier|number)\b`)

type GenericFactSelection struct {
	Schema string              `json:"schema"`
	Facts  []GenericFactChoice `json:"facts"`
}

type GenericFactChoice struct {
	Label    string `json:"label"`
	Scope    string `json:"scope"`
	TokenRef string `json:"token_ref"`
}

func GenericFactsJSONSchema(tokens []NumericToken) map[string]any {
	choices := make([]any, 0, len(tokens))
	for _, token := range tokens {
		labels := genericFactLabels(token)
		if len(labels) == 0 {
			continue
		}
		choices = append(choices, map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"label":     map[string]any{"type": "string", "enum": labels},
				"scope":     map[string]any{"type": "string", "minLength": 1, "maxLength": 120},
				"token_ref": map[string]any{"type": "string", "const": token.Key},
			},
			"required": []string{"label", "scope", "token_ref"},
		})
	}
	items := any(false)
	if len(choices) > 0 {
		items = map[string]any{"oneOf": choices}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"schema": map[string]any{"const": GenericFactsSchema},
			"facts":  map[string]any{"type": "array", "maxItems": 64, "items": items},
		},
		"required": []string{"schema", "facts"},
	}
}

// genericFactLabels builds source-owned label choices for a numeric token. The
// model may classify a token and state its scope, but it must not paraphrase
// the business name: the pair is constrained together by the JSON schema.
func genericFactLabels(token NumericToken) []string {
	quote := token.Source.Quote
	needle := token.Source.Literal + token.Unit
	index := strings.Index(quote, needle)
	if token.LiteralOffset > 0 && token.LiteralOffset < len(quote) && strings.HasPrefix(quote[token.LiteralOffset:], token.Source.Literal) {
		index = token.LiteralOffset
	}
	if index < 0 {
		index = strings.Index(quote, token.Source.Literal)
	}
	if index < 0 {
		return nil
	}
	end := index + len(needle)
	if end > len(quote) {
		end = index + len(token.Source.Literal)
	}
	left := genericFactLabelSegment(quote[:index], true)
	right := genericFactLabelSegment(quote[end:], false)
	labels := make([]string, 0, 2)
	for _, label := range []string{left, right} {
		if label == "" || utf8.RuneCountInString(label) > 80 || genericNonBusinessNumericLabel(label) {
			continue
		}
		duplicate := false
		for _, existing := range labels {
			if existing == label {
				duplicate = true
				break
			}
		}
		if !duplicate {
			labels = append(labels, label)
		}
	}
	return labels
}

func genericFactLabelSegment(text string, beforeNumber bool) string {
	if !beforeNumber {
		text = strings.TrimLeft(text, "的 \t")
		if text == "" || strings.ContainsRune("，,。；;：:\n", []rune(text)[0]) {
			return ""
		}
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return strings.ContainsRune("，,。；;：:\n", r)
	})
	if len(parts) == 0 {
		return ""
	}
	partIndex := len(parts) - 1
	if !beforeNumber {
		partIndex = 0
	}
	segment := strings.TrimSpace(parts[partIndex])
	if !beforeNumber {
		segment = strings.TrimLeft(segment, "的 ")
		if delimiter := strings.IndexAny(segment, "，,。；;：:\n"); delimiter >= 0 {
			segment = segment[:delimiter]
		}
		segment = strings.TrimSpace(segment)
		if sourceNumberPattern.MatchString(segment) {
			return ""
		}
		return segment
	}
	// 范围上界沿用同一段原文名称，不把下界数字带进指标标签。
	if (strings.HasSuffix(segment, "到") && !strings.HasSuffix(segment, "达到")) || strings.HasSuffix(segment, "至") || strings.HasSuffix(segment, " to") {
		if span := sourceNumberPattern.FindStringIndex(segment); span != nil {
			segment = strings.TrimSpace(segment[:span[0]])
		}
	}
	for _, suffix := range []string{"只有", "达到", "为", "是", "约", "近", "共", "在"} {
		segment = strings.TrimSuffix(segment, suffix)
	}
	return strings.TrimSpace(segment)
}

// ResolveGenericFacts derives both the displayed value and its machine key
// from a platform token.  The model can name only a phrase that occurs in the
// quoted source context and never supplies a number, unit, or formula.
func ResolveGenericFacts(selection GenericFactSelection, tokens []NumericToken) ([]DraftDatum, error) {
	if selection.Schema != GenericFactsSchema || selection.Facts == nil || len(selection.Facts) > 64 {
		return nil, fmt.Errorf("evidence.generic_facts_schema_invalid")
	}
	byKey := make(map[string]NumericToken, len(tokens))
	for _, token := range tokens {
		byKey[token.Key] = token
	}
	seen := map[string]bool{}
	out := make([]DraftDatum, 0, len(selection.Facts))
	for i, fact := range selection.Facts {
		if utf8.RuneCountInString(fact.Label) > 80 || utf8.RuneCountInString(fact.Scope) > 120 || strings.TrimSpace(fact.Label) == "" || strings.TrimSpace(fact.Scope) == "" {
			return nil, fmt.Errorf("evidence.generic_fact_invalid")
		}
		token, ok := byKey[fact.TokenRef]
		if !ok {
			return nil, fmt.Errorf("evidence.generic_fact_token_missing")
		}
		if genericNonBusinessNumericLabel(fact.Label) {
			return nil, fmt.Errorf("evidence.generic_fact_identifier_or_date_forbidden")
		}
		if !containsString(genericFactLabels(token), strings.TrimSpace(fact.Label)) {
			return nil, fmt.Errorf("evidence.generic_fact_label_not_in_source")
		}
		identity := token.Source.Pointer + "\x00" + token.Source.Quote + "\x00" + token.Source.Literal + "\x00" + token.Unit + "\x00" + fact.Label
		if seen[identity] {
			// A repeated selection names the exact same source-backed fact, not
			// additional evidence. Keep the first declared scope deterministically
			// and continue; failing the whole report would discard valid upstream
			// work solely because an LLM repeated an already selected token.
			continue
		}
		seen[identity] = true
		out = append(out, DraftDatum{Key: fmt.Sprintf("reported_fact_%d", i+1), Label: strings.TrimSpace(fact.Label), Unit: token.Unit, Scope: strings.TrimSpace(fact.Scope), Kind: "reported", Source: token.Source})
	}
	return out, nil
}

func genericNonBusinessNumericLabel(label string) bool {
	if genericIdentifierContextPattern.MatchString(label) {
		return true
	}
	return strings.Contains(label, "编号") || strings.Contains(label, "订单号") || strings.Contains(label, "日期")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
