package tts

import (
	"context"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
)

type Client interface {
	Synthesize(ctx context.Context, in contract.TTSRequest) (*contract.TTSResponse, error)
}
