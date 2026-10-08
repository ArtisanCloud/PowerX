package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const PolicySchema = "powerx.skill-calculation-policy/v2"

var ErrActivityProfileUnmatched = errors.New("evidence.activity_profile_unmatched")

// ActivityProfile is a Skill-owned business applicability declaration. Core only
// matches its terms against the declared evidence sources; it never recognises a
// particular industry, agent, or team.
type ActivityProfile struct {
	Key             string              `json:"key"`
	LabelI18n       map[string]string   `json:"label_i18n"`
	EvidenceAnyI18n map[string][]string `json:"evidence_any_i18n"`
}

type InputField struct {
	Key               string              `json:"key"`
	Kind              string              `json:"kind"`
	UnitTokens        []string            `json:"unit_tokens"`
	LabelI18n         map[string]string   `json:"label_i18n"`
	DescriptionI18n   map[string]string   `json:"description_i18n"`
	EvidenceTermsI18n map[string][]string `json:"evidence_terms_i18n"`
	AppliesTo         []string            `json:"applies_to"`
	ScopeI18n         map[string]string   `json:"scope_i18n,omitempty"`
	TokenRole         string              `json:"token_role,omitempty"`
}

type Formula struct {
	Key            string            `json:"key"`
	LabelI18n      map[string]string `json:"label_i18n"`
	Expression     string            `json:"expression"`
	Bindings       map[string]string `json:"bindings"`
	Precision      int               `json:"precision"`
	Percent        bool              `json:"percent"`
	CompareTo      string            `json:"compare_to"`
	WhenAnyPresent []string          `json:"when_any_present"`
	AppliesTo      []string          `json:"applies_to"`
}

type CalculationPolicy struct {
	Schema           string            `json:"schema"`
	ActivityProfiles []ActivityProfile `json:"activity_profiles"`
	InputFields      []InputField      `json:"input_fields"`
	Formulas         []Formula         `json:"formulas"`
}

type InputValue struct {
	Key    string `json:"key"`
	Unit   string `json:"unit"`
	Scope  string `json:"scope"`
	Source Source `json:"source"`
}

type PolicyExtraction struct {
	Schema string       `json:"schema"`
	Data   []InputValue `json:"data"`
}

type MissingOperands struct {
	CalculationKey string   `json:"calculation_key"`
	Label          string   `json:"label"`
	InputLabels    []string `json:"input_labels"`
}

