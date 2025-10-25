package schemas

import "strings"

// IntentMatcherType 用于 blueprint 中的 matchers
type IntentMatcherType string

const (
	IntentMatcherKeyword IntentMatcherType = "keyword"
	IntentMatcherPattern IntentMatcherType = "pattern"
)

// IntentMatcher 对应 blueprint.metadata.intent.matchers
type IntentMatcher struct {
	Type     IntentMatcherType `json:"type"`
	Any      []string          `json:"any,omitempty"` // 关键词
	Pattern  string            `yaml:"pattern,omitempty"`
	Regex    string            `json:"regex,omitempty"` // 正则表达式
	Priority int               `json:"priority,omitempty"`
}

func (m IntentMatcher) RegexValue() string {
	if strings.TrimSpace(m.Regex) != "" {
		return m.Regex
	}
	return m.Pattern
}

type IntentEmbedding struct {
	Title    string   `yaml:"title,omitempty"`
	Synonyms []string `yaml:"synonyms,omitempty"`
}

// IntentExamples 对应 blueprint.metadata.intent.examples
type IntentExamples struct {
	Positive []string `json:"positive,omitempty"`
	Negative []string `json:"negative,omitempty"`
}

type IntentSpec struct {
	// 绑定信息
	FlowID   string        `json:"flow_id"`            // 必填：对应的 flow
	Name     string        `json:"name,omitempty"`     // 可选：flow name
	Domain   string        `json:"domain,omitempty"`   // 可选：来自 metadata.domain
	Version  string        `json:"version,omitempty"`  // 可选：flow 版本
	Metadata *FlowMetadata `json:"metadata,omitempty"` // 透传 flow 的 metadata（含 IO / requires 等）

	// 识别配置
	Register  bool             `json:"register,omitempty"`  // 是否注册到路由（缺省 true）
	Group     string           `json:"group,omitempty"`     // 业务分组/簇，比如 "demo.baseline"
	Weight    float64          `json:"weight,omitempty"`    // 该意图在聚合时的权重（默认 1.0）
	Matchers  []IntentMatcher  `json:"matchers,omitempty"`  // 规则匹配
	Embedding *IntentEmbedding `json:"embedding,omitempty"` // 向量检索提示
	Examples  IntentExamples   `json:"examples,omitempty"`  // 正负例

	// 预留：可在 loader 填充，策略层不必关心
	// Extra map[string]any `json:"extra,omitempty"`
}

type IntentResult struct {
	Matched  bool    `json:"matched"`
	FlowID   string  `json:"flow_id,omitempty"`
	AgentID  string  `json:"agent_id,omitempty"`
	Score    float64 `json:"score,omitempty"`
	Strategy string  `json:"strategy,omitempty"`
	Reason   string  `json:"reason,omitempty"`
}

type DetectedTask struct {
	FlowID    string                 `json:"flow_id"`
	AgentID   string                 `json:"agent_id"`
	Score     float64                `json:"score"`
	Strategy  string                 `json:"strategy"` // rule/embedding/llm
	Reason    string                 `json:"reason,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`     // 槽位填充(可选)
	DependsOn []string               `json:"depends_on,omitempty"` // 依赖的 task_id
	TaskID    string                 `json:"task_id"`              // 生成的唯一ID
}
