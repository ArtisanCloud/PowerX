package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/core"
	"io"
	"net/http"
	"strings"
)

// Ollama /api/chat 的返回是 NDJSON（每行一个 JSON 对象），不是 SSE。
// 本实现：
// - Invoke：POST /api/chat + stream=false，解析最终 message.content
// - Stream：POST /api/chat + stream=true，逐行解析 message.content 增量，并回调 onDelta
// - 默认 endpoint = http://127.0.0.1:11434
// - 选项：温度/最大 tokens/TopP 映射到 options
// - 若服务端不支持流，返回 ErrStreamNotSupported（上层可选择回退到 Invoke）

type ollamaClient struct{}

func NewLLMClient() *ollamaClient { return &ollamaClient{} }

type ollamaChatReq struct {
	Model    string              `json:"model"`
	Messages []map[string]string `json:"messages"`
	Stream   bool                `json:"stream"`
	Think    *bool               `json:"think,omitempty"`
	Options  map[string]any      `json:"options,omitempty"`
}

type ollamaChatResp struct {
	Model      string `json:"model"`
	Done       bool   `json:"done"`
	DoneReason string `json:"done_reason,omitempty"`
	Error      string `json:"error,omitempty"`
	Message    struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	PromptEvalCount int `json:"prompt_eval_count,omitempty"`
	EvalCount       int `json:"eval_count,omitempty"`
}

func (c *ollamaClient) endpoint(mc *config.ModelConfig) string {
	base := strings.TrimRight(mc.Endpoint, "/")
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	return base + "/api/chat"
}

func (c *ollamaClient) httpClient(mc *config.ModelConfig) *http.Client {
	return &http.Client{Timeout: mc.Timeout}
}

func (c *ollamaClient) makeBody(mc *config.ModelConfig, userMessage string, streaming bool) ([]byte, error) {
	model := mc.Model
	if strings.TrimSpace(model) == "" {
		model = "llama3.1"
	}
	msgs := []map[string]string{
		{"role": "system", "content": mc.SystemPrompt},
		{"role": "user", "content": userMessage},
	}
	opts := map[string]any{}
	if mc.Temperature > 0 {
		opts["temperature"] = mc.Temperature
	}
	if mc.MaxTokens > 0 {
		// Ollama 对应字段叫 num_predict
		opts["num_predict"] = mc.MaxTokens
	}
	if mc.TopP > 0 {
		opts["top_p"] = mc.TopP
	}
	// 透传扩展参数（如果你在 ModelConfig 里有 Extra）
	var thinkPtr *bool
	for k, v := range mc.Extra {
		kl := strings.ToLower(strings.TrimSpace(k))
		if kl == "think" || kl == "thinking" {
			if b, ok := boolFromAny(v); ok {
				bv := b
				thinkPtr = &bv
			}
			continue
		}
		// 避免覆盖已有强约束字段
		if k == "temperature" || k == "num_predict" || k == "top_p" {
			continue
		}
		opts[k] = v
	}
	req := ollamaChatReq{
		Model:    model,
		Messages: msgs,
		Stream:   streaming,
		Think:    thinkPtr,
	}
	if len(opts) > 0 {
		req.Options = opts
	}
	return json.Marshal(req)
}

func boolFromAny(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes", "y", "on":
			return true, true
		case "false", "0", "no", "n", "off":
			return false, true
		}
	}
	return false, false
}

/* ------------------- Invoke（非流式） ------------------- */

func (c *ollamaClient) Invoke(ctx context.Context, mc *config.ModelConfig, userMessage string) (*config.InvokeResult, error) {
	body, err := c.makeBody(mc, userMessage, false)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(mc), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient(mc).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		bt, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama invoke status=%d body=%s", resp.StatusCode, string(bt))
	}

	var jr ollamaChatResp
	if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
		return nil, err
	}
	if jr.Error != "" {
		return nil, errors.New(jr.Error)
	}
	result := &config.InvokeResult{
		Text:         jr.Message.Content,
		FinishReason: strings.TrimSpace(jr.DoneReason),
	}
	usage := map[string]any{}
	if jr.PromptEvalCount > 0 {
		usage["prompt_tokens"] = jr.PromptEvalCount
	}
	if jr.EvalCount > 0 {
		usage["completion_tokens"] = jr.EvalCount
	}
	if pt, ok := usage["prompt_tokens"].(int); ok {
		if ct, ok2 := usage["completion_tokens"].(int); ok2 {
			usage["total_tokens"] = pt + ct
		}
	}
	if len(usage) > 0 {
		result.Usage = usage
	}
	return result, nil
}

/* ------------------- Stream（增量） ------------------- */

func (c *ollamaClient) Stream(ctx context.Context, mc *config.ModelConfig, prompt string, onDelta func(string)) (string, error) {
	body, err := c.makeBody(mc, prompt, true)
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(mc), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient(mc).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Ollama 的流是 NDJSON。如果返回不是 2xx，认为不支持或失败
	if resp.StatusCode/100 != 2 {
		// 让上层决定是否回退到 Invoke
		return "", core.ErrStreamNotSupported
	}

	reader := bufio.NewScanner(resp.Body)
	// 放大扫描缓冲，避免长行截断（默认 64K）
	buf := make([]byte, 0, 256*1024)
	reader.Buffer(buf, 2*1024*1024)

	var final strings.Builder
	for reader.Scan() {
		select {
		case <-ctx.Done():
			return final.String(), ctx.Err()
		default:
		}
		line := bytes.TrimSpace(reader.Bytes())
		if len(line) == 0 {
			continue
		}
		var chunk ollamaChatResp
		if err := json.Unmarshal(line, &chunk); err != nil {
			// 遇到非标准行，忽略继续
			continue
		}
		if chunk.Error != "" {
			return final.String(), errors.New(chunk.Error)
		}
		// 增量内容
		if s := chunk.Message.Content; s != "" {
			if onDelta != nil {
				onDelta(s)
			}
			final.WriteString(s)
		}
		// 结束
		if chunk.Done {
			break
		}
	}
	if err := reader.Err(); err != nil && !errors.Is(err, io.EOF) {
		return final.String(), err
	}
	return final.String(), nil
}
