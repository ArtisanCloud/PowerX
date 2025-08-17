package schemas

// HandlerFunc 抽象的工具执行函数签名
type HandlerFunc func(ctx Context) (Result, error)

// Context / Result 通用运行时上下文与结果
type (
	Context = map[string]interface{}
	Result  = map[string]interface{}
)

// ToolSpec 入口工具（Task-Tool）规范
type ToolSpec struct {
	ID           string                 `yaml:"id" json:"id"`
	Name         string                 `yaml:"name,omitempty" json:"name,omitempty"`
	Description  string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Version      string                 `yaml:"version,omitempty" json:"version,omitempty"`
	HandlerType  string                 `yaml:"handler_type,omitempty" json:"handler_type,omitempty"`
	InputSchema  *JSONSchema            `yaml:"input_schema,omitempty" json:"input_schema,omitempty"`
	OutputSchema *JSONSchema            `yaml:"output_schema,omitempty" json:"output_schema,omitempty"`
	Examples     []ToolExample          `yaml:"examples,omitempty" json:"examples,omitempty"`
	AllowedRoles []string               `yaml:"roles,omitempty" json:"roles,omitempty"`
	Metadata     map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// ToolExample 入口工具示例
type ToolExample struct {
	Input  map[string]interface{} `yaml:"input,omitempty" json:"input,omitempty"`
	Output map[string]interface{} `yaml:"output,omitempty" json:"output,omitempty"`
}
