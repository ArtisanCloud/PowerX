package vlm

import (
	"context"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
)

// Client defines the VLM driver interface.
type Client interface {
	Invoke(ctx context.Context, in contract.VLMRequest) (*contract.VLMResponse, error)
	Stream(ctx context.Context, in contract.VLMRequest, onDelta func(string)) (*contract.VLMResponse, error)
}
