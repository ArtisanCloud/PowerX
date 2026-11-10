package catalog

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMediaParametersRejectUnknownTypesAndKeepExplicitZero(t *testing.T) {
	minimum, maximum := 0.0, 2.0
	op := MediaOperation{Parameters: map[string]ParameterSchema{"temperature": {Type: "number", Minimum: &minimum, Maximum: &maximum}, "size": {Type: "string", Enum: []any{"1024x1024"}}}}
	field, reason := ValidateMediaParameters(op, map[string]any{"temperature": 0})
	require.Empty(t, field)
	require.Empty(t, reason)
	for _, test := range []struct {
		params map[string]any
		reason string
	}{{map[string]any{"temperature": "0"}, "invalid_type"}, {map[string]any{"temperature": 3}, "out_of_range"}, {map[string]any{"seed": 0}, "unsupported_parameter"}, {map[string]any{"size": "bad"}, "invalid_enum"}} {
		_, reason = ValidateMediaParameters(op, test.params)
		require.Equal(t, test.reason, reason)
	}
}
func TestMediaModelKeysPreserveAppAndNativeColon(t *testing.T) {
	require.Equal(t, "provider/native:tag", (MediaModel{Provider: "provider", Model: ModelManifest{ID: "native:tag"}}).ModelKey())
	require.Equal(t, "provider/app:model", (MediaModel{Provider: "provider", App: "app", Model: ModelManifest{ID: "model"}}).ModelKey())
}
