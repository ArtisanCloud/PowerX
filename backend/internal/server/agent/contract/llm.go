// services/agent/contract/llm.go
package contract

type Message struct {
	Role    string
	Content []ContentPart
}
type ContentPart struct {
	Type  string
	Text  string
	URL   string
	Bytes []byte
	Meta  map[string]any
}

type ToolCall struct {
	Name string
	Args map[string]any
	ID   string
}

type LLMRequest struct {
	Messages    []Message
	Tools       []ToolSpec // ⇦ 直接复用 flowschema.ToolSpec
	Temperature float64
	TopP        float64
	MaxTokens   int
	JSONMode    bool
	Runtime     map[string]any
}
type LLMResponse struct {
	Text            string
	ToolCalls       []ToolCall
	Usage           map[string]int
	Provider, Model string
}
