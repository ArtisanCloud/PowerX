package qwen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
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
		return nil, fmt.Errorf("qwen vlm: missing model config")
	}
	if strings.TrimSpace(mc.APIKey) == "" {
		return nil, fmt.Errorf("qwen vlm: missing api_key")
	}
	model := resolveModel(mc.Model)
	if model == "" {
		return nil, fmt.Errorf("qwen vlm: missing model")
	}

	reqBody := map[string]any{
		"model":    model,
		"messages": buildMessages(in.Messages),
		"stream":   false,
	}
	if in.Temperature > 0 || in.Runtime["temperature_set"] == true {
		reqBody["temperature"] = in.Temperature
	}
	if in.TopP > 0 {
		reqBody["top_p"] = in.TopP
	}
	if in.MaxTokens > 0 {
		reqBody["max_tokens"] = in.MaxTokens
	}
	if in.JSONMode {
		reqBody["response_format"] = map[string]any{"type": "json_object"}
	}

	rawReq, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	base := resolveEndpoint(mc.Endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(rawReq))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(mc.APIKey))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: resolveTimeout(mc.Timeout)}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, core.MediaHTTPError(resp.StatusCode, resp.Header, body)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage map[string]int `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("qwen vlm decode failed: %w", err)
	}
	if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
		return nil, fmt.Errorf("qwen vlm error: %s", strings.TrimSpace(result.Error.Message))
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("qwen vlm: empty choices")
	}

	return &contract.VLMResponse{
		Text:     contentToText(result.Choices[0].Message.Content),
		Usage:    result.Usage,
		Provider: strings.TrimSpace(mc.Provider),
		Model:    strings.TrimSpace(mc.Model),
	}, nil
}

func (c *vlmClient) Stream(ctx context.Context, in contract.VLMRequest, onDelta func(string)) (*contract.VLMResponse, error) {
	_ = onDelta
	return c.Invoke(ctx, in)
}

func resolveTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 120 * time.Second
	}
	return timeout
}

func buildMessages(messages []contract.Message) []map[string]any {
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
