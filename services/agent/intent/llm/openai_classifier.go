package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

type OpenAIClassifier struct {
	BaseURL string // https://api.openai.com/v1
	APIKey  string
	Model   string // gpt-4o-mini / gpt-4o / gpt-3.5-turbo 等
	Timeout time.Duration
	HTTP    *http.Client
}

type oaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type oaReq struct {
	Model       string  `json:"model"`
	Messages    []oaMsg `json:"messages"`
	Temperature float64 `json:"temperature,omitempty"`
}
type oaResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (o *OpenAIClassifier) Name() string { return "openai" }

func (o *OpenAIClassifier) client() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	to := o.Timeout
	if to <= 0 {
		to = 15 * time.Second
	}
	return &http.Client{Timeout: to}
}
func (o *OpenAIClassifier) endpoint() string {
	base := o.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	return base + "/chat/completions"
}

func (o *OpenAIClassifier) Classify(ctx context.Context, question string, cands []Candidate) (Result, error) {
	reqBody := oaReq{
		Model: o.Model,
		Messages: []oaMsg{
			{Role: "system", Content: SystemPrompt},
			{Role: "user", Content: BuildUserPrompt(question, cands)},
		},
		Temperature: 0,
	}
	bs, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint(), bytes.NewReader(bs))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.APIKey)
	}

	resp, err := o.client().Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return Result{}, fmt.Errorf("openai chat HTTP %d", resp.StatusCode)
	}

	var or oaResp
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return Result{}, err
	}
	if len(or.Choices) == 0 {
		return Result{}, fmt.Errorf("no choices")
	}
	txt := or.Choices[0].Message.Content
	return parseClassifierJSON(txt)
}

var jsonExtract = regexp.MustCompile(`\{[\s\S]*\}`)

func parseClassifierJSON(s string) (Result, error) {
	// 容错：从文本里提取第一个 JSON 对象
	m := jsonExtract.FindString(s)
	if m == "" {
		return Result{}, fmt.Errorf("no json in llm output")
	}
	var r Result
	if err := json.Unmarshal([]byte(m), &r); err != nil {
		return Result{}, err
	}
	// 规整置信度范围
	if r.Confidence < 0 {
		r.Confidence = 0
	}
	if r.Confidence > 1 {
		r.Confidence = 1
	}
	return r, nil
}
