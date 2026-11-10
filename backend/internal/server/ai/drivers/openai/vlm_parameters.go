package openai

import (
	"github.com/ArtisanCloud/PowerX/internal/server/ai/modelparams"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// ValidateVLMParameters 校验目标型号的跨参数约束；可用于保存、目录与实际调用。
func ValidateVLMParameters(model string, params map[string]any) error {
	policy := modelparams.Known("openai", model)
	effort, _ := params["reasoning_effort"].(string)
	if policy.SupportsSampling(effort) {
		return nil
	}
	for _, field := range []string{"temperature", "top_p"} {
		if _, ok := params[field]; ok {
			return &dto.AppError{HTTPCode: 422, Code: "AI_PARAMETER_INVALID", Message: "AI_PARAMETER_INVALID", Details: map[string]any{"parameter": field, "reason": "requires_reasoning_effort_none"}}
		}
	}
	return nil
}
