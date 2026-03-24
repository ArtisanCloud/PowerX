package runtime

import (
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

const baseFlowDirectGuardrail = "当用户请求是确定性文本操作（如原样返回、提取、格式化）时，直接输出结果，不要展开分析、不要解释步骤。"

func applyBaseFlowDirectGuardrail(reqCfg *dto.ChatConfig) *dto.ChatConfig {
	if reqCfg == nil {
		return &dto.ChatConfig{SystemPrompt: baseFlowDirectGuardrail}
	}
	out := *reqCfg
	cur := strings.TrimSpace(out.SystemPrompt)
	if strings.Contains(cur, baseFlowDirectGuardrail) {
		return &out
	}
	if cur == "" {
		out.SystemPrompt = baseFlowDirectGuardrail
		return &out
	}
	out.SystemPrompt = cur + "\n" + baseFlowDirectGuardrail
	return &out
}
