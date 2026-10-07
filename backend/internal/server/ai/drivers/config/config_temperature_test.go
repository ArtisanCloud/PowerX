package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeConfigKeepsExplicitZeroTemperature(t *testing.T) {
	merged := MergeConfig(&ModelConfig{Temperature: 0.7}, &ModelConfig{TemperatureSet: true})
	require.Zero(t, merged.Temperature)
	require.True(t, merged.TemperatureSet)

	unspecified := MergeConfig(&ModelConfig{Temperature: 0.7}, &ModelConfig{})
	require.Equal(t, 0.7, unspecified.Temperature)
}
