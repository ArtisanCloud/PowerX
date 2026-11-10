package agnes

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
)

type imageClient struct{}

// NewImageClient 构造 Agnes 生图客户端；网络请求只在 Generate 中发生。
func NewImageClient() *imageClient                   { return &imageClient{} }
func (*imageClient) Cap() contract.ModelCapabilities { return contract.ModelCapabilities{} }
func (*imageClient) Health(context.Context) error    { return nil }

func (*imageClient) Generate(ctx context.Context, in contract.ImageRequest) (*contract.ImageResponse, error) {
	mc, err := modelConfig(in.Runtime)
	if err != nil {
		return nil, err
	}
	if !oneOf(mc.Model, "agnes-image-2.5-flash", "agnes-image-2.1-flash") {
		return nil, parameterError("model_key", "unsupported_model")
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return nil, parameterError("inputs", "prompt_required")
	}
	size := in.Size
	if size == "" {
		size = setting(in.Runtime, mc, "size", "1K")
	}
	if !oneOf(size, "1K", "2K", "3K", "4K") {
		return nil, parameterError("size", "invalid_enum")
	}
	ratio := setting(in.Runtime, mc, "ratio", "1:1")
	if !oneOf(ratio, "1:1", "3:4", "4:3", "16:9", "9:16", "2:3", "3:2", "21:9") {
		return nil, parameterError("ratio", "invalid_enum")
	}
	if len(in.RefImages) > 3 {
		return nil, parameterError("inputs", "too_many_images")
	}
	extra := map[string]any{"response_format": "b64_json"}
	payload := map[string]any{"model": mc.Model, "prompt": in.Prompt, "size": size, "ratio": ratio, "extra_body": extra}
	if len(in.RefImages) == 0 {
		payload["return_base64"] = true
	} else {
		images := []string{}
		for _, part := range in.RefImages {
			var ref string
			switch part.Type {
			case contract.ContentTypeImageURL:
				ref = part.URL
			}
			if strings.TrimSpace(ref) == "" {
				return nil, parameterError("inputs", "invalid_image")
			}
			images = append(images, ref)
		}
		extra["image"] = images
	}
	endpoint, err := apiURL(mc, "/v1/images/generations")
	if err != nil {
		return nil, err
	}
	var out struct {
		Data []struct {
			B64 string `json:"b64_json"`
			URL string `json:"url"`
		} `json:"data"`
		Usage     map[string]int `json:"usage"`
		RequestID string         `json:"request_id"`
	}
	start := time.Now()
	headers, err := call(ctx, mc, http.MethodPost, endpoint, payload, &out, 96<<20)
	if err != nil {
		return nil, err
	}
	if len(out.Data) != 1 {
		return nil, outputError()
	}
	result := &contract.ImageResponse{Provider: "agnes", Model: mc.Model, Usage: out.Usage, TraceID: out.RequestID, LatencyMS: int(time.Since(start).Milliseconds())}
	if result.TraceID == "" {
		result.TraceID = headers.Get("X-Request-ID")
	}
	item := out.Data[0]
	if item.B64 != "" {
		data, e := base64.StdEncoding.DecodeString(item.B64)
		if e != nil || len(data) == 0 || len(data) > 64<<20 {
			return nil, outputError()
		}
		result.Images = [][]byte{data}
	} else if strings.TrimSpace(item.URL) != "" {
		result.ImageURLs = []string{item.URL}
	} else {
		return nil, outputError()
	}
	return result, nil
}
