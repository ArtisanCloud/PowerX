package ai

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/stretchr/testify/require"
)

func TestTextModelProfileUsesDeclaredModalityAndRejectsMisclassifiedMedia(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fixture.yaml"), []byte(`id: fixture
name: fixture
modalities:
  llm:
    models:
      - id: neutral-text
      - id: speech-in-name-but-text
  audio_tts:
    models:
      - id: neutral-a
  image:
    models:
      - id: neutral-b
  video:
    models:
      - id: neutral-c
apps:
  studio:
    modalities:
      llm:
        models:
          - id: app-text
      audio_tts:
        models:
          - id: app-audio
`), 0600))
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{dir}, FailIfEmpty: true}, nil))
	for _, model := range []string{"neutral-text", "speech-in-name-but-text", "app-text", "explicit-custom-text"} {
		require.True(t, textModelProfile(reg, dbModelProfileView{Modality: "llm", Provider: "fixture", Model: model}), model)
	}
	for _, model := range []string{"neutral-a", "neutral-b", "neutral-c", "app-audio", "NEUTRAL-A"} {
		require.False(t, textModelProfile(reg, dbModelProfileView{Modality: "llm", Provider: "fixture", Model: model}), model)
	}
	for _, modality := range []string{"", "image", "video", "audio_tts", "vlm"} {
		require.False(t, textModelProfile(reg, dbModelProfileView{Modality: modality, Provider: "fixture", Model: "neutral-text"}), modality)
	}
}
