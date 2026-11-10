package openai

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/modelparams"
)

func (c *openaiClient) chatPolicy(mc *config.ModelConfig) *modelparams.Profile {
	provider := c.rawProvider
	if provider == "" {
		provider = mc.Provider
	}
	if provider == "" {
		provider = "openai"
	}
	if p := mc.ParameterProfile; p != nil && p.Provider == provider && p.Model == mc.Model {
		return p
	}
	return modelparams.Known(provider, mc.Model)
}

// 目录、表单及运行时共用型号参数合同；不通过生成请求探测支持情况。
func applyChatParameterPolicy(body map[string]any, policy *modelparams.Profile) error {
	if policy == nil {
		return nil
	}
	if policy.TokenParameter == "max_completion_tokens" {
		if tokens, ok := body["max_tokens"]; ok {
			if _, explicit := body["max_completion_tokens"]; !explicit {
				body["max_completion_tokens"] = tokens
			}
			delete(body, "max_tokens")
		}
	}
	effort, _ := body["reasoning_effort"].(string)
	if effort == "none" && policy.Sampling == "unsupported" {
		return fmt.Errorf("openai: model %s does not support reasoning_effort=none", body["model"])
	}
	if effort != "" && len(policy.ReasoningEfforts) > 0 {
		valid := false
		for _, candidate := range policy.ReasoningEfforts {
			if candidate == effort {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("openai: model %s does not support reasoning_effort=%s", body["model"], effort)
		}
	}
	for _, key := range []string{"temperature", "top_p", "logprobs", "top_logprobs"} {
		if !policy.SupportsSampling(effort) || !policy.Supports(key) {
			delete(body, key)
		}
	}
	return nil
}
