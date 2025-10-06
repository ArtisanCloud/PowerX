package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/config"
	"io"
	"net/http"
	"strings"
	"time"
)

// 兼容现有接口
type openaiClient struct{ NoopStream }

func NewOpenAIClient() LLMClient { return &openaiClient{} }

/* ------------ 公共结构 ------------ */

type openAIChatReq struct {
	Model       string              `json:"model"`
	Messages    []map[string]string `json:"messages"`
	Temperature float32             `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Stream      bool                `json:"stream,omitempty"`
	Stop        []string            `json:"stop,omitempty"`
	ResponseFmt map[string]any      `json:"response_format,omitempty"`
	Extra       map[string]any      `json:"-"`
}

type openAINonStreamResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			Role    string `json:"role"`
		} `json:"message"`
	} `json:"choices"`
}

type openAIStreamDelta struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
			Role    string `json:"role"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

/* ------------ 帮助方法 ------------ */

// 统一构造 URL 与头部
func (c *openaiClient) buildEndpointAndHeaders(mc *config.ModelConfig, streaming bool) (string, http.Header, error) {
	base := strings.TrimRight(mc.Endpoint, "/")
	if base == "" {
		base = "https://api.openai.com" // 默认
	}
	h := http.Header{}
	if mc.Organization != "" {
		h.Set("OpenAI-Organization", mc.Organization)
	}

	// Azure OpenAI：若配置了部署名，则拼 Azure 风格 URL 并用 api-key
	if mc.AzureDeployment != "" {
		// 期望 mc.APIKey 为 Azure Key，mc.APIVersion 为 Query
		if mc.APIKey == "" {
			return "", nil, errors.New("openai(azure): missing api_key")
		}
		if mc.APIVersion == "" {
			mc.APIVersion = "2024-02-01"
		}
		// https://{resource}.openai.azure.com/openai/deployments/{deployment}/chat/completions?api-version=...
		url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", base, mc.AzureDeployment, mc.APIVersion)
		h.Set("api-key", mc.APIKey)
		h.Set("Content-Type", "application/json")
		return url, h, nil
	}

	// 标准 OpenAI
	if mc.APIKey == "" {
		return "", nil, errors.New("openai: missing api_key")
	}
	h.Set("Authorization", "Bearer "+mc.APIKey)
	h.Set("Content-Type", "application/json")
	url := base + "/v1/chat/completions"
	return url, h, nil
}

func (c *openaiClient) makeBody(mc *config.ModelConfig, userMessage string, streaming bool) ([]byte, error) {
	req := openAIChatReq{
		Model: mc.Model,
		Messages: []map[string]string{
			{"role": "system", "content": mc.SystemPrompt}, // 若为空也无妨
			{"role": "user", "content": userMessage},
		},
		Temperature: float32(mc.Temperature),
		MaxTokens:   mc.MaxTokens,
		Stream:      streaming,
	}
	// 如需 JSON mode，可按你配置补上（示例）：
	// if mc.JSONMode {
	// 	req.ResponseFmt = map[string]any{"type": "json_object"}
	// }

	// 合并 Extra
	if len(mc.Extra) > 0 {
		req.Extra = mc.Extra
	}

	// 拼 JSON
	type alias openAIChatReq
	m := map[string]any{}
	b, _ := json.Marshal(alias(req))
	_ = json.Unmarshal(b, &m)
	for k, v := range req.Extra {
		m[k] = v
	}
	return json.Marshal(m)
}

func (c *openaiClient) httpClient(mc *config.ModelConfig) *http.Client {
	to := mc.Timeout
	if to <= 0 {
		to = 30 * time.Second
	}
	return &http.Client{Timeout: to}
}

/* ------------ Invoke（非流） ------------ */

func (c *openaiClient) Invoke(ctx context.Context, mc *config.ModelConfig, userMessage string) (string, error) {
	url, headers, err := c.buildEndpointAndHeaders(mc, false)
	if err != nil {
		return "", err
	}
	body, err := c.makeBody(mc, userMessage, false)
	if err != nil {
		return "", err
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header = headers

	resp, err := c.httpClient(mc).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		bt, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai invoke status=%d body=%s", resp.StatusCode, string(bt))
	}

	var jr openAINonStreamResp
	if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
		return "", err
	}
	if len(jr.Choices) == 0 {
		return "", errors.New("openai: no choices")
	}
	return jr.Choices[0].Message.Content, nil
}

/* ------------ Stream（优先流；不支持时自动回退） ------------ */

func (c *openaiClient) Stream(ctx context.Context, mc *config.ModelConfig, prompt string, onDelta func(string)) (string, error) {
	// 若上层“配置禁用流”，你可以在外部判断；这里即便请求流，也能自动回退到 Invoke
	url, headers, err := c.buildEndpointAndHeaders(mc, true)
	if err != nil {
		return "", err
	}
	body, err := c.makeBody(mc, prompt, true)
	if err != nil {
		return "", err
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header = headers

	resp, err := c.httpClient(mc).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	isSSE := strings.Contains(ct, "text/event-stream")

	// 不支持流：404/400/非 SSE，自动回退到 Invoke
	if resp.StatusCode/100 != 2 || !isSSE {
		// 读取错误体供调试
		if resp.StatusCode/100 != 2 {
			bt, _ := io.ReadAll(resp.Body)
			// 仍然尝试回退
			_ = bt
		}
		// 回退
		return c.Invoke(ctx, mc, prompt)
	}

	// 逐行读 SSE：data: {...}
	reader := bufio.NewReader(resp.Body)
	var final strings.Builder

	for {
		// 读取一行（包含 \n）
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return final.String(), err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == ':' {
			// 空行或注释，忽略
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := TrimDataPrefix(line) // 你已有的小工具

		// 结束标志
		if bytes.EqualFold(bytes.TrimSpace(payload), []byte("[DONE]")) {
			break
		}

		var chunk openAIStreamDelta
		if err := json.Unmarshal(payload, &chunk); err != nil {
			// 某些代理会在流中掺杂心跳行/其他结构；忽略不可解析的行
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		if delta != "" {
			if onDelta != nil {
				onDelta(delta)
			}
			final.WriteString(delta)
		}
	}
	return final.String(), nil
}
