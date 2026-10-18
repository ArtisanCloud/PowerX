package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeStrictJSONRejectsAmbiguousDocuments(t *testing.T) {
	for _, raw := range []string{
		`{"value":"first","value":"second"}`, `{"v\u0061lue":"first","value":"second"}`,
		`{"value":"ok","unknown":1}`, `{"value":"ok"} {}`, `{"value":"ok"} trailing`,
		"{\"value\":\"\xff\"}", strings.Repeat("[", 130) + strings.Repeat("]", 130),
	} {
		t.Run(raw[:min(len(raw), 40)], func(t *testing.T) {
			var value struct {
				Value string `json:"value"`
			}
			require.Error(t, DecodeStrictJSON([]byte(raw), &value))
		})
	}
	var value struct {
		Value string `json:"value"`
	}
	require.NoError(t, DecodeStrictJSON([]byte(`{"value":"正确"}`), &value))
	require.Equal(t, "正确", value.Value)
}
