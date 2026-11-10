package qwen

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"strings"
)

type ttsClient struct{}

func NewTTSClient() *ttsClient { return &ttsClient{} }

func (c *ttsClient) Synthesize(ctx context.Context, in contract.TTSRequest) (*contract.TTSResponse, error) {
	mc, err := modelConfigFromRuntime(in.Runtime)
	if err != nil {
		return nil, err
	}
	if mc == nil {
		return nil, fmt.Errorf("qwen tts: missing model config")
	}
	if strings.TrimSpace(mc.APIKey) == "" {
		return nil, fmt.Errorf("qwen tts: missing api_key")
	}
	model := resolveModel(mc.Model)
	if model == "" {
		return nil, fmt.Errorf("qwen tts: missing model")
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil, fmt.Errorf("qwen tts: empty text")
	}

	input := map[string]any{"text": text, "voice": in.Voice}
	if language, ok := in.Runtime["language_type"]; ok {
		input["language_type"] = language
	}
	out, err := nativeMediaCall(ctx, mc, map[string]any{"model": model, "input": input})
	if err != nil {
		return nil, err
	}
	var audio []byte
	if out.Output.Audio.Data != "" {
		audio, err = base64.StdEncoding.DecodeString(out.Output.Audio.Data)
		if err != nil {
			return nil, fmt.Errorf("qwen audio payload invalid")
		}
	}
	if len(audio) == 0 && out.Output.Audio.URL == "" {
		return nil, fmt.Errorf("qwen audio output missing")
	}
	return &contract.TTSResponse{Audio: audio, AudioURL: out.Output.Audio.URL, Provider: mc.Provider, Model: mc.Model, TraceID: out.RequestID, Usage: out.Usage}, nil
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}
