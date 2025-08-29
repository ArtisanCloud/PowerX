package llm

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// services/agent/drivers/eino/llm/openai.go

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"
)

type openaiClient struct{}

func (c *openaiClient) ChatOnce(ctx context.Context, mc ModelConfig, userMessage string) (string, error) {
	if mc.APIKey == "" {
		return "", errors.New("openai: missing api_key")
	}
	body := map[string]any{
		"model": mc.Model,
		"messages": []map[string]string{
			{"role": "system", "content": mc.SystemPrompt},
			{"role": "user", "content": userMessage},
		},
		"temperature": mc.Temperature,
		"max_tokens":  mc.MaxTokens,
	}
	bs, _ := json.Marshal(body)

	url := strings.TrimRight(mc.Endpoint, "/") + "/chat/completions"
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bs))
	req.Header.Set("Authorization", "Bearer "+mc.APIKey)
	req.Header.Set("Content-Type", "application/json")

	cli := &http.Client{Timeout: 30 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		bt, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai status=%d body=%s", resp.StatusCode, string(bt))
	}

	var jr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
		return "", err
	}
	if len(jr.Choices) == 0 {
		return "", errors.New("openai: no choices")
	}
	return jr.Choices[0].Message.Content, nil
}
