package skills

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptTemplateExecutor_Execute(t *testing.T) {
	executor := newPromptTemplateExecutor()
	result, err := executor.Execute(context.Background(), ExecuteInput{
		TraceID:    "trace-prompt-template-unit",
		SkillID:    "skill.thirdparty.prompt-template",
		Version:    "1.0.0",
		Entrypoint: "runbook.default",
		Payload: map[string]interface{}{
			"template": "Hello {{name}}, ticket={{ticket_id}}",
			"variables": map[string]interface{}{
				"name":      "chengong",
				"ticket_id": "PX-42",
			},
		},
		Manifest: map[string]interface{}{
			"schema": "powerx.skill-manifest.v1",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "Hello chengong, ticket=PX-42", result["rendered_text"])
	require.Equal(t, float64(2), toFloat64(result["variables_used"]))
	require.Equal(t, "prompt-template", result["executor"])
}

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}
