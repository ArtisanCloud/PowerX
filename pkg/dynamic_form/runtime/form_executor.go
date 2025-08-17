package runtime

import (
	"github.com/ArtisanCloud/PowerX/pkg/dynamic_form/model"
	"regexp"
)

// Result 表单执行结果（清洗 + 错误）
type Result struct {
	ValidatedInputs map[string]interface{} `json:"validated_inputs"`
	FieldErrors     map[string][]string    `json:"field_errors"`
	VisibleFields   []string               `json:"visible_fields"`
	EnabledFields   []string               `json:"enabled_fields"`
	Variables       map[string]interface{} `json:"variables,omitempty"`
}

type FormExecutor struct{}

func NewFormExecutor() *FormExecutor {
	return &FormExecutor{}
}

func (e *FormExecutor) ValidateAndFill(schema *model.FormSchema, input map[string]interface{}) (*Result, error) {
	res := &Result{
		ValidatedInputs: make(map[string]interface{}),
		FieldErrors:     map[string][]string{},
		VisibleFields:   []string{},
		EnabledFields:   []string{},
		Variables:       map[string]interface{}{},
	}

	for _, f := range schema.Fields {
		// 基本可见/可用判断（这里默认都可见可用，扩展时插入条件引擎）
		res.VisibleFields = append(res.VisibleFields, f.Name)
		res.EnabledFields = append(res.EnabledFields, f.Name)

		raw, exists := input[f.Name]
		if !exists {
			if f.Default != nil {
				raw = f.Default
			}
		}

		errors := []string{}
		// Required 检查
		if !exists && f.Default == nil {
			for _, vr := range f.Validations {
				if vr.Required {
					errors = append(errors, "required")
				}
			}
		}
		// Pattern
		for _, vr := range f.Validations {
			if vr.Pattern != "" {
				strVal, ok := raw.(string)
				if ok {
					matched, err := regexp.MatchString(vr.Pattern, strVal)
					if err != nil || !matched {
						errors = append(errors, "pattern_mismatch")
					}
				}
			}
		}

		if len(errors) > 0 {
			res.FieldErrors[f.Name] = errors
		} else if raw != nil {
			res.ValidatedInputs[f.Name] = raw
		}
	}

	// 变量派生可以在这里插入（如根据输入算出某些 derived 值）
	return res, nil
}
