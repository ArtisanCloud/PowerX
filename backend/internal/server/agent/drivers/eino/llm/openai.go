package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// 兼容现有接口
type openaiClient struct {
	NoopStream
	rawProvider string
}

func NewOpenAIClient(rawProvider string) LLMClient {
	return &openaiClient{rawProvider: strings.TrimSpace(rawProvider)}
}

const defaultOpenAIPath = "/v1/chat/completions"

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
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type,omitempty"`
		Code    any    `json:"code,omitempty"`
		Param   any    `json:"param,omitempty"`
	} `json:"error,omitempty"`
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
	path := resolveAPIPath(mc)
	url := joinEndpoint(base, path)
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

func sanitizeLLMURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		if len(raw) > 300 {
			return raw[:300] + "…"
		}
		return raw
	}
	u.Fragment = ""
	s := u.String()
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

func (c *openaiClient) logRequest(ctx context.Context, reqURL string, mc *config.ModelConfig, streaming bool) {
	provider := strings.TrimSpace(c.rawProvider)
	if provider == "" {
		provider = "openai"
	}
	logger.Info(ctx, "llm_request",
		zap.String("trace_id", audit.GetTraceID(ctx)),
		zap.String("tenant_uuid", reqctx.GetTenantUUID(ctx)),
		zap.String("driver", "openai"),
		zap.String("provider", provider),
		zap.String("model", strings.TrimSpace(mc.Model)),
		zap.Bool("stream", streaming),
		zap.Bool("azure", strings.TrimSpace(mc.AzureDeployment) != ""),
		zap.String("url", sanitizeLLMURL(reqURL)),
	)
}

func (c *openaiClient) httpClient(mc *config.ModelConfig) *http.Client {
	to := mc.Timeout
	if to <= 0 {
		// Chat/SSE 场景默认要足够长，避免“模型慢一点就被 30s 掐断”
		// 由上层 ctx 超时兜底（Engine 默认 10min）。
		to = 10 * time.Minute
	}
	return &http.Client{Timeout: to}
}

/* ------------ Invoke（非流） ------------ */