func ReadCalculationPolicy(raw any) (CalculationPolicy, error) {
	var p CalculationPolicy
	if raw == nil {
		return p, fmt.Errorf("skill.calculation_policy_required")
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return p, err
	}
	if err := Decode(b, &p); err != nil {
		return p, err
	}
	if p.Schema != PolicySchema || p.ActivityProfiles == nil || p.InputFields == nil || p.Formulas == nil || len(p.ActivityProfiles) == 0 || len(p.ActivityProfiles) > 16 || len(p.InputFields) > 64 || len(p.Formulas) > 32 {
		return p, fmt.Errorf("skill.calculation_policy_invalid")
	}
	profiles := map[string]bool{}
	for _, profile := range p.ActivityProfiles {
		if !keyPattern.MatchString(profile.Key) || profiles[profile.Key] || len(profile.LabelI18n) == 0 || len(profile.EvidenceAnyI18n) == 0 {
			return p, fmt.Errorf("skill.calculation_profile_invalid")
		}
		profiles[profile.Key] = true
		for locale, label := range profile.LabelI18n {
			if label == "" || len(profile.EvidenceAnyI18n[locale]) == 0 {
				return p, fmt.Errorf("skill.calculation_locale_required")
			}
		}
	}
	fields := map[string]InputField{}
	for _, f := range p.InputFields {
		if !keyPattern.MatchString(f.Key) || (f.Kind != "quantity" && f.Kind != "reported") || len(f.LabelI18n) == 0 || len(f.DescriptionI18n) == 0 || len(f.EvidenceTermsI18n) == 0 || len(f.AppliesTo) == 0 || len(f.UnitTokens) == 0 {
			return p, fmt.Errorf("skill.calculation_input_invalid")
		}
		if _, exists := fields[f.Key]; exists {
			return p, fmt.Errorf("skill.calculation_input_duplicate")
		}
		for locale, label := range f.LabelI18n {
			if label == "" || f.DescriptionI18n[locale] == "" || len(f.EvidenceTermsI18n[locale]) == 0 {
				return p, fmt.Errorf("skill.calculation_locale_required")
			}
			if f.ScopeI18n != nil && (strings.TrimSpace(f.ScopeI18n[locale]) == "" || utf8.RuneCountInString(f.ScopeI18n[locale]) > 120) {
				return p, fmt.Errorf("skill.calculation_scope_locale_required")
			}
		}
		if f.TokenRole != "" && f.TokenRole != "range_start" && f.TokenRole != "range_end" {
			return p, fmt.Errorf("skill.calculation_token_role_invalid")
		}
		for _, profile := range f.AppliesTo {
			if !profiles[profile] {
				return p, fmt.Errorf("skill.calculation_profile_reference_invalid")
			}
		}
		fields[f.Key] = f
	}
	keys := map[string]bool{}
	for _, f := range p.Formulas {
		if !keyPattern.MatchString(f.Key) || keys[f.Key] || len(f.LabelI18n) == 0 || f.Precision < 0 || f.Precision > 12 || len(f.WhenAnyPresent) == 0 || len(f.AppliesTo) == 0 {
			return p, fmt.Errorf("skill.calculation_formula_invalid")
		}
		keys[f.Key] = true
		if err := validateBoundExpression(f.Expression, f.Bindings); err != nil {
			return p, err
		}
		for _, key := range f.Bindings {
			if fields[key].Kind != "quantity" {
				return p, fmt.Errorf("skill.calculation_quantity_required: %s", key)
			}
		}
		if f.CompareTo != "" && fields[f.CompareTo].Kind != "reported" {
			return p, fmt.Errorf("skill.calculation_comparison_invalid")
		}
		for _, key := range f.WhenAnyPresent {
			if _, ok := fields[key]; !ok {
				return p, fmt.Errorf("skill.calculation_trigger_invalid")
			}
		}
		for _, profile := range f.AppliesTo {
			if !profiles[profile] {
				return p, fmt.Errorf("skill.calculation_profile_reference_invalid")
			}
		}
		for locale, label := range f.LabelI18n {
			if label == "" {
				return p, fmt.Errorf("skill.calculation_locale_required")
			}
			for _, key := range f.Bindings {
				if fields[key].LabelI18n[locale] == "" {
					return p, fmt.Errorf("skill.calculation_locale_required")
				}
			}
		}
	}
	return p, nil
}

func (p CalculationPolicy) InputDescriptions(locale string, activeProfiles []string) ([]map[string]string, error) {
	items := []map[string]string{}
	for _, f := range p.InputFields {
		if !matchesProfile(f.AppliesTo, activeProfiles) {
			continue
		}
		if f.LabelI18n[locale] == "" || f.DescriptionI18n[locale] == "" {
			return nil, fmt.Errorf("skill.calculation_locale_required")
		}
		items = append(items, map[string]string{"key": f.Key, "description": f.DescriptionI18n[locale], "label": f.LabelI18n[locale]})
	}
	return items, nil
}

// DetectProfiles applies only Skill-declared textual indicators. It is deliberately
// deterministic so a model cannot label a GMV review as a lead-generation review.
func (p CalculationPolicy) DetectProfiles(payload map[string]any, sources []string, locale string) ([]string, error) {
	if locale == "" {
		return nil, fmt.Errorf("skill.calculation_locale_required")
	}
	texts := make([]string, 0, len(sources))
	for _, source := range sources {
		raw, err := pointer(payload, source)
		if err != nil {
			return nil, err
		}
		text, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("evidence.source_text_required")
		}
		texts = append(texts, normalizeEvidenceText(text))
	}
	active := []string{}
	for _, profile := range p.ActivityProfiles {
		terms := profile.EvidenceAnyI18n[locale]
		if len(terms) == 0 {
			return nil, fmt.Errorf("skill.calculation_locale_required")
		}
		for _, term := range terms {
			needle := normalizeEvidenceText(term)
			if needle == "" {
				continue
			}
			for _, text := range texts {
				if strings.Contains(text, needle) {
					active = append(active, profile.Key)
					goto nextProfile
				}
			}
		}
	nextProfile:
	}
	if len(active) == 0 {
		return nil, ErrActivityProfileUnmatched
	}
	return active, nil
}

