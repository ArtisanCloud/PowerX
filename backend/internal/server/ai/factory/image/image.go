package image

import (
	"context"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
)

// Client defines the Image driver interface.
type Client interface {
	Generate(ctx context.Context, in contract.ImageRequest) (*contract.ImageResponse, error)
	Cap() contract.ModelCapabilities
	Health(ctx context.Context) error
}
