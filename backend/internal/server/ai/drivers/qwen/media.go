package qwen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/core"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type imageClient struct{}

func NewImageClient() *imageClient                   { return &imageClient{} }
func (*imageClient) Cap() contract.ModelCapabilities { return contract.ModelCapabilities{} }
func (*imageClient) Health(context.Context) error    { return nil }

// nativeMediaEndpoint 只从受信任凭据配置解析原生 API，不使用调用方代理参数。
func nativeMediaEndpoint(mc *config.ModelConfig) (string, error) {
	u, err := url.Parse(strings.TrimSpace(mc.Endpoint))
	if err != nil || u.Host == "" || u.User != nil || u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("qwen media endpoint invalid")
	}
	u.Path = "/api/v1/services/aigc/multimodal-generation/generation"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

type nativeMediaOutput struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id"`
	Usage     map[string]int `json:"usage"`
	Output    struct {
		Audio struct {
			URL  string `json:"url"`
			Data string `json:"data"`
		} `json:"audio"`
		Choices []struct {
			Message struct {
				Content []struct {
					Image string `json:"image"`
				} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	} `json:"output"`
}

func nativeMediaCall(ctx context.Context, mc *config.ModelConfig, payload any) (*nativeMediaOutput, error) {
	endpoint, err := nativeMediaEndpoint(mc)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(mc.APIKey) == "" {
		return nil, fmt.Errorf("qwen media credential missing")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+mc.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: resolveTimeout(mc.Timeout)}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, core.MediaHTTPError(resp.StatusCode, resp.Header, raw)
	}
	var out nativeMediaOutput
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("qwen media response invalid")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || out.Code != "" {
		status := resp.StatusCode
		if status < 400 {
			status = 502
		}
		return nil, &dto.AppError{HTTPCode: status, Code: "AI_PROVIDER_REJECTED", Message: "AI_PROVIDER_REJECTED", Details: map[string]any{"upstream_code": out.Code, "upstream_status": resp.StatusCode, "provider_trace_id": out.RequestID}}
	}
	return &out, nil
}
func (*imageClient) Generate(ctx context.Context, in contract.ImageRequest) (*contract.ImageResponse, error) {
	mc, err := modelConfigFromRuntime(in.Runtime)
	if err != nil {
		return nil, err
	}
	if len(in.RefImages) > 0 {
		return nil, fmt.Errorf("qwen text-to-image does not accept reference images")
	}
	parameters := map[string]any{}
	for _, name := range []string{"size", "negative_prompt", "prompt_extend", "watermark", "seed", "n"} {
		if v, ok := in.Runtime[name]; ok {
			parameters[name] = v
		}
	}
	if in.Size != "" {
		parameters["size"] = in.Size
	}
	out, err := nativeMediaCall(ctx, mc, map[string]any{"model": resolveModel(mc.Model), "input": map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"text": in.Prompt}}}}}, "parameters": parameters})
	if err != nil {
		return nil, err
	}
	result := &contract.ImageResponse{Provider: mc.Provider, Model: mc.Model, TraceID: out.RequestID, Usage: out.Usage}
	for _, choice := range out.Output.Choices {
		for _, content := range choice.Message.Content {
			if content.Image != "" {
				result.ImageURLs = append(result.ImageURLs, content.Image)
			}
		}
	}
	if len(result.ImageURLs) == 0 {
		return nil, fmt.Errorf("qwen image output missing")
	}
	return result, nil
}
