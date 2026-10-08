package evidence

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const ReviewPolicySchema = "powerx.skill-review-policy/v1"

// ReviewPolicy 由 Skill 声明业务说明及适用条件，Core 只匹配原文词面和
// 已核验报告状态。模型不能自行新增基准、提升、因果或补数判断。
type ReviewPolicy struct {
	Schema string       `json:"schema"`
	Rules  []ReviewRule `json:"rules"`
}

type ReviewRule struct {
	Key              string              `json:"key"`
	Target           string              `json:"target"`
	TextI18n         map[string]string   `json:"text_i18n"`
	EvidenceAllI18n  map[string][]string `json:"evidence_all_i18n"`
	WhenAnyFields    []string            `json:"when_any_fields"`
	WhenAnyConflicts []string            `json:"when_any_conflicts"`
	WhenAnyMissing   []string            `json:"when_any_missing"`
	WhenAnyComputed  []string            `json:"when_any_computed,omitempty"`
}

type ReviewChoices struct {
	Hypotheses []string
	Actions    []string
}

func ReadReviewPolicy(raw any, calculation CalculationPolicy) (*ReviewPolicy, error) {
	if raw == nil {
		return nil, nil
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var policy ReviewPolicy
	if err := Decode(body, &policy); err != nil {
		return nil, err
	}
	if policy.Schema != ReviewPolicySchema || policy.Rules == nil || len(policy.Rules) > 32 {
		return nil, fmt.Errorf("skill.review_policy_invalid")
	}
	fields, formulas, keys := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, field := range calculation.InputFields {
		fields[field.Key] = true
	}
	for _, formula := range calculation.Formulas {
		formulas[formula.Key] = true
	}
	for _, rule := range policy.Rules {
		if !keyPattern.MatchString(rule.Key) || keys[rule.Key] || !slices.Contains([]string{"hypotheses", "actions"}, rule.Target) || len(rule.TextI18n) == 0 {
			return nil, fmt.Errorf("skill.review_rule_invalid")
		}
		keys[rule.Key] = true
		for locale, text := range rule.TextI18n {
			terms, ok := rule.EvidenceAllI18n[locale]
			if !ok || strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > 300 || strings.ContainsAny(text, "0123456789") || strings.Contains(text, "〔原文数值〕") {
				return nil, fmt.Errorf("skill.review_rule_locale_or_text_invalid")
			}
			if len(terms)+len(rule.WhenAnyFields)+len(rule.WhenAnyConflicts)+len(rule.WhenAnyMissing)+len(rule.WhenAnyComputed) == 0 {
				return nil, fmt.Errorf("skill.review_rule_condition_required")
			}
			for _, term := range terms {
				if strings.TrimSpace(term) == "" {
					return nil, fmt.Errorf("skill.review_rule_evidence_invalid")
				}
			}
		}
		for _, key := range rule.WhenAnyFields {
			if !fields[key] {
				return nil, fmt.Errorf("skill.review_rule_field_invalid: %s", key)
			}
		}
		for _, key := range append(append(append([]string{}, rule.WhenAnyConflicts...), rule.WhenAnyMissing...), rule.WhenAnyComputed...) {
			if !formulas[key] {
				return nil, fmt.Errorf("skill.review_rule_calculation_invalid: %s", key)
			}
		}
	}
	return &policy, nil
}

func (p *ReviewPolicy) Choices(prepared map[string]any, missing []MissingOperands, payload map[string]any, sources []string, locale string) (ReviewChoices, error) {
	choices := ReviewChoices{Hypotheses: []string{}, Actions: []string{}}
	body, err := json.Marshal(prepared)
	if err != nil {
		return choices, err
	}
	var report Report
	if err := Decode(body, &report); err != nil {
		return choices, err
	}
	fields, conflicts, gaps, computed := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, item := range report.Presentation.Computed {
		computed[item.Request.Key] = true
	}
	for _, item := range report.Presentation.Reported {
		fields[item.Key] = true
	}
	for _, item := range report.Presentation.Conflicts {
		conflicts[item.CalculationKey] = true
	}
	for _, item := range missing {
		gaps[item.CalculationKey] = true
	}
	texts := []string{}
	for _, ref := range sources {
		value, err := pointer(payload, ref)
		if err != nil {
			return choices, err
		}
		text, ok := value.(string)
		if !ok {
			return choices, fmt.Errorf("evidence.source_text_required")
		}
		texts = append(texts, normalizeEvidenceText(text))
	}
	for _, rule := range p.Rules {
		text, ok := rule.TextI18n[locale]
		if !ok {
			return choices, fmt.Errorf("skill.review_rule_locale_required")
		}
		if !matchesReviewKeys(rule.WhenAnyFields, fields) || !matchesReviewKeys(rule.WhenAnyConflicts, conflicts) || !matchesReviewKeys(rule.WhenAnyMissing, gaps) || !matchesReviewKeys(rule.WhenAnyComputed, computed) {
			continue
		}
		terms := rule.EvidenceAllI18n[locale]
		matched := len(terms) == 0
		for _, source := range texts {
			all := true
			for _, term := range terms {
				all = all && strings.Contains(source, normalizeEvidenceText(term))
			}
			matched = matched || all
		}
		if !matched {
			continue
		}
		if rule.Target == "hypotheses" {
			if !slices.Contains(choices.Hypotheses, text) {
				choices.Hypotheses = append(choices.Hypotheses, text)
			}
		} else if !slices.Contains(choices.Actions, text) {
			choices.Actions = append(choices.Actions, text)
		}
	}
	if len(choices.Hypotheses) > 6 || len(choices.Actions) > 6 {
		return choices, fmt.Errorf("evidence.statement_limit_exceeded")
	}
	return choices, nil
}

func matchesReviewKeys(wanted []string, available map[string]bool) bool {
	if len(wanted) == 0 {
		return true
	}
	for _, key := range wanted {
		if available[key] {
			return true
		}
	}
	return false
}

// 说明仍经过独立模型阶段，但只可选择本次满足来源/状态条件的完整声明。
func (c ReviewChoices) ConstrainSchema(schema map[string]any) {
	properties := schema["properties"].(map[string]any)
	for key, values := range map[string][]string{"hypotheses": c.Hypotheses, "actions": c.Actions} {
		field := properties[key].(map[string]any)
		field["minItems"], field["maxItems"], field["uniqueItems"] = len(values), len(values), true
		if len(values) > 0 {
			field["items"].(map[string]any)["enum"] = values
		}
	}
}

func (c ReviewChoices) Validate(notes Notes) error {
	for _, pair := range []struct{ actual, allowed []string }{{notes.Hypotheses, c.Hypotheses}, {notes.Actions, c.Actions}} {
		if len(pair.actual) != len(pair.allowed) {
			return fmt.Errorf("evidence.review_notes_incomplete")
		}
		for i, text := range pair.actual {
			if !slices.Contains(pair.allowed, text) || slices.Contains(pair.actual[:i], text) {
				return fmt.Errorf("evidence.review_notes_unsupported")
			}
		}
	}
	return nil
}
