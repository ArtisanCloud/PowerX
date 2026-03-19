package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/core"
)

type vlmClient struct{}

func NewVLMClient() *vlmClient { return &vlmClient{} }

func (c *vlmClient) Invoke(ctx context.Context, in contract.VLMRequest) (*contract.VLMResponse, error) {
	mc, err := modelConfigFromRuntime(in.Runtime)
	if err != nil {
		return nil, err
	}
	if mc == nil {
		return nil, errors.New("openai vlm: missing model config")
	}
	if strings.TrimSpace(mc.Model) == "" {
		return nil, errors.New("openai vlm: missing model")
	}

	body, err := c.makeBody(mc, in, false)
	if err != nil {
		return nil, err
	}
	url, headers, err := (&openaiClient{}).buildEndpointAndHeaders(mc, false)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = headers

	resp, err := (&openaiClient{}).httpClient(mc).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai vlm status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage map[string]int `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("openai vlm decode failed: %w", err)
	}
	if out.Error != nil && strings.TrimSpace(out.Error.Message) != "" {
		return nil, errors.New(strings.TrimSpace(out.Error.Message))
	}
	if len(out.Choices) == 0 {
		return nil, errors.New("openai vlm: empty choices")
	}

	return &contract.VLMResponse{
		Text:     contentToText(out.Choices[0].Message.Content),
		Usage:    out.Usage,
		Provider: strings.TrimSpace(mc.Provider),
		Model:    strings.TrimSpace(mc.Model),
	}, nil
}

func (c *vlmClient) Stream(ctx context.Context, in contract.VLMRequest, onDelta func(string)) (*contract.VLMResponse, error) {
	mc, err := modelConfigFromRuntime(in.Runtime)
	if err != nil {
		return nil, err
	}
	if mc == nil {
		return nil, errors.New("openai vlm: missing model config")
	}
	body, err := c.makeBody(mc, in, true)
	if err != nil {
		return nil, err
	}
	url, headers, err := (&openaiClient{}).buildEndpointAndHeaders(mc, true)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = headers

	httpResp, err := (&openaiClient{}).httpClient(mc).Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	ct := strings.ToLower(httpResp.Header.Get("Content-Type"))
	isSSE := strings.Contains(ct, "text/event-stream")
	if httpResp.StatusCode/100 != 2 || !isSSE {
		return c.Invoke(ctx, in)
	}

	type openAIVLMStreamDelta struct {
		Choices []struct {
			Delta struct {
				Content any `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	reader := bufio.NewReader(httpResp.Body)
	var final strings.Builder
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return &contract.VLMResponse{
				Text:     final.String(),
				Provider: strings.TrimSpace(mc.Provider),
				Model:    strings.TrimSpace(mc.Model),
			}, readErr
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == ':' {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := core.TrimDataPrefix(line)
		if bytes.EqualFold(bytes.TrimSpace(payload), []byte("[DONE]")) {
			break
		}
		var chunk openAIVLMStreamDelta
		if err := json.Unmarshal(payload, &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := contentToText(chunk.Choices[0].Delta.Content)
		if strings.TrimSpace(delta) == "" {
			continue
		}
		if onDelta != nil {
			onDelta(delta)
		}
		final.WriteString(delta)
	}
	return &contract.VLMResponse{
		Text:     final.String(),
		Provider: strings.TrimSpace(mc.Provider),
		Model:    strings.TrimSpace(mc.Model),
	}, nil
}

func (c *vlmClient) makeBody(mc *config.ModelConfig, in contract.VLMRequest, streaming bool) ([]byte, error) {
	req := map[string]any{
		"model":    strings.TrimSpace(mc.Model),
		"messages": buildVLMMessages(in.Messages),
		"stream":   streaming,
	}
	if in.Temperature > 0 {
		req["temperature"] = in.Temperature
	}
	if in.TopP > 0 {
		req["top_p"] = in.TopP
	}
	if in.MaxTokens > 0 {
		req["max_tokens"] = in.MaxTokens
	}
	if in.JSONMode {
		req["response_format"] = map[string]any{"type": "json_object"}
	}
	for k, v := range mc.Extra {
		switch k {
		case "model", "messages", "stream", "temperature", "top_p", "max_tokens":
			continue
		default:
			req[k] = v
		}
	}
	return json.Marshal(req)
}

func buildVLMMessages(messages []contract.Message) []map[string]any {
	if len(messages) == 0 {
		return []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": "Describe the image."},
				},
			},
		}
	}
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		content := make([]map[string]any, 0, len(msg.Content))
		for _, part := range msg.Content {
			switch strings.ToLower(strings.TrimSpace(part.Type)) {
			case contract.ContentTypeImageURL:
				if strings.TrimSpace(part.URL) == "" {
					continue
				}
				content = append(content, map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": strings.TrimSpace(part.URL),
					},
				})
			default:
				txt := strings.TrimSpace(part.Text)
				if txt == "" {
					continue
				}
				content = append(content, map[string]any{
					"type": "text",
					"text": txt,
				})
			}
		}
		if len(content) == 0 {
			continue
		}
		out = append(out, map[string]any{
			"role":    role,
			"content": content,
		})
	}
	if len(out) == 0 {
		out = append(out, map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "Describe the image."},
			},
		})
	}
	return out
}

func contentToText(content any) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			txt, _ := m["text"].(string)
			if strings.TrimSpace(txt) == "" {
				continue
			}
			parts = append(parts, strings.TrimSpace(txt))
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}
