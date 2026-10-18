package skills

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPluginToolSchemaRequiresExplicitClosedSupportedContract(t *testing.T) {
	metadata := map[string]PluginToolField{"/value": {FieldKey: "value", LabelKey: "quantity", Unit: "cm", Scope: "parcel"}}
	labels := map[string]map[string]string{"en-US": {"quantity": "fixture"}, "zh-CN": {"quantity": "fixture"}}
	for _, raw := range []string{
		`{"type":"object","properties":{"value":{"type":"number"}},"required":["value"]}`,
		`{"type":"object","additionalProperties":true,"properties":{"value":{"type":"number"}},"required":["value"]}`,
		`{"type":"object","additionalProperties":false,"properties":{"value":{"type":"number","maximum":99}},"required":["value"]}`,
		`{"type":"object","additionalProperties":false,"properties":{"value":{"$ref":"https://untrusted/schema"}},"required":["value"]}`,
	} {
		_, err := compilePluginSchema(context.Background(), PluginToolSchema{Definition: json.RawMessage(raw), Fields: metadata}, labels, false)
		require.Error(t, err)
	}
	raw := `{"type":"object","additionalProperties":false,"properties":{"value":{"type":"number"}},"required":["value"]}`
	schema, err := compilePluginSchema(context.Background(), PluginToolSchema{Definition: json.RawMessage(raw), Fields: metadata}, labels, false)
	require.NoError(t, err)
	require.NoError(t, validatePluginJSON(schema, []byte(`{"value":12.5}`)))
	require.Error(t, validatePluginJSON(schema, []byte(`{"value":12.5,"extra":1}`)))
	require.Error(t, validatePluginJSON(schema, []byte(`{"value":12.5,"value":99}`)))
}
