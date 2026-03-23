package skillsintegration

import (
	"testing"
	"time"

	eino "github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino"
	"github.com/stretchr/testify/require"
)

func TestSkillAgentPromptCache_StrategyMatrix(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		mode      string
		wantMode  string
		supported bool
		enabled   bool
	}{
		{name: "openai auto", provider: "openai", mode: "auto", wantMode: "auto", supported: true, enabled: true},
		{name: "anthropic auto", provider: "anthropic", mode: "auto", wantMode: "auto", supported: true, enabled: true},
		{name: "gemini auto", provider: "gemini", mode: "auto", wantMode: "auto", supported: true, enabled: true},
		{name: "ollama auto", provider: "ollama", mode: "auto", wantMode: "auto", supported: false, enabled: false},
		{name: "ollama force on", provider: "ollama", mode: "force_on", wantMode: "force_on", supported: false, enabled: true},
		{name: "openai force off", provider: "openai", mode: "force_off", wantMode: "force_off", supported: true, enabled: false},
		{name: "invalid mode fallback auto", provider: "openai", mode: "xxx", wantMode: "auto", supported: true, enabled: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := eino.ResolvePromptCacheStrategy(tc.provider, tc.mode)
			require.Equal(t, tc.wantMode, got.Mode)
			require.Equal(t, tc.supported, got.Supported)
			require.Equal(t, tc.enabled, got.Enabled)
		})
	}
}

func TestSkillAgentPromptCache_DecisionLatencyRegression(t *testing.T) {
	start := time.Now()
	enabledCount := 0
	for i := 0; i < 20000; i++ {
		got := eino.ResolvePromptCacheStrategy("openai", "auto")
		if got.Enabled {
			enabledCount++
		}
	}
	cost := time.Since(start)

	require.Equal(t, 20000, enabledCount)
	require.Less(t, cost, 2*time.Second)
}