func (c *openaiClient) Invoke(ctx context.Context, mc *config.ModelConfig, userMessage string) (string, error) {
	start := time.Now()
	url, headers, err := c.buildEndpointAndHeaders(mc, false)
	if err != nil {
		return "", err
	}
	c.logRequest(ctx, url, mc, false)
	body, err := c.makeBody(mc, userMessage, false)
	if err != nil {
		return "", err
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header = headers

	resp, err := c.httpClient(mc).Do(req)
	if err != nil {
		logger.Info(ctx, "llm_response",
			zap.String("trace_id", audit.GetTraceID(ctx)),
			zap.String("driver", "openai"),
			zap.String("provider", strings.TrimSpace(c.rawProvider)),
			zap.String("model", strings.TrimSpace(mc.Model)),
			zap.Bool("stream", false),
			zap.Int64("latency_ms", time.Since(start).Milliseconds()),
			zap.String("error", err.Error()),
		)
		return "", err
	}
	defer resp.Body.Close()

	bt, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		logger.Info(ctx, "llm_response",
			zap.String("trace_id", audit.GetTraceID(ctx)),
			zap.String("driver", "openai"),
			zap.String("provider", strings.TrimSpace(c.rawProvider)),
			zap.String("model", strings.TrimSpace(mc.Model)),
			zap.Bool("stream", false),
			zap.Int("status", resp.StatusCode),
			zap.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return "", fmt.Errorf("openai invoke url=%s status=%d body=%s", url, resp.StatusCode, string(bt))
	}

	var jr openAINonStreamResp
	if err := json.Unmarshal(bt, &jr); err != nil {
		logger.Info(ctx, "llm_response",
			zap.String("trace_id", audit.GetTraceID(ctx)),
			zap.String("driver", "openai"),
			zap.String("provider", strings.TrimSpace(c.rawProvider)),
			zap.String("model", strings.TrimSpace(mc.Model)),
			zap.Bool("stream", false),
			zap.Int("status", resp.StatusCode),
			zap.Int64("latency_ms", time.Since(start).Milliseconds()),
			zap.String("error", err.Error()),
		)
		return "", fmt.Errorf("openai decode failed: %w (body=%s)", err, string(bt))
	}
	if jr.Error != nil && strings.TrimSpace(jr.Error.Message) != "" {
		logger.Info(ctx, "llm_response",
			zap.String("trace_id", audit.GetTraceID(ctx)),
			zap.String("driver", "openai"),
			zap.String("provider", strings.TrimSpace(c.rawProvider)),
			zap.String("model", strings.TrimSpace(mc.Model)),
			zap.Bool("stream", false),
			zap.Int("status", resp.StatusCode),
			zap.Int64("latency_ms", time.Since(start).Milliseconds()),
			zap.String("error", strings.TrimSpace(jr.Error.Message)),
		)
		return "", fmt.Errorf("openai error: %s", strings.TrimSpace(jr.Error.Message))
	}
	if len(jr.Choices) == 0 {
		// 兼容诊断：部分 OpenAI-compatible 网关会返回腾讯云 TC3 风格结构（Response.Error.*）
		// 此时 choices 为空但 StatusCode 可能仍为 2xx，容易误判为 “no choices”。
		var tr struct {
			Response struct {
				Error *struct {
					Code    string `json:"Code"`
					Message string `json:"Message"`
				} `json:"Error"`
			} `json:"Response"`
		}
		if e := json.Unmarshal(bt, &tr); e == nil && tr.Response.Error != nil && strings.TrimSpace(tr.Response.Error.Message) != "" {
			code := strings.TrimSpace(tr.Response.Error.Code)
			msg := strings.TrimSpace(tr.Response.Error.Message)
			if code != "" {
				return "", fmt.Errorf("openai-compatible: unexpected tencent response url=%s code=%s message=%s", url, code, msg)
			}
			return "", fmt.Errorf("openai-compatible: unexpected tencent response url=%s message=%s", url, msg)
		}

		trim := string(bt)
		if len(trim) > 2000 {
			trim = trim[:2000] + "…"
		}
		logger.Info(ctx, "llm_response",
			zap.String("trace_id", audit.GetTraceID(ctx)),
			zap.String("driver", "openai"),
			zap.String("provider", strings.TrimSpace(c.rawProvider)),
			zap.String("model", strings.TrimSpace(mc.Model)),
			zap.Bool("stream", false),
			zap.Int("status", resp.StatusCode),
			zap.Int64("latency_ms", time.Since(start).Milliseconds()),
			zap.String("error", "empty choices"),
		)
		return "", fmt.Errorf("openai: empty choices (url=%s body=%s)", url, trim)
	}
	logger.Info(ctx, "llm_response",
		zap.String("trace_id", audit.GetTraceID(ctx)),
		zap.String("driver", "openai"),
		zap.String("provider", strings.TrimSpace(c.rawProvider)),
		zap.String("model", strings.TrimSpace(mc.Model)),
		zap.Bool("stream", false),
		zap.Int("status", resp.StatusCode),
		zap.Int64("latency_ms", time.Since(start).Milliseconds()),
	)
	return jr.Choices[0].Message.Content, nil
}

/* ------------ Stream（优先流；不支持时自动回退） ------------ */

func (c *openaiClient) Stream(ctx context.Context, mc *config.ModelConfig, prompt string, onDelta func(string)) (string, error) {
	start := time.Now()
	// 若上层“配置禁用流”，你可以在外部判断；这里即便请求流，也能自动回退到 Invoke
	url, headers, err := c.buildEndpointAndHeaders(mc, true)
	if err != nil {
		return "", err
	}
	c.logRequest(ctx, url, mc, true)
	body, err := c.makeBody(mc, prompt, true)
	if err != nil {
		return "", err
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header = headers

	resp, err := c.httpClient(mc).Do(req)
	if err != nil {
		logger.Info(ctx, "llm_response",
			zap.String("trace_id", audit.GetTraceID(ctx)),
			zap.String("driver", "openai"),
			zap.String("provider", strings.TrimSpace(c.rawProvider)),
			zap.String("model", strings.TrimSpace(mc.Model)),
			zap.Bool("stream", true),
			zap.Int64("latency_ms", time.Since(start).Milliseconds()),
			zap.String("error", err.Error()),
		)
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
		logger.Info(ctx, "llm_response",
			zap.String("trace_id", audit.GetTraceID(ctx)),
			zap.String("driver", "openai"),
			zap.String("provider", strings.TrimSpace(c.rawProvider)),
			zap.String("model", strings.TrimSpace(mc.Model)),
			zap.Bool("stream", true),
			zap.Int("status", resp.StatusCode),
			zap.Int64("latency_ms", time.Since(start).Milliseconds()),
			zap.Bool("fallback_invoke", true),
			zap.String("content_type", resp.Header.Get("Content-Type")),
		)
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
	logger.Info(ctx, "llm_response",
		zap.String("trace_id", audit.GetTraceID(ctx)),
		zap.String("driver", "openai"),
		zap.String("provider", strings.TrimSpace(c.rawProvider)),
		zap.String("model", strings.TrimSpace(mc.Model)),
		zap.Bool("stream", true),
		zap.Int("status", resp.StatusCode),
		zap.Int64("latency_ms", time.Since(start).Milliseconds()),
	)
	return final.String(), nil
}

func resolveAPIPath(mc *config.ModelConfig) string {
	if mc != nil && mc.Extra != nil {
		if raw, ok := mc.Extra["api_path"]; ok {
			if s, ok2 := raw.(string); ok2 {
				s = strings.TrimSpace(s)
				if s != "" {
					if !strings.HasPrefix(s, "/") {
						s = "/" + s
					}
					return s
				}
			}
		}
	}
	return defaultOpenAIPath
}

func joinEndpoint(base, path string) string {
	trimmed := strings.TrimRight(base, "/")
	if trimmed == "" {
		trimmed = base
	}
	// 兼容 OpenAI-compatible 网关：base_url 常见自带 /v1
	// 若同时使用默认 path "/v1/..."，会拼出 "/v1/v1/..." 进而返回非预期 schema（choices 为空）。
	if strings.HasSuffix(trimmed, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	} else if strings.HasSuffix(trimmed, "/v1") && path == "/v1" {
		path = ""
	}
	return trimmed + path
}