func matchesProfile(declared, active []string) bool {
	for _, want := range declared {
		for _, got := range active {
			if want == got {
				return true
			}
		}
	}
	return false
}

func normalizeEvidenceText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), ""))
}

// BuildPlan 只解释 Skill 声明，不选择业务公式，不推断缺失数量，不执行隐式单位换算。
func (p CalculationPolicy) BuildPlan(extracted PolicyExtraction, activeProfiles []string, locale string) (Plan, []MissingOperands, error) {
	plan := Plan{Schema: PlanSchema, Kind: "analysis", Data: []DraftDatum{}, Calculations: []Calculation{}}
	missing := []MissingOperands{}
	if extracted.Schema != ExtractionSchema || extracted.Data == nil {
		return plan, missing, fmt.Errorf("evidence.source_schema_invalid")
	}
	fields := map[string]InputField{}
	for _, f := range p.InputFields {
		fields[f.Key] = f
	}
	values := map[string]bool{}
	for _, v := range extracted.Data {
		f, ok := fields[v.Key]
		if !ok || !matchesProfile(f.AppliesTo, activeProfiles) || values[v.Key] {
			return plan, missing, fmt.Errorf("evidence.input_key_invalid")
		}
		if f.LabelI18n[locale] == "" {
			return plan, missing, fmt.Errorf("skill.calculation_locale_required")
		}
		allowedUnit := false
		for _, unit := range f.UnitTokens {
			allowedUnit = allowedUnit || v.Unit == unit
		}
		if !allowedUnit {
			return plan, missing, fmt.Errorf("evidence.input_unit_invalid: %s", v.Key)
		}
		if scope := f.ScopeI18n[locale]; scope != "" && scope != v.Scope {
			return plan, missing, fmt.Errorf("evidence.input_scope_invalid: %s", v.Key)
		}
		values[v.Key] = true
		plan.Data = append(plan.Data, DraftDatum{Key: v.Key, Label: f.LabelI18n[locale], Kind: f.Kind, Unit: v.Unit, Scope: v.Scope, Source: v.Source})
	}
	for _, f := range p.Formulas {
		if !matchesProfile(f.AppliesTo, activeProfiles) {
			continue
		}
		active := false
		for _, key := range f.WhenAnyPresent {
			active = active || values[key]
		}
		if !active {
			continue
		}
		if f.LabelI18n[locale] == "" {
			return plan, missing, fmt.Errorf("skill.calculation_locale_required")
		}
		gap := MissingOperands{CalculationKey: f.Key, Label: f.LabelI18n[locale], InputLabels: []string{}}
		// input_fields 顺序稳定，避免 map 遍历导致缺口输出抖动。
		for _, field := range p.InputFields {
			for _, key := range f.Bindings {
				if key == field.Key && !values[key] {
					gap.InputLabels = append(gap.InputLabels, field.LabelI18n[locale])
					break
				}
			}
		}
		if len(gap.InputLabels) > 0 {
			missing = append(missing, gap)
			continue
		}
		compare := ""
		if values[f.CompareTo] {
			compare = f.CompareTo
		}
		plan.Calculations = append(plan.Calculations, Calculation{Key: f.Key, Label: f.LabelI18n[locale], Expression: f.Expression, Bindings: f.Bindings, Precision: f.Precision, Percent: f.Percent, CompareTo: compare})
	}
	return plan, missing, nil
}
