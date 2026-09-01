package skills

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManifestExecutor_RoutesByExecutorTypeNotSkillID(t *testing.T) {
	called := 0
	executor := NewManifestExecutor(ManifestExecutorOptions{
		LLM: func(_ context.Context, input ManifestLLMInvocation) (string, error) {
			called++
			require.Equal(t, "completely.user.defined.skill", input.SkillID)
			require.Equal(t, "custom prompt", input.PromptTemplate)
			return "# 已完成\n\n- 结果", nil
		},
	})
	result, err := executor.Execute(context.Background(), ExecuteInput{
		SkillID:  "completely.user.defined.skill",
		Manifest: map[string]any{"executor": map[string]any{"type": "llm_prompt", "prompt_template_i18n": map[string]any{"zh-CN": "custom prompt"}}},
		Context:  map[string]any{"locale": "zh-CN"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, called)
	require.Equal(t, "markdown", result["format"])
}

func TestManifestExecutor_InstructionOnlyFailsExplicitly(t *testing.T) {
	executor := NewManifestExecutor(ManifestExecutorOptions{})
	_, err := executor.Execute(context.Background(), ExecuteInput{
		Manifest: map[string]any{"executor": map[string]any{"type": "instruction_only"}},
	})
	require.ErrorContains(t, err, "skill.executor_instruction_only_not_runnable")
}

func TestManifestExecutor_ResponseEnvelopeOutputIsStructured(t *testing.T) {
	executor := NewManifestExecutor(ManifestExecutorOptions{
		LLM: func(_ context.Context, _ ManifestLLMInvocation) (string, error) {
			return `{"schema":"powerx.agent.response/v3","kind":"review_result","outcome":"completed","presentation":{"facts":[{"statement":"已提供数据","source":{"type":"input","ref":"input:message"}}],"metrics":[],"hypotheses":[],"gaps":[],"actions":[]}}`, nil
		},
	})
	result, err := executor.Execute(context.Background(), ExecuteInput{
		Manifest: map[string]any{"executor": map[string]any{"type": "llm_prompt", "output_mode": "response_envelope", "prompt_template_i18n": map[string]any{"zh-CN": "contract prompt"}}},
		Context:  map[string]any{"locale": "zh-CN"},
	})
	require.NoError(t, err)
	envelope, ok := result["response_envelope"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "powerx.agent.response/v3", envelope["schema"])
}
