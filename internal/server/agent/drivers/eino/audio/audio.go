package audio

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/config"
)

// internal/server/agent/drivers/eino/llm/audio.go
type AudioTTSClient interface {
	TTS(ctx context.Context, mc config.ModelConfig, text string) ([]byte, error)
}
type AudioASRClient interface {
	Transcribe(ctx context.Context, mc config.ModelConfig, audio []byte, mime string) (string, error)
}
