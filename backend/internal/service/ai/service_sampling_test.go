package ai

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRequestTemperature(t *testing.T) {
	value, present, err := parseRequestTemperature(nil)
	require.NoError(t, err)
	require.False(t, present)
	require.Zero(t, value)

	value, present, err = parseRequestTemperature(map[string]interface{}{"temperature": float64(0)})
	require.NoError(t, err)
	require.True(t, present)
	require.Zero(t, value)

	value, present, err = parseRequestTemperature(map[string]interface{}{"temperature": 0.001})
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, 0.001, value)

	for _, invalid := range []any{-0.1, "0", nil, math.NaN(), math.Inf(1)} {
		_, _, err = parseRequestTemperature(map[string]interface{}{"temperature": invalid})
		require.ErrorIs(t, err, ErrInvalidLLMParams)
	}
}

func TestParseOllamaSeed(t *testing.T) {
	for _, input := range []any{float64(0), float64(42), json.Number("42"), int64(-7)} {
		_, present, err := parseOllamaSeed("ollama", map[string]interface{}{"seed": input})
		require.NoError(t, err)
		require.True(t, present)
	}
	for _, invalid := range []any{1.5, "42", nil, math.NaN(), float64(1 << 53), json.Number("1.5")} {
		_, _, err := parseOllamaSeed("ollama", map[string]interface{}{"seed": invalid})
		require.True(t, errors.Is(err, ErrInvalidLLMParams))
	}
	_, _, err := parseOllamaSeed("openai", map[string]interface{}{"seed": 42})
	require.ErrorIs(t, err, ErrInvalidLLMParams)
}
