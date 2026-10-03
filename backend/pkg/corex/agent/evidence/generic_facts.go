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
	for _, suffix := range []string{"只有", "达到", "为", "是", "约", "近", "共"} {
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
		identity := token.Source.Pointer + "\x00" + token.Source.Literal + "\x00" + token.Unit + "\x00" + fact.Label
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
