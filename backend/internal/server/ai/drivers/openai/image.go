package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

const defaultOpenAIImagePath = "/v1/images/generations"

type imageClient struct {
	rawProvider string
}

func NewImageClient(rawProvider string) *imageClient {
	return &imageClient{rawProvider: strings.TrimSpace(rawProvider)}
}

func (c *imageClient) Cap() contract.ModelCapabilities {
	return contract.ModelCapabilities{}
}

func (c *imageClient) Health(ctx context.Context) error {
	return nil
}

type openAIImageReq struct {
	Model             string `json:"model"`
	Prompt            string `json:"prompt"`
	Size              string `json:"size,omitempty"`
	Quality           string `json:"quality,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	ResponseFormat    string `json:"response_format,omitempty"`
	N                 int    `json:"n,omitempty"`
	Background        string `json:"background,omitempty"`
	OutputCompression int    `json:"output_compression,omitempty"`
}

type openAIImageResp struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	Usage map[string]int `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type,omitempty"`
		Code    any    `json:"code,omitempty"`
		Param   any    `json:"param,omitempty"`
	} `json:"error,omitempty"`
}

func (c *imageClient) Generate(ctx context.Context, in contract.ImageRequest) (*contract.ImageResponse, error) {
	mc, err := modelConfigFromRuntime(in.Runtime)
	if err != nil {
		return nil, err
	}
	if mc == nil {
		return nil, errors.New("openai image: missing model config")
	}
	if strings.TrimSpace(mc.AzureDeployment) != "" {
		return nil, errors.New("openai image: azure deployment not supported yet")
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return nil, errors.New("openai image: prompt required")
	}
	if len(in.RefImages) > 0 {
		return nil, errors.New("openai image: reference images not supported")
	}

	req := openAIImageReq{
		Model:        strings.TrimSpace(mc.Model),
		Prompt:       prompt,
		Size:         strings.TrimSpace(in.Size),
		Quality:      normalizeImageQuality(strings.TrimSpace(in.Quality), mc.Model),
		OutputFormat: normalizeImageFormat(strings.TrimSpace(in.Format)),
	}

	if in.Runtime != nil {
		if v, ok := in.Runtime["response_format"]; ok {
			if s, ok2 := v.(string); ok2 {
				req.ResponseFormat = strings.TrimSpace(s)
			}
		}
		if v, ok := in.Runtime["n"]; ok {
			if n := intFromRuntime(v); n > 0 {
				req.N = n
			}
		}
		if v, ok := in.Runtime["background"]; ok {
			if s, ok2 := v.(string); ok2 {
				req.Background = strings.TrimSpace(s)
			}
		}
		if v, ok := in.Runtime["output_compression"]; ok {
			if n := intFromRuntime(v); n > 0 {
				req.OutputCompression = n
			}
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	endpoint, headers, err := buildImageEndpointAndHeaders(mc)
	if err != nil {
		return nil, err
	}

	httpClient := imageHTTPClient(mc)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header = headers

	start := time.Now()
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	logger.InfoF(ctx, "[ai-image] openai http_call provider=%s model=%s status=%d latency_ms=%d", strings.TrimSpace(c.rawProvider), strings.TrimSpace(mc.Model), resp.StatusCode, time.Since(start).Milliseconds())

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai image status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out openAIImageResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.Error != nil && strings.TrimSpace(out.Error.Message) != "" {
		return nil, errors.New(out.Error.Message)
	}
	if len(out.Data) == 0 {
		return nil, errors.New("openai image: empty response")
	}

	imgs := make([][]byte, 0, len(out.Data))
	urls := make([]string, 0, len(out.Data))
	for _, item := range out.Data {
		if strings.TrimSpace(item.B64JSON) != "" {
			decoded, err := base64.StdEncoding.DecodeString(item.B64JSON)
			if err != nil {
				return nil, fmt.Errorf("openai image: decode base64 failed: %w", err)
			}
			imgs = append(imgs, decoded)
		} else if strings.TrimSpace(item.URL) != "" {
			urls = append(urls, strings.TrimSpace(item.URL))
		}
	}
	return &contract.ImageResponse{
		Images:    imgs,
		ImageURLs: urls,
		Provider:  strings.TrimSpace(c.rawProvider),
		Model:     strings.TrimSpace(mc.Model),
		Usage:     out.Usage,
		LatencyMS: int(time.Since(start).Milliseconds()),
	}, nil
}

func intFromRuntime(val any) int {
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return 0
}

func modelConfigFromRuntime(runtime map[string]any) (*config.ModelConfig, error) {
	if runtime == nil {
		return nil, errors.New("image runtime config missing")
	}
	if raw, ok := runtime["config"]; ok {
		switch v := raw.(type) {
		case *config.ModelConfig:
			return v, nil
		case config.ModelConfig:
			return &v, nil
		}
	}
	if raw, ok := runtime["model_config"]; ok {
		switch v := raw.(type) {
		case *config.ModelConfig:
			return v, nil
		case config.ModelConfig:
			return &v, nil
		}
	}
	return nil, errors.New("image runtime config invalid")
}

func buildImageEndpointAndHeaders(mc *config.ModelConfig) (string, http.Header, error) {
	base := strings.TrimRight(mc.Endpoint, "/")
	if base == "" {
		base = "https://api.openai.com"
	}
	if strings.TrimSpace(mc.APIKey) == "" {
		return "", nil, errors.New("openai image: missing api_key")
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+strings.TrimSpace(mc.APIKey))
	h.Set("Content-Type", "application/json")
	if strings.TrimSpace(mc.Organization) != "" {
		h.Set("OpenAI-Organization", strings.TrimSpace(mc.Organization))
	}
	path := resolveImageAPIPath(mc)
	return joinEndpoint(base, path), h, nil
}

func resolveImageAPIPath(mc *config.ModelConfig) string {
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
	return defaultOpenAIImagePath
}

func imageHTTPClient(mc *config.ModelConfig) *http.Client {
	to := mc.Timeout
	if to <= 0 {
		to = 5 * time.Minute
	}
	return &http.Client{Timeout: to}
}

func normalizeImageQuality(q, model string) string {
	if q == "" {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(q))
	modelLower := strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(modelLower, "gpt-image") {
		switch lower {
		case "standard":
			return "medium"
		case "hd":
			return "high"
		case "auto", "low", "medium", "high":
			return lower
		}
		return lower
	}
	if strings.Contains(modelLower, "dall-e-3") {
		switch lower {
		case "hd", "standard":
			return lower
		case "high":
			return "hd"
		default:
			return "standard"
		}
	}
	if strings.Contains(modelLower, "dall-e-2") {
		return "standard"
	}
	return lower
}

func normalizeImageFormat(format string) string {
	if format == "" {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(format))
	switch lower {
	case "png", "jpeg", "jpg", "webp":
		if lower == "jpg" {
			return "jpeg"
		}
		return lower
	default:
		return lower
	}
}
