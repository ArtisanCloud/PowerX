package runtime

import (
	"testing"

	sessions "github.com/ArtisanCloud/PowerX/internal/service/agent_session"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func TestServiceSessionFinalEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload any
		valid   bool
	}{
		{"valid", map[string]any{"success": true, "data": map[string]any{"content": "test-output"}}, true},
		{"failed", map[string]any{"success": false, "data": map[string]any{"content": "test-output"}}, false},
		{"missing_content", map[string]any{"success": true, "data": map[string]any{}}, false},
		{"wrong_shape", map[string]any{"content": "test-output"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := &serviceSessionFinalSink{}
			_ = sink.Emit(dto.EventToken, map[string]any{"delta": "must-not-become-final"})
			_ = sink.Emit(dto.EventFinal, tc.payload)
			output, err := sink.result()
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, "test-output", output)
			} else {
				require.ErrorIs(t, err, sessions.ErrDependency)
				require.Empty(t, output)
			}
		})
	}
	sink := &serviceSessionFinalSink{}
	_, err := sink.result()
	require.ErrorIs(t, err, sessions.ErrDependency)
	valid := map[string]any{"success": true, "data": map[string]any{"content": "test-output"}}
	require.NoError(t, sink.Emit(dto.EventFinal, valid))
	require.ErrorIs(t, sink.Emit(dto.EventFinal, valid), sessions.ErrDependency)
	_, err = sink.result()
	require.ErrorIs(t, err, sessions.ErrDependency)
}
