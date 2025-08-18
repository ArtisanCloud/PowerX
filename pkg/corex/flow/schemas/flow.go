package schemas

type Flow struct {
	FlowPath    string
	FlowID      string `yaml:"flow_id" json:"flow_id"`
	Name        string `yaml:"name,omitempty" json:"name,omitempty"`
	Version     string `yaml:"version,omitempty" json:"version,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	Metadata *FlowMetadata `yaml:"metadata" json:"metadata"` // 含 io / requires / intent
	Slots    []*SlotSpec   `yaml:"slots,omitempty" json:"slots,omitempty"`

	Nodes []*Node `yaml:"nodes" json:"nodes"`                     // 节点列表
	Edges []*Edge `yaml:"edges,omitempty" json:"edges,omitempty"` // 可选：节点连边

	Ref *RefSpec `yaml:"ref,omitempty"`
}

type Edge struct {
	From string `yaml:"from" json:"from"`
	To   string `yaml:"to" json:"to"`
	Type string `yaml:"type,omitempty" json:"type,omitempty"` // 例如 "SEQ"
}

// ============ Flow Metadata ============

type FlowMetadata struct {
	Domain      string            `yaml:"domain,omitempty" json:"domain,omitempty"`
	Version     string            `yaml:"version,omitempty" json:"version,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	IO          *IODecl           `yaml:"io,omitempty"`                                 // inputs / outputs
	Requires    []string          `yaml:"requires,omitempty"`                           // 例如: ["core.demo.baseline.task_1"]
	Priority    int               `yaml:"priority,omitempty" json:"priority,omitempty"` // ⇦ 新增
	Tags        []string          `yaml:"tags,omitempty" json:"tags,omitempty"`
	Owner       string            `yaml:"owner,omitempty" json:"owner,omitempty"`
	IsPublic    bool              `yaml:"is_public,omitempty" json:"is_public,omitempty"`
	ExtraInfo   map[string]string `yaml:"extra_info,omitempty" json:"extra_info,omitempty"`
	Intent      *IntentSpec       `yaml:"intent,omitempty"` // 意图配置（可省略）

}

// ============ IO ============

type IODecl struct {
	Inputs  []ParamSpec `yaml:"inputs,omitempty"`
	Outputs []ParamSpec `yaml:"outputs,omitempty"`
}

type ParamSpec struct {
	Name        string `yaml:"name" json:"name"`
	Type        string `yaml:"type,omitempty" json:"type,omitempty"`
	Required    bool   `yaml:"required,omitempty" json:"required,omitempty"`
	Default     any    `yaml:"default,omitempty" json:"default,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// ============ Ref ============
type RefSpec struct {
	Enable         bool   `yaml:"enable"`
	Target         string `yaml:"target"`
	Variant        string `yaml:"variant,omitempty"`
	RegisterIntent bool   `yaml:"register_intent,omitempty"`
	Overrides      struct {
		Metadata *FlowMetadata `yaml:"metadata,omitempty"`
		// 如果还要覆盖 nodes/params 等，继续加字段
	} `yaml:"overrides,omitempty"`
}

// ============ Slot ============

// SlotSpec 描述一个输入槽位（参数）的获取与校验规则
type SlotSpec struct {
	Name     string       `yaml:"name" json:"name"`                     // 槽位名，如 "upstream_id"
	Type     string       `yaml:"type,omitempty" json:"type,omitempty"` // 数据类型，如 "string"
	Required bool         `yaml:"required,omitempty" json:"required,omitempty"`
	From     []SlotSource `yaml:"from,omitempty" json:"from,omitempty"` // 抽取策略按顺序尝试
}

// SlotSource 定义单个抽取策略
// 目前最小实现包含：rule（正则/规则）、llm（大模型抽取）
type SlotSource struct {
	Type    string `yaml:"type" json:"type"`                           // "rule" | "llm"
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"` // type=rule 时使用
	Hint    string `yaml:"hint,omitempty" json:"hint,omitempty"`       // type=llm 时使用
}
