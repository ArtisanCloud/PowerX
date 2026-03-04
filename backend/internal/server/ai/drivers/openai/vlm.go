package openai

import (
	"context"
	"errors"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
)

type vlmClient struct{}

func NewVLMClient() *vlmClient { return &vlmClient{} }

func (c *vlmClient) Invoke(ctx context.Context, in contract.VLMRequest) (*contract.VLMResponse, error) {
	return nil, errors.New("vlm provider not implemented")
}

func (c *vlmClient) Stream(ctx context.Context, in contract.VLMRequest, onDelta func(string)) (*contract.VLMResponse, error) {
	return nil, errors.New("vlm provider not implemented")
}
