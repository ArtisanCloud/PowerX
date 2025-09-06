package audio

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino"
)

// internal/server/agent/drivers/eino/llm/audio.go
type AudioTTSClient interface {
	TTS(ctx context.Context, mc eino.ModelConfig, text string) ([]byte, error)
}
type AudioASRClient interface {
	Transcribe(ctx context.Context, mc eino.ModelConfig, audio []byte, mime string) (string, error)
}
