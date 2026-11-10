package agnes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

func TestImageWireContractAndResults(t *testing.T) {
	for _, reference := range []bool{false, true} {
		t.Run(fmt.Sprint(reference), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/v1/images/generations", r.URL.Path)
				require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, "2K", body["size"])
				require.Equal(t, "4:3", body["ratio"])
				require.NotContains(t, body, "response_format")
				extra := body["extra_body"].(map[string]any)
				require.Equal(t, "b64_json", extra["response_format"])
				if reference {
					require.NotContains(t, body, "return_base64")
					require.Equal(t, []any{"data:image/png;base64,fixture"}, extra["image"])
				} else {
					require.Equal(t, true, body["return_base64"])
					require.NotContains(t, extra, "image")
				}
				w.Header().Set("X-Request-ID", "image-trace")
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString([]byte("fixture-image"))}}})
			}))
			defer server.Close()
			in := contract.ImageRequest{Prompt: "fixture", Size: "2K", Runtime: map[string]any{"config": &config.ModelConfig{Endpoint: server.URL + "/v1", APIKey: "fixture-key", Model: "agnes-image-2.5-flash"}, "ratio": "4:3"}}
			if reference {
				in.RefImages = []contract.ContentPart{{Type: contract.ContentTypeImageURL, URL: "data:image/png;base64,fixture"}}
			}
			out, err := NewImageClient().Generate(context.Background(), in)
			require.NoError(t, err)
			require.Equal(t, [][]byte{[]byte("fixture-image")}, out.Images)
			require.Equal(t, "image-trace", out.TraceID)
		})
	}
}

func TestVideoSubmissionUsesVideoIDAndModeFields(t *testing.T) {
	for _, model := range []string{"agnes-video-2.5", "agnes-video-2.5-flash"} {
		for _, mode := range []string{"text", "keyframe", "reference"} {
			t.Run(model+"/"+mode, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, http.MethodPost, r.Method)
					require.Equal(t, "/v1/videos", r.URL.Path)
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, model, body["model"])
					require.Equal(t, "4", body["seconds"])
					require.Equal(t, "720P", body["size"])
					require.Equal(t, mode, body["mode"])
					require.Equal(t, "9:16", body["aspect_ratio"])
					require.EqualValues(t, 1, body["n"])
					require.NotContains(t, body, "extra_body")
					require.NotContains(t, body, "fps")
					if mode == "keyframe" {
						require.Equal(t, "https://example.com/first.png", body["first_frame"])
						require.Equal(t, "https://example.com/last.png", body["last_frame"])
						require.NotContains(t, body, "images")
					}
					if mode == "reference" {
						require.Equal(t, []any{"https://example.com/first.png", "https://example.com/last.png"}, body["images"])
						require.NotContains(t, body, "first_frame")
					}
					_, _ = w.Write([]byte(`{"id":"task_wrong","task_id":"task_also_wrong","video_id":"video_right"}`))
				}))
				defer server.Close()
				in := contract.VideoRequest{Prompt: "fixture", Resolution: "720p", MaxDurationS: 4, Runtime: map[string]any{"config": &config.ModelConfig{Endpoint: server.URL, APIKey: "fixture-key", Model: model}, "mode": mode, "aspect_ratio": "9:16"}}
				if mode != "text" {
					in.RefImages = []contract.ContentPart{{Type: contract.ContentTypeImageURL, URL: "https://example.com/first.png"}, {Type: contract.ContentTypeImageURL, URL: "https://example.com/last.png"}}
				}
				out, err := NewVideoClient().Submit(context.Background(), in)
				require.NoError(t, err)
				require.Equal(t, "video_right", out.TaskID)
			})
		}
	}
}

func TestVideoQueriesUsePublicStatusAndModelName(t *testing.T) {
	for _, sample := range []struct{ status, want string }{{"pending", "queued"}, {"queued", "queued"}, {"in_progress", "running"}, {"processing", "running"}, {"running", "running"}, {"completed", "succeeded"}, {"failed", "failed"}, {"cancelled", "cancelled"}} {
		t.Run(sample.status, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/agnesapi", r.URL.Path)
				require.Equal(t, "video_lookup", r.URL.Query().Get("video_id"))
				require.Equal(t, "agnes-video-2.5-flash", r.URL.Query().Get("model_name"))
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
				_ = json.NewEncoder(w).Encode(map[string]any{"status": sample.status, "url": "https://example.com/private-result.mp4", "internal_status": "pending", "internal_progress": 0, "error": map[string]any{"code": "fixture_code", "message": "private provider error"}})
			}))
			defer server.Close()
			out, err := NewVideoClient().Query(context.Background(), "video_lookup", map[string]any{"config": &config.ModelConfig{Endpoint: server.URL + "/v1", APIKey: "fixture-key", Model: "agnes-video-2.5-flash"}})
			require.NoError(t, err)
			require.Equal(t, sample.want, out.Status)
			if sample.want == "succeeded" {
				require.Equal(t, []string{"https://example.com/private-result.mp4"}, out.VideoURLs)
			} else {
				require.Empty(t, out.VideoURLs)
			}
			if sample.want == "failed" {
				require.Equal(t, "fixture_code", out.ErrorCode)
			}
		})
	}
}

