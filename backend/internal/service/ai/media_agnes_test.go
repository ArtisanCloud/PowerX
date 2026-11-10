package ai

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ai"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func configureAgnesFixture(t *testing.T, f *mediaFixture, endpoint string) {
	t.Helper()
	require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", ptr(mediaTestTenant), &am.AIProviderCredential{Name: "dev-agnes", Provider: "agnes", Data: datatypes.JSONMap{"api_key": "fixture-key", "base_url": endpoint + "/v1"}}))
	for _, spec := range []struct{ modality, model string }{{"image", "agnes-image-2.5-flash"}, {"image", "agnes-image-2.1-flash"}, {"video", "agnes-video-2.5"}, {"video", "agnes-video-2.5-flash"}} {
		require.NoError(t, f.db.Create(&am.AIModelProfile{Env: "dev", TenantUUID: ptr(mediaTestTenant), Modality: spec.modality, Provider: "agnes", Model: spec.model, Defaults: datatypes.JSONMap{}}).Error)
	}
}

func TestAgnesManagedImageAndAsyncVideoRestart(t *testing.T) {
	f := newMediaFixture(t)
	var imageCalls, videoCalls, queryCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/v1/images/generations":
			imageCalls.Add(1)
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, "4:3", payload["ratio"])
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(f.png)}}})
		case "/v1/videos":
			videoCalls.Add(1)
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, "9:16", payload["aspect_ratio"])
			require.Equal(t, "4", payload["seconds"])
			require.Equal(t, "reference", payload["mode"])
			_, _ = w.Write([]byte(`{"id":"task_wrong","video_id":"video_private"}`))
		case "/agnesapi":
			queryCalls.Add(1)
			require.Equal(t, "video_private", r.URL.Query().Get("video_id"))
			require.Equal(t, "agnes-video-2.5-flash", r.URL.Query().Get("model_name"))
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "internal_status": "pending", "url": f.provider.URL + "/clip.mp4"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	configureAgnesFixture(t, f, provider.URL)
	for _, ability := range []string{catalog.ImageGeneration, catalog.VideoGeneration} {
		models, err := f.s.ai.ListMediaModels(f.ctx, "dev", mediaTestTenant, "agnes", "", ability)
		require.NoError(t, err)
		require.Len(t, models, 2)
		for _, model := range models {
			require.True(t, model.Available)
			require.Equal(t, []string{ability}, model.Capabilities)
		}
	}
	vision, err := f.s.ai.ListMediaModels(f.ctx, "dev", mediaTestTenant, "agnes", "", catalog.VisionUnderstanding)
	require.NoError(t, err)
	require.Empty(t, vision)
	imageTask, err := f.s.Submit(f.ctx, MediaSubmitInput{ModelKey: "agnes/agnes-image-2.5-flash", Capability: catalog.ImageGeneration, Inputs: videoInput().Inputs, Params: map[string]any{"size": "1K", "ratio": "4:3"}, IdempotencyKey: "agnes-image"})
	require.NoError(t, err)
	f.consume(t, f.s)
	imageResult, err := f.s.Get(f.ctx, imageTask.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", imageResult.Status)
	require.Len(t, imageResult.Assets, 1)
	_, resource, err := f.media.OpenAssetResource(f.ctx, mediaTestTenant, imageResult.Assets[0].MediaAssetUUID)
	require.NoError(t, err)
	data, err := io.ReadAll(resource.Body)
	require.NoError(t, err)
	require.NoError(t, resource.Body.Close())
	require.Equal(t, f.png, data)
	in := MediaSubmitInput{ModelKey: "agnes/agnes-video-2.5-flash", Capability: catalog.VideoGeneration, Inputs: append(videoInput().Inputs, ContentItem{Type: "image_url", URL: "https://example.com/reference.png"}), Params: map[string]any{"mode": "reference", "aspect_ratio": "9:16", "max_duration_sec": 4}, IdempotencyKey: "agnes-video"}
	task, err := f.s.Submit(f.ctx, in)
	require.NoError(t, err)
	f.consume(t, f.s)
	restarted := NewMediaRuntime(f.db, f.media)
	restarted.Configure(f.queue, nil)
	restarted.fetch = f.provider.Client()
	restarted.providerDelay = time.Millisecond
	repeated, err := restarted.Submit(f.ctx, in)
	require.NoError(t, err)
	require.Equal(t, task.TaskUUID, repeated.TaskUUID)
	for i := 0; i < 3; i++ {
		snapshot, e := restarted.Get(f.ctx, task.TaskUUID)
		require.NoError(t, e)
		require.Equal(t, "running", snapshot.Status)
	}
	require.Zero(t, queryCalls.Load())
	f.consume(t, restarted)
	result, err := restarted.Get(f.ctx, task.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", result.Status)
	require.Len(t, result.Assets, 1)
	require.Equal(t, "video", result.Assets[0].Type)
	_, resource, err = f.media.OpenAssetResource(f.ctx, mediaTestTenant, result.Assets[0].MediaAssetUUID)
	require.NoError(t, err)
	data, err = io.ReadAll(resource.Body)
	require.NoError(t, err)
	require.NoError(t, resource.Body.Close())
	require.Equal(t, f.video, data)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "video_private")
	require.NotContains(t, string(raw), provider.URL)
	require.NotContains(t, string(raw), f.provider.URL)
	require.EqualValues(t, 1, imageCalls.Load())
	require.EqualValues(t, 1, videoCalls.Load())
	require.EqualValues(t, 1, queryCalls.Load())
}

func TestAgnesInvalidParametersNeverCreateCoreTasks(t *testing.T) {
	f := newMediaFixture(t)
	configureAgnesFixture(t, f, f.provider.URL)
	for _, params := range []map[string]any{{"resolution": "1080p"}, {"fps": 24}, {"mode": "reference"}, {"mode": "keyframe"}, {"max_duration_sec": 3}, {"aspect_ratio": "auto"}} {
		_, err := f.s.Submit(f.ctx, MediaSubmitInput{ModelKey: "agnes/agnes-video-2.5-flash", Capability: catalog.VideoGeneration, Inputs: videoInput().Inputs, Params: params, IdempotencyKey: "invalid"})
		requireMediaCode(t, err, "AI_PARAMETER_INVALID")
	}
	_, err := f.s.Submit(f.ctx, MediaSubmitInput{ModelKey: "agnes/agnes-image-2.5-flash", Capability: catalog.ImageGeneration, Inputs: videoInput().Inputs, Params: map[string]any{"temperature": 1}, IdempotencyKey: "invalid-image"})
	requireMediaCode(t, err, "AI_PARAMETER_INVALID")
	_, err = f.s.Submit(f.ctx, MediaSubmitInput{ModelKey: "agnes/agnes-video-2.5", Capability: catalog.VideoGeneration, Inputs: videoInput().Inputs, Params: map[string]any{"resolution": "1K"}, IdempotencyKey: "invalid-fixed-square"})
	requireMediaCode(t, err, "AI_PARAMETER_INVALID")
	var count int64
	require.NoError(t, f.db.Model(&m.MediaTask{}).Count(&count).Error)
	require.Zero(t, count)
	require.Zero(t, f.generations.Load())
}
