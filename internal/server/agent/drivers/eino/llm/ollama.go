package llm

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// services/agent/drivers/eino/llm/ollama.go

import (
	"context"
	"encoding/json"
)

type ollamaClient struct{}

func (c *ollamaClient) ChatOnce(ctx context.Context, mc ModelConfig, userMessage string) (string, error) {
	if mc.Model == "" {
		mc.Model = "llama3.1"
	}
	body := map[string]any{
		"model": mc.Model,
		"messages": []map[string]string{
			{"role": "system", "content": mc.SystemPrompt},
			{"role": "user", "content": userMessage},
		},
		"stream": false,
	}
	bs, _ := json.Marshal(body)

	url := strings.TrimRight(mc.Endpoint, "/") + "/api/chat"
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bs))
	req.Header.Set("Content-Type", "application/json")

	cli := &http.Client{Timeout: 60 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		bt, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama status=%d body=%s", resp.StatusCode, string(bt))
	}

	var jr struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
		return "", err
	}
	return jr.Message.Content, nil
}