func TestUnsupportedCancelOnlyQueriesAndNeverDeletes(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodGet, r.Method)
		_, _ = w.Write([]byte(`{"status":"running"}`))
	}))
	defer server.Close()
	out, err := NewVideoClient().Cancel(context.Background(), "video_test", map[string]any{"config": &config.ModelConfig{Endpoint: server.URL, APIKey: "fixture-key", Model: "agnes-video-2.5"}})
	require.NoError(t, err)
	require.Equal(t, "unsupported", out.CancelStatus)
	require.Equal(t, "running", out.Status)
	require.Equal(t, 1, calls)
}

func TestValidationStopsInvalidVideoBeforeSubmission(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	for name, mutate := range map[string]func(*contract.VideoRequest){
		"flash_resolution": func(in *contract.VideoRequest) { in.Resolution = "1080p" },
		"short_duration":   func(in *contract.VideoRequest) { in.MaxDurationS = 3 },
		"long_duration":    func(in *contract.VideoRequest) { in.MaxDurationS = 13 },
		"fps":              func(in *contract.VideoRequest) { in.FPS = 24 },
		"mode_media": func(in *contract.VideoRequest) {
			in.RefImages = []contract.ContentPart{{Type: contract.ContentTypeImageURL, URL: "https://example.com/a.png"}}
		},
		"missing_reference": func(in *contract.VideoRequest) { in.Runtime["mode"] = "reference" },
		"missing_frame":     func(in *contract.VideoRequest) { in.Runtime["mode"] = "keyframe" },
		"invalid_ratio":     func(in *contract.VideoRequest) { in.Runtime["aspect_ratio"] = "auto" },
		"video_reference": func(in *contract.VideoRequest) {
			in.RefVideos = []contract.ContentPart{{URL: "https://example.com/a.mp4"}}
		},
		"reference_limit": func(in *contract.VideoRequest) {
			in.Runtime["mode"] = "reference"
			for i := 0; i < 6; i++ {
				in.RefImages = append(in.RefImages, contract.ContentPart{Type: contract.ContentTypeImageURL, URL: "https://example.com/a.png"})
			}
		},
		"data_uri_video_reference": func(in *contract.VideoRequest) {
			in.Runtime["mode"] = "reference"
			in.RefImages = []contract.ContentPart{{Type: contract.ContentTypeImageURL, URL: "data:image/png;base64,private"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			in := contract.VideoRequest{Prompt: "fixture", Resolution: "720p", MaxDurationS: 5, Runtime: map[string]any{"config": &config.ModelConfig{Endpoint: server.URL, APIKey: "fixture-key", Model: "agnes-video-2.5-flash"}}}
			mutate(&in)
			_, err := NewVideoClient().Submit(context.Background(), in)
			var app *dto.AppError
			require.ErrorAs(t, err, &app)
			require.Equal(t, "AI_PARAMETER_INVALID", app.Code)
		})
	}
	require.Zero(t, calls.Load())
}

func TestProviderErrorsPreserveStatusTraceAndNoAutomaticRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("X-Request-ID", "fixture-trace")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":{"code":"LIMIT","message":"private supplier body"}}`))
	}))
	defer server.Close()
	_, err := NewImageClient().Generate(context.Background(), contract.ImageRequest{Prompt: "fixture", Size: "1K", Runtime: map[string]any{"config": &config.ModelConfig{Endpoint: server.URL, APIKey: "fixture-key", Model: "agnes-image-2.5-flash"}}})
	var app *dto.AppError
	require.ErrorAs(t, err, &app)
	require.Equal(t, 429, app.HTTPCode)
	require.Equal(t, "LIMIT", app.Details["upstream_code"])
	require.Equal(t, "fixture-trace", app.Details["provider_trace_id"])
	require.NotContains(t, err.Error(), "private supplier body")
	require.Equal(t, 1, calls)
}

func TestRedirectDoesNotForwardCredentials(t *testing.T) {
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := NewImageClient().Generate(context.Background(), contract.ImageRequest{Prompt: "fixture", Runtime: map[string]any{"config": &config.ModelConfig{Endpoint: server.URL, APIKey: "fixture-key", Model: "agnes-image-2.5-flash"}}})
	require.Error(t, err)
	require.Zero(t, leaked.Load())
	require.NotContains(t, err.Error(), "fixture-key")
}

func TestMalformedOutputNeverAcceptsTaskIDAsVideoID(t *testing.T) {
	for _, body := range []string{`{"id":"task_only"}`, `{"video_id":"https://attacker.example/task"}`, `{}`, `invalid`} {
		t.Run(strings.ReplaceAll(body, "/", "_"), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			_, err := NewVideoClient().Submit(context.Background(), contract.VideoRequest{Prompt: "fixture", Runtime: map[string]any{"config": &config.ModelConfig{Endpoint: server.URL, APIKey: "fixture-key", Model: "agnes-video-2.5"}}})
			require.Error(t, err)
		})
	}
}
