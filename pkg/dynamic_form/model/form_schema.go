package model

// FieldType 定义支持的字段类型
type FieldType string

const (
	FieldTypeString  FieldType = "string"
	FieldTypeNumber  FieldType = "number"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeSelect  FieldType = "select"
	FieldTypeObject  FieldType = "object"
	FieldTypeArray   FieldType = "array"
)

// ValidationRule 单字段验证规则
type ValidationRule struct {
	Required     bool     `json:"required,omitempty"`
	MinLength    *int     `json:"min_length,omitempty"`
	MaxLength    *int     `json:"max_length,omitempty"`
	Min          *float64 `json:"min,omitempty"`
	Max          *float64 `json:"max,omitempty"`
	Pattern      string   `json:"pattern,omitempty"` // regexp
	Custom       string   `json:"custom,omitempty"`  // 自定义表达式（例如 CEL/JSONLogic）
	ErrorMessage string   `json:"error_message,omitempty"`
}

// Condition 用于 visibility / enablement 等
type Condition struct {
	Expr map[string]interface{} `json:"expr,omitempty"`
}

// Field 表单字段定义
type FormField struct {
	Name        string                 `json:"name"`
	Label       string                 `json:"label,omitempty"`
	Type        FieldType              `json:"type"`
	Default     interface{}            `json:"default,omitempty"`
	Options     []string               `json:"options,omitempty"`
	Validations []ValidationRule       `json:"validations,omitempty"`
	Visibility  *Condition             `json:"visibility,omitempty"`
	Enablement  *Condition             `json:"enablement,omitempty"`
	Description string                 `json:"description,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// FormSchema 整体表单 schema
type FormSchema struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	Fields      []FormField            `json:"fields"`
	Variables   map[string]string      `json:"variables,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
