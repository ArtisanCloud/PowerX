package eino

import (
	"github.com/ArtisanCloud/PowerX/internal/server/ai/modelparams"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestModelConfigRetainsSavedSamplingAndReasoning(t *testing.T) {
	p := modelparams.Known("openai", "gpt-6-luna")
	c := modelConfigFromChatConfig(&dto.ChatConfig{Provider: "openai", ModelName: "gpt-6-luna", Temperature: 0, TemperatureSet: true, TopP: .8, TopPSet: true, ReasoningEffort: "none", ParameterProfile: p})
	require.True(t, c.TemperatureSet)
	require.EqualValues(t, 0, c.Temperature)
	require.InDelta(t, .8, c.TopP, .0001)
	require.Equal(t, "none", c.Extra["reasoning_effort"])
	require.Equal(t, p, c.ParameterProfile)
}
