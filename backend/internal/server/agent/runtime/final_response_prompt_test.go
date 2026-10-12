package runtime

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildModeSpecificSystemPromptRequiresLocalizedMarkdownContract(t *testing.T) {
	prompt, err := BuildModeSpecificSystemPrompt("base prompt", &ResponsePlan{ResponseMode: ResponseModeNormalChat}, "zh-CN")
	require.NoError(t, err)
	require.Contains(t, prompt, "[RESPONSE_FORMAT]")
	require.Contains(t, prompt, "Markdown")
	require.Contains(t, prompt, "##")
}

func TestBuildModeSpecificSystemPromptRejectsUndeclaredLocale(t *testing.T) {
	_, err := BuildModeSpecificSystemPrompt("base prompt", &ResponsePlan{ResponseMode: ResponseModeNormalChat}, "")
	require.ErrorContains(t, err, "agent.final_response_locale_required")
}
