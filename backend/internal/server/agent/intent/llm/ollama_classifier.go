package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// 走 /api/chat
type OllamaClassifier struct {
	BaseURL string // http://127.0.0.1:11434
	Model   string // qwen2.5:7b-instruct / llama3.1:8b-instruct 等
	Timeout time.Duration
	HTTP    *http.Client
}

type olMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type olReq struct {
	Model    string         `json:"model"`
	Messages []olMsg        `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  map[string]any `json:"options,omitempty"`
}
type olResp struct {
	Message olMsg `json:"message"`
	Done    bool  `json:"done"`
}

func (o *OllamaClassifier) Name() string { return "ollama" }
func (o *OllamaClassifier) client() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	to := o.Timeout
	if to <= 0 {
		to = 20 * time.Second
	}
	return &http.Client{Timeout: to}
}
func (o *OllamaClassifier) endpoint() string {
	base := o.BaseURL
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	return base + "/api/chat"
}

func (o *OllamaClassifier) Classify(ctx context.Context, question string, cands []Candidate) (Result, error) {
	body := olReq{
		Model:  o.Model,
		Stream: false,
		Messages: []olMsg{
			{Role: "system", Content: SystemPrompt},
			{Role: "user", Content: BuildUserPrompt(question, cands)},
		},
		Options: map[string]any{
			"temperature": 0,
		},
	}
	bs, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint(), bytes.NewReader(bs))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client().Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return Result{}, fmt.Errorf("ollama chat HTTP %d", resp.StatusCode)
	}

	var or olResp
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return Result{}, err
	}
	return parseClassifierJSON(or.Message.Content)
}
