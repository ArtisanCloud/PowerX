package video

import (
	"context"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
)

type Client interface {
	Generate(ctx context.Context, in contract.VideoRequest) (*contract.VideoResponse, error)
}
