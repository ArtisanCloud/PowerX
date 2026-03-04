package qwen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
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

	body := map[string]any{
		"model": model,
		"input": text,
	}
	if voice := strings.TrimSpace(in.Voice); voice != "" {
		body["voice"] = voice
	}
	if speed := in.Speed; speed > 0 {
		body["speed"] = speed
	}
	if format := strings.TrimSpace(in.Format); format != "" {
		body["response_format"] = format
	}
	if mc.Extra != nil {
		for k, v := range mc.Extra {
			if _, exists := body[k]; exists {
				continue
			}
			body[k] = v
		}
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	base := resolveEndpoint(mc.Endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/audio/speech", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(mc.APIKey))
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := (&http.Client{Timeout: resolveTimeout(mc.Timeout)}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("qwen tts status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.Contains(ct, "application/json") {
		var out struct {
			Audio      string `json:"audio"`
			AudioBase64 string `json:"audio_base64"`
			Output     struct {
				AudioURL   string `json:"audio_url"`
				Audio      string `json:"audio"`
				AudioBase64 string `json:"audio_base64"`
			} `json:"output"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Usage map[string]int `json:"usage"`
		}
		if err := json.Unmarshal(payload, &out); err != nil {
			return nil, fmt.Errorf("qwen tts decode failed: %w", err)
		}
		if out.Error != nil && strings.TrimSpace(out.Error.Message) != "" {
			return nil, fmt.Errorf("qwen tts error: %s", strings.TrimSpace(out.Error.Message))
		}
		audioURL := strings.TrimSpace(out.Output.AudioURL)
		audioBase64 := firstNonEmpty(out.AudioBase64, out.Audio, out.Output.AudioBase64, out.Output.Audio)
		var audio []byte
		if strings.TrimSpace(audioBase64) != "" {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(audioBase64))
			if err == nil {
				audio = decoded
			}
		}
		return &contract.TTSResponse{
			Audio:     audio,
			AudioURL:  audioURL,
			Provider:  strings.TrimSpace(mc.Provider),
			Model:     strings.TrimSpace(mc.Model),
			Usage:     out.Usage,
			LatencyMS: int(time.Since(start).Milliseconds()),
		}, nil
	}

	return &contract.TTSResponse{
		Audio:     payload,
		Provider:  strings.TrimSpace(mc.Provider),
		Model:     strings.TrimSpace(mc.Model),
		LatencyMS: int(time.Since(start).Milliseconds()),
	}, nil
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}
