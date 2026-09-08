package openai

import (
	"encoding/json"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/stretchr/testify/require"
)

func TestMakeBodyPassesTypedResponseSchemaAsOpenAIJSONSchema(t *testing.T) {
	schema := map[string]any{"type": "object", "required": []string{"schema"}}
	body, err := NewLLMClient("openai").makeBody(&config.ModelConfig{
		Model:          "gpt-test",
		ResponseSchema: schema,
	}, "test", false)
	require.NoError(t, err)

	var request map[string]any
	require.NoError(t, json.Unmarshal(body, &request))
	responseFormat := request["response_format"].(map[string]any)
	require.Equal(t, "json_schema", responseFormat["type"])
	jsonSchema := responseFormat["json_schema"].(map[string]any)
	encodedSchema := jsonSchema["schema"].(map[string]any)
	require.Equal(t, "object", encodedSchema["type"])
	require.Equal(t, []any{"schema"}, encodedSchema["required"])
}
