package google

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

const defaultGeminiEndpoint = "https://generativelanguage.googleapis.com/v1beta"

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

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiImageConfig struct {
	AspectRatio string `json:"aspectRatio,omitempty"`
	ImageSize   string `json:"imageSize,omitempty"`
}

type geminiGenerationConfig struct {
	ResponseModalities []string           `json:"responseModalities,omitempty"`
	ImageConfig        *geminiImageConfig `json:"imageConfig,omitempty"`
}

type geminiGenerateReq struct {
	Contents         []geminiContent         `json:"contents"`
	GenerationConfig *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiGenerateResp struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text       string            `json:"text,omitempty"`
				InlineData *geminiInlineData `json:"inlineData,omitempty"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

func (c *imageClient) Generate(ctx context.Context, in contract.ImageRequest) (*contract.ImageResponse, error) {
	mc, err := modelConfigFromRuntime(in.Runtime)
	if err != nil {
		return nil, err
	}
	if mc == nil {
		return nil, errors.New("gemini image: missing model config")
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return nil, errors.New("gemini image: prompt required")
	}
	if len(in.RefImages) > 0 {
		return nil, errors.New("gemini image: reference images not supported")
	}
	if strings.TrimSpace(mc.Model) == "" {
		return nil, errors.New("gemini image: missing model")
	}

	aspectRatio, imageSize := normalizeGeminiImageConfig(strings.TrimSpace(in.Size))
	if in.Runtime != nil {
		if v, ok := in.Runtime["aspect_ratio"]; ok {
			if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
				aspectRatio = strings.TrimSpace(s)
			}
		}
		if v, ok := in.Runtime["image_size"]; ok {
			if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
				imageSize = strings.TrimSpace(s)
			}
		}
	}

	req := geminiGenerateReq{
		Contents: []geminiContent{{Parts: []geminiPart{{Text: prompt}}}},
		GenerationConfig: &geminiGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
			ImageConfig: &geminiImageConfig{
				AspectRatio: aspectRatio,
				ImageSize:   imageSize,
			},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	endpoint, headers, err := buildGeminiEndpointAndHeaders(mc)
	if err != nil {
		return nil, err
	}

	httpClient := geminiHTTPClient(mc)
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
	logger.InfoF(ctx, "[ai-image] gemini http_call provider=%s model=%s status=%d latency_ms=%d", strings.TrimSpace(c.rawProvider), strings.TrimSpace(mc.Model), resp.StatusCode, time.Since(start).Milliseconds())

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini image status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out geminiGenerateResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.Error != nil && strings.TrimSpace(out.Error.Message) != "" {
		return nil, errors.New(out.Error.Message)
	}

	imgs := make([][]byte, 0)
	if len(out.Candidates) > 0 {
		for _, part := range out.Candidates[0].Content.Parts {
			if part.InlineData == nil || strings.TrimSpace(part.InlineData.Data) == "" {
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
			if err != nil {
				return nil, fmt.Errorf("gemini image: decode base64 failed: %w", err)
			}
			imgs = append(imgs, decoded)
		}
	}
	if len(imgs) == 0 {
		return nil, errors.New("gemini image: empty response")
	}

	return &contract.ImageResponse{
		Images:    imgs,
		Provider:  strings.TrimSpace(c.rawProvider),
		Model:     strings.TrimSpace(mc.Model),
		LatencyMS: int(time.Since(start).Milliseconds()),
	}, nil
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

func buildGeminiEndpointAndHeaders(mc *config.ModelConfig) (string, http.Header, error) {
	base := strings.TrimRight(mc.Endpoint, "/")
	if base == "" {
		base = defaultGeminiEndpoint
	}
	if strings.TrimSpace(mc.APIKey) == "" {
		return "", nil, errors.New("gemini image: missing api_key")
	}
	if strings.TrimSpace(mc.Model) == "" {
		return "", nil, errors.New("gemini image: missing model")
	}
	path := "/models/" + strings.TrimPrefix(strings.TrimSpace(mc.Model), "/") + ":generateContent"
	endpoint := base + path

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("x-goog-api-key", strings.TrimSpace(mc.APIKey))
	return endpoint, h, nil
}

func normalizeGeminiImageConfig(size string) (string, string) {
	if size == "" {
		return "1:1", "1K"
	}
	width, height := parseSize(size)
	if width == 0 || height == 0 {
		return "1:1", "1K"
	}
	aspect := aspectRatioFromSize(width, height)
	imageSize := "1K"
	if width >= 1536 || height >= 1536 {
		imageSize = "2K"
	}
	return aspect, imageSize
}

func parseSize(size string) (int, int) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return 0, 0
	}
	w, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return w, h
}

func aspectRatioFromSize(width, height int) string {
	if width == 0 || height == 0 {
		return "1:1"
	}
	if width == height {
		return "1:1"
	}
	ratio := float64(width) / float64(height)
	switch {
	case ratio >= 1.65 && ratio <= 1.9:
		return "16:9"
	case ratio >= 1.45 && ratio < 1.65:
		return "3:2"
	case ratio >= 1.2 && ratio < 1.45:
		return "4:3"
	case ratio >= 0.6 && ratio <= 0.8:
		return "2:3"
	case ratio >= 0.8 && ratio < 0.9:
		return "3:4"
	case ratio < 0.6:
		return "9:16"
	default:
		return "1:1"
	}
}

func geminiHTTPClient(mc *config.ModelConfig) *http.Client {
	to := mc.Timeout
	if to <= 0 {
		to = 5 * time.Minute
	}
	return &http.Client{Timeout: to}
}
