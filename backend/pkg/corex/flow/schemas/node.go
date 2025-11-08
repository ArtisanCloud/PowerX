package schemas

// pkg/corex/flow/schemas/node.go

type NodeKind string

const (
	KindStart     NodeKind = "start"     // 开始（可选）
	KindEnd       NodeKind = "end"       // 结束（可选）
	KindLLM       NodeKind = "llm"       // 大模型
	KindPlugin    NodeKind = "plugin"    // 外部插件/工具
	KindWorkflow  NodeKind = "workflow"  // 触发子工作流
	KindBizLogic  NodeKind = "biz_logic" // 业务逻辑（代码/脚本）
	KindCode      NodeKind = "code"      // 代码节点（片段执行）
	KindSelector  NodeKind = "selector"  // 条件/选择器
	KindIntent    NodeKind = "intent_recognizer"
	KindLoop      NodeKind = "loop"
	KindBatch     NodeKind = "batch"
	KindAggVars   NodeKind = "var_aggregate"
	KindIOInput   NodeKind = "input"
	KindIOOutput  NodeKind = "output"
	KindDB        NodeKind = "db" // 统一 DB 类
	KindHTTP      NodeKind = "http"
	KindQA        NodeKind = "qa"
	KindTextProc  NodeKind = "text_proc"
	KindJSONSerDe NodeKind = "json_serde"
	KindTrigger   NodeKind = "trigger"
	KindSession   NodeKind = "session"
	KindMessage   NodeKind = "message"
	KindKnowledge NodeKind = "knowledge"
	KindMemory    NodeKind = "memory"
	KindImage     NodeKind = "image"
	KindVideo     NodeKind = "video"
	KindComponent NodeKind = "component"
)

// Node 与 YAML 的 nodes[*] 一一对应
type Node struct {
	ID     string                 `yaml:"id" json:"id"`
	Use    string                 `yaml:"use" json:"use"`
	Kind   NodeKind               `yaml:"kind,omitempty" json:"kind,omitempty"`
	Params map[string]interface{} `yaml:"params,omitempty" json:"params,omitempty"`
	IO     *NodeIO                `yaml:"io,omitempty" json:"io,omitempty"`
}

// NodeIO：Go 里用 InMap/OutMap 更清晰；YAML 仍是 in / out_map
type NodeIO struct {
	InMap  map[string]string `yaml:"in_map" json:"in_map"`
	OutMap map[string]string `yaml:"out_map" json:"out_map"`
}

func (s *Node) IOInMap() map[string]string {
	if s == nil || s.IO == nil || len(s.IO.InMap) == 0 {
		return nil
	}
	out := make(map[string]string, len(s.IO.InMap))
	for k, v := range s.IO.InMap {
		out[k] = v
	}
	return out
}
func (s *Node) IOOutMap() map[string]string {
	if s == nil || s.IO == nil || len(s.IO.OutMap) == 0 {
		return nil
	}
	out := make(map[string]string, len(s.IO.OutMap))
	for k, v := range s.IO.OutMap {
		out[k] = v
	}
	return out
}
