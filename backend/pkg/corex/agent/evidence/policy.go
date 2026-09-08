package evidence

import (
	"encoding/json"
	"fmt"
)

const PolicySchema = "powerx.skill-calculation-policy/v1"

type InputField struct {
	Key             string            `json:"key"`
	Kind            string            `json:"kind"`
	UnitTokens      []string          `json:"unit_tokens"`
	LabelI18n       map[string]string `json:"label_i18n"`
	DescriptionI18n map[string]string `json:"description_i18n"`
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
}

type CalculationPolicy struct {
	Schema      string       `json:"schema"`
	InputFields []InputField `json:"input_fields"`
	Formulas    []Formula    `json:"formulas"`
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
	if p.Schema != PolicySchema || p.InputFields == nil || p.Formulas == nil || len(p.InputFields) > 64 || len(p.Formulas) > 32 {
		return p, fmt.Errorf("skill.calculation_policy_invalid")
	}
	fields := map[string]InputField{}
	for _, f := range p.InputFields {
		if !keyPattern.MatchString(f.Key) || (f.Kind != "quantity" && f.Kind != "reported") || len(f.LabelI18n) == 0 || len(f.DescriptionI18n) == 0 || len(f.UnitTokens) == 0 {
			return p, fmt.Errorf("skill.calculation_input_invalid")
		}
		if _, exists := fields[f.Key]; exists {
			return p, fmt.Errorf("skill.calculation_input_duplicate")
		}
		for locale, label := range f.LabelI18n {
			if label == "" || f.DescriptionI18n[locale] == "" {
				return p, fmt.Errorf("skill.calculation_locale_required")
			}
		}
		fields[f.Key] = f
	}
	keys := map[string]bool{}
	for _, f := range p.Formulas {
		if !keyPattern.MatchString(f.Key) || keys[f.Key] || len(f.LabelI18n) == 0 || f.Precision < 0 || f.Precision > 12 || len(f.WhenAnyPresent) == 0 {
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

func (p CalculationPolicy) InputDescriptions(locale string) ([]map[string]string, error) {
	items := []map[string]string{}
	for _, f := range p.InputFields {
		if f.LabelI18n[locale] == "" || f.DescriptionI18n[locale] == "" {
			return nil, fmt.Errorf("skill.calculation_locale_required")
		}
		items = append(items, map[string]string{"key": f.Key, "description": f.DescriptionI18n[locale], "label": f.LabelI18n[locale]})
	}
	return items, nil
}

// BuildPlan 只解释 Skill 声明，不选择业务公式，不推断缺失数量，不执行隐式单位换算。
func (p CalculationPolicy) BuildPlan(extracted PolicyExtraction, locale string) (Plan, []MissingOperands, error) {
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
		if !ok || values[v.Key] {
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
		values[v.Key] = true
		plan.Data = append(plan.Data, DraftDatum{Key: v.Key, Label: f.LabelI18n[locale], Kind: f.Kind, Unit: v.Unit, Scope: v.Scope, Source: v.Source})
	}
	for _, f := range p.Formulas {
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
