package catalog

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVisionClassificationUsesOperationsAndKeepsGenerationSeparate(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.LoadByConfig(CatalogConfig{Dirs: []string{"../../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	vision, err := r.Models("vlm", "openai")
	require.NoError(t, err)
	require.Contains(t, vision[0].Operations, VisionUnderstanding)
	for _, m := range vision {
		require.Equal(t, "gpt-6-luna", m.ID)
		require.Equal(t, 1, m.Operations[VisionUnderstanding].MaxImages)
	}
	legacy, err := r.Models("vlm", "qwen-cn")
	require.NoError(t, err)
	require.NotEmpty(t, legacy)
	images, err := r.Models("image", "qwen-cn")
	require.NoError(t, err)
	for _, m := range images {
		require.NotContains(t, m.Operations, VisionUnderstanding)
	}
	for _, p := range r.Providers("vlm") {
		if p.ID == "openai" {
			return
		}
	}
	t.Fatal("OpenAI is missing from vision providers")
}
