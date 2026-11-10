package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func referencePNG(t *testing.T, shade uint8) []byte {
	t.Helper()
	var out bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: shade, A: 255})
	require.NoError(t, png.Encode(&out, img))
	return out.Bytes()
}
func referencePart(data []byte) contract.ContentPart {
	return contract.ContentPart{Type: "image_url", URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)}
}
func referenceRequest(endpoint string, refs ...contract.ContentPart) contract.ImageRequest {
	return contract.ImageRequest{Prompt: "图1保持人物，图2保持装束", Size: "1024x1024", Quality: "auto", Format: "png", RefImages: refs, Runtime: map[string]any{"config": &config.ModelConfig{Model: "gpt-image-2", APIKey: "fixture-only", Endpoint: endpoint}, "n": 1}}
}
func requireImageCode(t *testing.T, err error, code string) *dto.AppError {
	t.Helper()
	var failure *dto.AppError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, code, failure.Code)
	return failure
}

func TestImageReferenceWirePreservesBytesAndOrder(t *testing.T) {
	first, second := referencePNG(t, 20), referencePNG(t, 200)
	for _, count := range []int{0, 1, 2, 16} {
		t.Run(strings.Repeat("i", count)+"images", func(t *testing.T) {
			refs := []contract.ContentPart{}
			for i := 0; i < count; i++ {
				if i%2 == 0 {
					refs = append(refs, referencePart(first))
				} else {
					refs = append(refs, referencePart(second))
				}
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				path := "/v1/images/edits"
				if count == 0 {
					path = "/v1/images/generations"
				}
				require.Equal(t, path, r.URL.Path)
				require.Equal(t, "POST", r.Method)
				require.Equal(t, "Bearer fixture-only", r.Header.Get("Authorization"))
				require.Equal(t, "application/json", r.Header.Get("Content-Type"))
				var body struct {
					Model, Prompt, Quality, Size string
					Format                       string                 `json:"output_format"`
					Images                       []openAIImageReference `json:"images"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, "gpt-image-2", body.Model)
				require.Equal(t, "图1保持人物，图2保持装束", body.Prompt)
				require.Equal(t, "auto", body.Quality)
				require.Equal(t, "png", body.Format)
				require.Len(t, body.Images, count)
				for i, ref := range body.Images {
					require.Equal(t, refs[i].URL, ref.URL)
				}
				w.Header().Set("X-Request-Id", "provider-reference-trace")
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(first)}}})
			}))
			defer server.Close()
			out, err := NewImageClient("openai").Generate(context.Background(), referenceRequest(server.URL+"/v1", refs...))
			require.NoError(t, err)
			require.Equal(t, [][]byte{first}, out.Images)
			require.Equal(t, "provider-reference-trace", out.TraceID)
			require.Equal(t, 1, calls)
		})
	}
}

func TestImageReferenceRejectsInvalidInputsBeforeProvider(t *testing.T) {
	data := referencePNG(t, 20)
	largePixels := append([]byte(nil), data...)
	binary.BigEndian.PutUint32(largePixels[16:20], 10000)
	binary.BigEndian.PutUint32(largePixels[20:24], 10000)
	binary.BigEndian.PutUint32(largePixels[29:33], crc32.ChecksumIEEE(largePixels[12:29]))
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	for _, test := range []struct {
		name, code string
		change     func(*contract.ImageRequest)
	}{
		{"count", "AI_INPUT_LIMIT_EXCEEDED", func(in *contract.ImageRequest) { in.RefImages = make([]contract.ContentPart, 17) }},
		{"base64", "AI_INPUT_INVALID", func(in *contract.ImageRequest) { in.RefImages[0].URL = "data:image/png;base64,broken!" }},
		{"mime", "AI_INPUT_INVALID", func(in *contract.ImageRequest) {
			in.RefImages[0].URL = strings.Replace(in.RefImages[0].URL, "image/png", "image/jpeg", 1)
		}},
		{"damaged", "AI_INPUT_INVALID", func(in *contract.ImageRequest) { in.RefImages[0] = referencePart(data[:len(data)/2]) }},
		{"not-image", "AI_INPUT_INVALID", func(in *contract.ImageRequest) { in.RefImages[0] = referencePart([]byte("private body")) }},
		{"format", "AI_INPUT_UNSUPPORTED", func(in *contract.ImageRequest) { in.RefImages[0].URL = "data:image/gif;base64,R0lG" }},
		{"remote", "AI_INPUT_UNSUPPORTED", func(in *contract.ImageRequest) { in.RefImages[0].URL = "https://example.invalid/image.png" }},
		{"pixels", "AI_INPUT_LIMIT_EXCEEDED", func(in *contract.ImageRequest) { in.RefImages[0] = referencePart(largePixels) }},
		{"bytes", "AI_INPUT_LIMIT_EXCEEDED", func(in *contract.ImageRequest) {
			in.RefImages[0] = referencePart(bytes.Repeat([]byte{0}, ImageReferenceMaxBytes+1))
		}},
		{"total", "AI_INPUT_LIMIT_EXCEEDED", func(in *contract.ImageRequest) {
			padded := append(append([]byte{}, data...), make([]byte, (4<<20)+1)...)
			in.RefImages = make([]contract.ContentPart, 5)
			for i := range in.RefImages {
				in.RefImages[i] = referencePart(padded)
			}
		}},
		{"quality", "AI_PARAMETER_INVALID", func(in *contract.ImageRequest) { in.Quality = "ultra" }},
		{"size", "AI_PARAMETER_INVALID", func(in *contract.ImageRequest) { in.Size = "1x1" }},
		{"output", "AI_PARAMETER_INVALID", func(in *contract.ImageRequest) { in.Format = "gif" }},
		{"fractional-count", "AI_PARAMETER_INVALID", func(in *contract.ImageRequest) { in.Runtime["n"] = 1.5 }},
		{"input-fidelity", "AI_PARAMETER_INVALID", func(in *contract.ImageRequest) { in.Runtime["input_fidelity"] = "high" }},
		{"channel-cap", "AI_INPUT_LIMIT_EXCEEDED", func(in *contract.ImageRequest) {
			in.RefImages = append(in.RefImages, referencePart(data))
			in.Runtime["config"].(*config.ModelConfig).Extra = map[string]any{"image_edit_max_images": 1}
		}},
		{"channel-disabled", "AI_INPUT_UNSUPPORTED", func(in *contract.ImageRequest) {
			in.Runtime["config"].(*config.ModelConfig).Extra = map[string]any{"image_edit_max_images": 0}
		}},
		{"unknown-path", "AI_INPUT_UNSUPPORTED", func(in *contract.ImageRequest) {
			in.Runtime["config"].(*config.ModelConfig).Extra = map[string]any{"api_path": "/custom/generate"}
		}},
		{"arbitrary-url", "AI_MODEL_CONFIGURATION_INVALID", func(in *contract.ImageRequest) {
			in.Runtime["config"].(*config.ModelConfig).Extra = map[string]any{"image_edit_api_path": "https://example.invalid/edits"}
		}},
		{"unsupported-model", "AI_INPUT_UNSUPPORTED", func(in *contract.ImageRequest) { in.Runtime["config"].(*config.ModelConfig).Model = "dall-e-3" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			in := referenceRequest(server.URL, referencePart(data))
			test.change(&in)
			_, err := NewImageClient("openai").Generate(context.Background(), in)
			requireImageCode(t, err, test.code)
		})
	}
	require.Zero(t, calls.Load())
}

func TestImageReferenceAcceptsRawBase64AndExplicitChannelPath(t *testing.T) {
	data := referencePNG(t, 100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/custom-edits", r.URL.Path)
		var body openAIImageReq
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, referencePart(data).URL, body.Images[0].URL)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(data)}}})
	}))
	defer server.Close()
	in := referenceRequest(server.URL+"/v1", contract.ContentPart{Type: "image_base64", Text: base64.StdEncoding.EncodeToString(data)})
	in.Runtime["config"].(*config.ModelConfig).Extra = map[string]any{"api_path": "/v1/custom-generate", "image_edit_api_path": "/v1/custom-edits", "image_edit_max_images": 2}
	_, err := NewImageClient("openai").Generate(context.Background(), in)
	require.NoError(t, err)
}

func TestImageReferenceProviderFailureKeepsSafeDiagnostics(t *testing.T) {
	for _, status := range []int{409, 200} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-Id", "provider-error-trace")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"image_policy_denied","message":"private prompt body and token","param":"images"}}`))
		}))
		_, err := NewImageClient("openai").Generate(context.Background(), referenceRequest(server.URL, referencePart(referencePNG(t, 10))))
		server.Close()
		failure := requireImageCode(t, err, "AI_PROVIDER_REJECTED")
		require.Equal(t, "provider-error-trace", failure.Details["provider_trace_id"])
		require.Equal(t, "image_policy_denied", failure.Details["upstream_code"])
		require.Equal(t, status, failure.Details["upstream_status"])
		require.NotContains(t, failure.Error(), "private")
	}
}

func TestImageResponseLargerThanEightMiBIsComplete(t *testing.T) {
	data := bytes.Repeat([]byte{1}, 7<<20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(data)}}})
	}))
	defer server.Close()
	out, err := NewImageClient("openai").Generate(context.Background(), referenceRequest(server.URL))
	require.NoError(t, err)
	require.Equal(t, [][]byte{data}, out.Images)
}

func TestImageReferenceChannelConfigurationRejectsInvalidBindings(t *testing.T) {
	for _, path := range []string{"/v1/images/edits?", "/v1/images/edits#", "/v1/../edits", "//other-host/edits", "/v1/%2e%2e/edits", "/v1/{path}", "/v1/*"} {
		_, err := ImageReferenceLimit(&config.ModelConfig{Model: "gpt-image-2", Extra: map[string]any{"image_edit_api_path": path}})
		requireImageCode(t, err, "AI_MODEL_CONFIGURATION_INVALID")
	}
	for _, limit := range []any{-1, 17, 1.5, "2", true} {
		_, err := ImageReferenceLimit(&config.ModelConfig{Model: "gpt-image-2", Extra: map[string]any{"image_edit_max_images": limit}})
		requireImageCode(t, err, "AI_MODEL_CONFIGURATION_INVALID")
	}
}
