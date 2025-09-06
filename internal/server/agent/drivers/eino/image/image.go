package image

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino"
)

// internal/server/agent/drivers/eino/llm/image.go
type ImageClient interface {
	GenerateImage(ctx context.Context, mc eino.ModelConfig, prompt string, opt ImageOptions) ([]byte, error)
}
type ImageOptions struct {
	Size   string // 1024x1024
	Format string // png|jpeg|webp
}
