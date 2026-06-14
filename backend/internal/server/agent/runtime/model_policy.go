package runtime

import (
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type ModelPolicyNode string

const (
	ModelPolicyNodeRuntimeIntent  ModelPolicyNode = "runtime_intent"
	ModelPolicyNodeIntent         ModelPolicyNode = "intent_classifier"
	ModelPolicyNodePlanner        ModelPolicyNode = "planner"
	ModelPolicyNodeParamExtractor ModelPolicyNode = "skill_param_extractor"
	ModelPolicyNodeFinalResponse  ModelPolicyNode = "final_response"
	ModelPolicyNodeReviewer       ModelPolicyNode = "reviewer"
)

type NodeModelSelection struct {
	Node     ModelPolicyNode `json:"node"`
	Mode     string          `json:"mode"`
	Provider string          `json:"provider,omitempty"`
	Model    string          `json:"model,omitempty"`
	Source   string          `json:"source"`
}

type NodeModelPolicy struct {
	DefaultProvider string                        `json:"default_provider,omitempty"`
	DefaultModel    string                        `json:"default_model,omitempty"`
	Selections      map[string]NodeModelSelection `json:"selections"`
}

func BuildDefaultNodeModelPolicy(cfg *dto.ChatConfig) NodeModelPolicy {
	provider := ""
	model := ""
	if cfg != nil {
		provider = strings.TrimSpace(cfg.Provider)
		model = strings.TrimSpace(cfg.ModelName)
	}
	policy := NodeModelPolicy{
		DefaultProvider: provider,
		DefaultModel:    model,
		Selections:      map[string]NodeModelSelection{},
	}
	for _, node := range []ModelPolicyNode{
		ModelPolicyNodeIntent,
		ModelPolicyNodePlanner,
		ModelPolicyNodeParamExtractor,
		ModelPolicyNodeFinalResponse,
		ModelPolicyNodeReviewer,
	} {
		policy.Selections[string(node)] = NodeModelSelection{
			Node:     node,
			Mode:     "inherit_default",
			Provider: provider,
			Model:    model,
			Source:   "agent_default",
		}
	}
	policy.Selections[string(ModelPolicyNodeRuntimeIntent)] = NodeModelSelection{
		Node:   ModelPolicyNodeRuntimeIntent,
		Mode:   "deterministic",
		Source: "runtime_command",
	}
	return policy
}

func (p NodeModelPolicy) Selection(node ModelPolicyNode) NodeModelSelection {
	if p.Selections == nil {
		return NodeModelSelection{}
	}
	return p.Selections[string(node)]
}
