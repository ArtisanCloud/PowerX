package schemas

// Capability 定义一个能力（Node-Tool）及其模板信息
type Capability struct {
	ID          string      `yaml:"id" json:"id"`
	Name        string      `yaml:"name,omitempty" json:"name,omitempty"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Template    string      `yaml:"template,omitempty" json:"template,omitempty"` // 引擎渲染用
	Parameters  []Parameter `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	Examples    []Example   `yaml:"examples,omitempty" json:"examples,omitempty"`
}

// Parameter 能力输入参数定义
type Parameter struct {
	Name        string      `yaml:"name" json:"name"`
	Type        string      `yaml:"type" json:"type"` // string, int, object…
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool        `yaml:"required,omitempty" json:"required,omitempty"`
	Default     interface{} `yaml:"default,omitempty" json:"default,omitempty"`
}

// Example 示例（能力调用示例）
type Example struct {
	Name        string                 `yaml:"name,omitempty" json:"name,omitempty"`
	Description string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Input       map[string]interface{} `yaml:"input,omitempty" json:"input,omitempty"`
	Output      interface{}            `yaml:"output,omitempty" json:"output,omitempty"`
}

// JSONSchema 简化版 JSON Schema，用于校验工具输入/输出
type JSONSchema struct {
	Type        string                 `yaml:"type" json:"type"`
	Properties  map[string]*JSONSchema `yaml:"properties,omitempty" json:"properties,omitempty"`
	Required    []string               `yaml:"required,omitempty" json:"required,omitempty"`
	Items       *JSONSchema            `yaml:"items,omitempty" json:"items,omitempty"`
	Description string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Default     interface{}            `yaml:"default,omitempty" json:"default,omitempty"`
	Enum        []interface{}          `yaml:"enum,omitempty" json:"enum,omitempty"`
}
