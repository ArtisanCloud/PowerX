package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	caprouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ai"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	mm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const mediaTestTenant = "11111111-1111-1111-1111-111111111111"

var mediaSQLiteOnce sync.Once

type mediaFixture struct {
	s             *MediaRuntime
	db            *gorm.DB
	queue         *taskqueue.StreamDriver
	ctx           context.Context
	manager       *mediamgr.MediaManager
	media         *mediasvc.MediaService
	mini          *miniredis.Miniredis
	provider      *httptest.Server
	generations   atomic.Int64
	queries       atomic.Int64
	deletes       atomic.Int64
	providerState atomic.Value
	video         []byte
	voice         []byte
	png           []byte
}

func newMediaFixture(t *testing.T) *mediaFixture {
	t.Helper()
	previous := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = previous })
	mediaSQLiteOnce.Do(func() {
		sql.Register("sqlite3_ai_media", &sqlite3.SQLiteDriver{ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			return conn.RegisterFunc("NOW", func() string { return time.Now().UTC().Format("2006-01-02 15:04:05.999999999-07:00") }, false)
		}})
	})
	var db *gorm.DB
	var err error
	pgDSN := os.Getenv("POWERX_AI_MEDIA_TEST_DSN")
	if pgDSN != "" {
		parsed, e := url.Parse(pgDSN)
		require.NoError(t, e)
		require.True(t, strings.HasPrefix(strings.TrimPrefix(parsed.Path, "/"), "powerx_ai_media_test_"), "disposable database required")
		core.PowerXSchema = "ai_media_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		db, err = gorm.Open(postgres.Open(pgDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	} else {
		db, err = gorm.Open(sqlite.Dialector{DriverName: "sqlite3_ai_media", DSN: t.TempDir() + "/media.db?_busy_timeout=5000"}, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	}
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	if pgDSN != "" {
		pool.SetMaxOpenConns(8)
		schema := core.PowerXSchema
		require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() { require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error) })
	}
	models := []any{&m.MediaTask{}, &ef.TaskHistory{}, &mm.MediaAsset{}, &mm.MediaAssetVariant{}, &cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &cm.RoutingPolicy{}, &cm.AdapterEndpoint{}, &cm.FallbackPlan{}, &setting.PluginInstanceConfig{}, &setting.TenantSetting{}, &agentmodel.AIModelProfile{}, &agentmodel.AIProviderCredential{}, &tenant.TenantKeyPair{}, &iam.APIKeyProfile{}, &iam.Permission{}, &iam.APIKeyProfilePermission{}, &gw.IntegrationGatewayAPIKey{}, &gw.IntegrationGatewayAPIKeyPermission{}}
	for _, model := range models {
		if pgDSN != "" {
			continue
		}
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(model))
		for _, field := range stmt.Schema.Fields {
			field.DefaultValue = strings.TrimSuffix(field.DefaultValue, "::jsonb")
			if field.DataType == "timestamp with time zone" {
				field.DataType = "datetime"
				field.TagSettings["TYPE"] = "datetime"
			}
			if strings.Contains(strings.ToLower(field.TagSettings["INDEX"]), "gin") {
				delete(field.TagSettings, "INDEX")
			}
		}
	}
	require.NoError(t, db.AutoMigrate(models...))
	oldKey := crypto.GetGlobalKeyB64()
	require.NoError(t, crypto.SetGlobalKeyB64(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))))
	t.Cleanup(func() { require.NoError(t, crypto.SetGlobalKeyB64(oldKey)) })
	reg := catalog.NewRegistry()
	require.NoError(t, reg.LoadByConfig(catalog.CatalogConfig{Dirs: []string{"../../../config/agents/providers.d"}, FailIfEmpty: true}, nil))
	oldReg := catalog.GlobalAIRegister
	catalog.GlobalAIRegister = reg
	t.Cleanup(func() { catalog.GlobalAIRegister = oldReg })
	store, err := local.New(local.Options{BasePath: t.TempDir()})
	require.NoError(t, err)
	manager := mediamgr.New("local")
	manager.RegisterDriver(store)
	media := mediasvc.NewMediaService(db, nil, manager, nil, time.Hour)
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	base, err := event_bus.NewRedisStreamTaskDriver(client, 2*time.Minute)
	require.NoError(t, err)
	queue := taskqueue.NewStreamDriverWithRetryDelay(db, base, time.Millisecond)
	f := &mediaFixture{db: db, queue: queue, manager: manager, media: media, mini: mini}
	f.s = NewMediaRuntime(db, media)
	f.s.Configure(queue, nil)
	f.s.providerDelay = time.Millisecond
	f.providerState.Store("succeeded")
	f.voice, err = os.ReadFile("testdata/voice.wav")
	require.NoError(t, err)
	f.video, err = os.ReadFile("testdata/clip.mp4")
	require.NoError(t, err)
	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	f.png = pngBytes.Bytes()
	f.provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/models"):
			if strings.Contains(r.URL.Path, "/api/v1/models") {
				_, _ = w.Write([]byte(`{"output":{"total":4,"models":[{"model":"qwen-image-2.0-pro"},{"model":"qwen-vl-max"},{"model":"qwen3-tts-flash"},{"model":"qwen-max-latest"}]}}`))
			} else {
				_, _ = w.Write([]byte(`{"data":[{"id":"gpt-6-luna"}]}`))
			}
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/contents/generations/tasks"):
			f.generations.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "native-private-task"})
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/contents/generations/tasks/"):
			f.queries.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "native-private-task", "status": f.providerState.Load(), "content": map[string]any{"video_url": f.provider.URL + "/clip.mp4"}})
		case r.Method == "DELETE":
			f.deletes.Add(1)
			f.providerState.Store("cancelled")
			fmt.Fprint(w, "{}")
		case r.URL.Path == "/clip.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write(f.video)
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.EqualValues(t, 0, body["temperature"])
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "reference analysis"}}}})
		case strings.HasSuffix(r.URL.Path, "/multimodal-generation/generation"):
			f.generations.Add(1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if strings.Contains(body["model"].(string), "tts") {
				_ = json.NewEncoder(w).Encode(map[string]any{"output": map[string]any{"audio": map[string]any{"url": f.provider.URL + "/voice.wav"}}})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"output": map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": []any{map[string]any{"image": f.provider.URL + "/image.png"}}}}}}})
			}
		case r.URL.Path == "/voice.wav":
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(f.voice)
		case r.URL.Path == "/image.png":
			_, _ = w.Write(f.png)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.provider.Close)
	f.s.fetch = f.provider.Client()
	claims := &reqctx.CoreXClaims{TenantUUID: mediaTestTenant, IsRoot: true, RegisteredClaims: jwt.RegisteredClaims{Subject: "root"}}
	f.ctx = reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTestTenant), claims)
	require.NoError(t, f.s.ai.settings.SetTenantCurrentAIEnv(f.ctx, mediaTestTenant, "dev"))
	for _, provider := range []string{"qwen-cn", "volcengine"} {
		require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", ptr(mediaTestTenant), &agentmodel.AIProviderCredential{Name: "dev-" + provider, Provider: provider, Data: datatypes.JSONMap{"api_key": "test-only", "base_url": f.provider.URL + "/compatible-mode/v1"}}))
	}
	for _, profile := range []agentmodel.AIModelProfile{{Modality: "image", Provider: "qwen-cn", Model: "qwen-image-2.0-pro"}, {Modality: "image", Provider: "qwen-cn", Model: "qwen-vl-max"}, {Modality: "audio_tts", Provider: "qwen-cn", Model: "qwen3-tts-flash"}, {Modality: "video", Provider: "volcengine", Model: "seedance:doubao-seedance-1-0-pro-250528"}} {
		profile.Env = "dev"
		profile.TenantUUID = ptr(mediaTestTenant)
		require.NoError(t, db.Create(&profile).Error)
	}
	return f
}
func ptr[T any](value T) *T { return &value }
func videoInput() MediaSubmitInput {
	return MediaSubmitInput{ModelKey: "volcengine/seedance:doubao-seedance-1-0-pro-250528", Capability: catalog.VideoGeneration, Inputs: []ContentItem{{Type: "text", Content: "test video"}}, IdempotencyKey: "video-1"}
}
func (f *mediaFixture) consume(t *testing.T, s *MediaRuntime) {
	t.Helper()
	time.Sleep(3 * time.Millisecond)
	deliveries, err := f.queue.Dequeue(context.Background(), mediaQueueTenant, mediaSubscriber, "tests", uuid.NewString(), 1, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	require.NoError(t, s.handleMediaDelivery(context.Background(), deliveries[0]))
}
func requireMediaCode(t *testing.T, err error, code string) {
	t.Helper()
	var app *dto.AppError
	require.ErrorAs(t, err, &app)
	require.Equal(t, code, app.Code)
}

func TestMediaTaskRestartIdempotencyManagedResultAndIsolation(t *testing.T) {
	f := newMediaFixture(t)
	in := videoInput()
	first, err := f.s.Submit(f.ctx, in)
	require.NoError(t, err)
	require.Equal(t, "queued", first.Status)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			same, e := f.s.Submit(f.ctx, in)
			require.NoError(t, e)
			if same != nil {
				require.Equal(t, first.TaskUUID, same.TaskUUID)
			}
		}()
	}
	wg.Wait()
	changed := in
	changed.Params = map[string]any{"resolution": "1080p"}
	_, err = f.s.Submit(f.ctx, changed)
	requireMediaCode(t, err, "AI_IDEMPOTENCY_CONFLICT")
	f.consume(t, f.s)
	require.EqualValues(t, 1, f.generations.Load())
	restarted := NewMediaRuntime(f.db, f.media)
	restarted.Configure(f.queue, nil)
	restarted.fetch = f.provider.Client()
	restarted.providerDelay = time.Millisecond
	for i := 0; i < 3; i++ {
		state, e := restarted.Get(f.ctx, first.TaskUUID)
		require.NoError(t, e)
		require.Equal(t, "running", state.Status)
	}
	require.Zero(t, f.queries.Load())
	f.consume(t, restarted)
	state, err := restarted.Get(f.ctx, first.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", state.Status)
	require.Len(t, state.Assets, 1)
	require.EqualValues(t, 1, f.generations.Load())
	asset, err := f.media.GetAsset(f.ctx, mediaTestTenant, state.Assets[0].MediaAssetUUID, false)
	require.NoError(t, err)
	require.Equal(t, "ready", asset.UploadState)
	_, object, err := f.media.OpenAssetResource(f.ctx, mediaTestTenant, asset.UUID)
	require.NoError(t, err)
	defer object.Body.Close()
	raw, err := io.ReadAll(object.Body)
	require.NoError(t, err)
	require.Equal(t, f.video, raw)
	raw, err = json.Marshal(state)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "native-private-task")
	require.NotContains(t, string(raw), f.provider.URL)
	other := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), uuid.NewString()), &reqctx.CoreXClaims{IsRoot: true, RegisteredClaims: jwt.RegisteredClaims{Subject: "root"}})
	_, err = restarted.Get(other, first.TaskUUID)
	requireMediaCode(t, err, "AI_TASK_NOT_FOUND")
}
func TestMediaCatalogRoutesByDeclaredCapabilityAndRejectsParameters(t *testing.T) {
	f := newMediaFixture(t)
	items, err := f.s.ai.ListMediaModels(f.ctx, "dev", mediaTestTenant, "qwen-cn", "", catalog.ImageGeneration)
	require.NoError(t, err)
	require.Len(t, items, 4)
	for _, item := range items {
		require.Equal(t, []string{catalog.ImageGeneration}, item.Capabilities)
		if item.Model == "qwen-image-2.0-pro" {
			require.True(t, item.Available)
		} else {
			require.False(t, item.Configured)
		}
	}
	empty, err := f.s.ai.ListMediaModels(f.ctx, "dev", mediaTestTenant, "qwen-cn", "", catalog.VideoGeneration)
	require.NoError(t, err)
	require.Empty(t, empty)
	_, err = f.s.Submit(f.ctx, MediaSubmitInput{ModelKey: "qwen-cn/qwen-vl-max", Capability: catalog.ImageGeneration, Inputs: videoInput().Inputs, IdempotencyKey: "wrong-modality"})
	requireMediaCode(t, err, "AI_MODEL_NOT_CONFIGURED")
	for _, params := range []map[string]any{{"temperature": 1}, {"n": 1.5}, {"n": "1"}, {"n": 0}, {"size": "invalid"}} {
		_, err = f.s.Submit(f.ctx, MediaSubmitInput{ModelKey: "qwen-cn/qwen-image-2.0-pro", Capability: catalog.ImageGeneration, Inputs: videoInput().Inputs, Params: params, IdempotencyKey: "illegal"})
		requireMediaCode(t, err, "AI_PARAMETER_INVALID")
	}
	analysis, err := f.s.ai.VLMInvoke(f.ctx, "dev", mediaTestTenant, "qwen-cn/qwen-vl-max", []ContentItem{{Type: "text", Content: "analyze"}, {Type: "image_url", URL: "https://example.com/reference.png"}}, map[string]any{"temperature": 0})
	require.NoError(t, err)
	require.Equal(t, "analysis", analysis["type"])
	require.Equal(t, "reference analysis", analysis["text"])
	imageTask, err := f.s.Submit(f.ctx, MediaSubmitInput{ModelKey: "qwen-cn/qwen-image-2.0-pro", Capability: catalog.ImageGeneration, Inputs: videoInput().Inputs, IdempotencyKey: "image-1"})
	require.NoError(t, err)
	f.consume(t, f.s)
	state, err := f.s.Get(f.ctx, imageTask.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", state.Status)
	require.Equal(t, "image", state.Assets[0].Type)
	audioTask, e := f.s.Submit(f.ctx, MediaSubmitInput{ModelKey: "qwen-cn/qwen3-tts-flash", Capability: catalog.SpeechSynthesis, Inputs: videoInput().Inputs, IdempotencyKey: "audio-1", Params: map[string]any{"voice": "Cherry"}})
	require.NoError(t, e)
	f.consume(t, f.s)
	audioState, e := f.s.Get(f.ctx, audioTask.TaskUUID)
	require.NoError(t, e)
	require.Equal(t, "succeeded", audioState.Status)
	require.Len(t, audioState.Assets, 1)
	require.Equal(t, "audio", audioState.Assets[0].Type)
	for _, output := range []struct {
		task *MediaTaskView
		data []byte
	}{{state, f.png}, {audioState, f.voice}} {
		_, object, readErr := f.media.OpenAssetResource(f.ctx, mediaTestTenant, output.task.Assets[0].MediaAssetUUID)
		require.NoError(t, readErr)
		actual, readErr := io.ReadAll(object.Body)
		require.NoError(t, object.Body.Close())
		require.NoError(t, readErr)
		require.Equal(t, output.data, actual)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(actual)), output.task.Assets[0].Checksum)
	}
	_, e = f.s.Submit(f.ctx, MediaSubmitInput{ModelKey: "qwen-cn/qwen3-tts-flash", Capability: catalog.SpeechSynthesis, Inputs: videoInput().Inputs, IdempotencyKey: "unsupported-speed", Params: map[string]any{"speed": 1.2}})
	requireMediaCode(t, e, "AI_PARAMETER_INVALID")

}
func TestMediaTaskCancelAndUncertainSubmissionNeverRegenerates(t *testing.T) {
	f := newMediaFixture(t)
	first, err := f.s.Submit(f.ctx, videoInput())
	require.NoError(t, err)
	cancelled, err := f.s.Cancel(f.ctx, first.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, "confirmed", cancelled.CancelStatus)
	f.consume(t, f.s)
	require.Zero(t, f.generations.Load())
	in := videoInput()
	in.IdempotencyKey = "uncertain"
	second, err := f.s.Submit(f.ctx, in)
	require.NoError(t, err)
	require.NoError(t, f.s.repo.Update(f.ctx, nil, mediaTestTenant, second.TaskUUID, func(row *m.MediaTask) error { row.Phase = "submitting"; row.Status = "running"; return nil }))
	f.consume(t, f.s)
	state, err := f.s.Get(f.ctx, second.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, "failed", state.Status)
	require.Equal(t, "AI_SUBMISSION_OUTCOME_UNKNOWN", state.ErrorCode)
	require.Zero(t, f.generations.Load())
}

func TestMediaTypedSTSAndAPIKeyLiveGrants(t *testing.T) {
	f := newMediaFixture(t)
	ids := []string{MediaCatalogCapability, MediaTaskReadCapability, MediaTaskCancelCapability, InvokeCapability(catalog.ImageGeneration), InvokeCapability(catalog.VideoGeneration), cap.GrantStatusCapabilityID}
	for _, id := range ids {
		require.NoError(t, f.db.Create(&cm.CapabilityRecord{CapabilityID: id, PluginID: "core", PluginVersion: "test", Title: id, CapabilitiesHash: id, ProtocolHash: id, Status: "published"}).Error)
		policy := cm.RoutingPolicy{Strategy: "weighted_round_robin"}
		require.NoError(t, f.db.Create(&policy).Error)
		registration := cm.CapabilityRegistration{CapabilityID: id, TenantUUID: mediaTestTenant, ContractRef: "test", Status: "published", Version: 1, RoutingPolicyID: policy.UUID}
		require.NoError(t, f.db.Create(&registration).Error)
		endpoint := MediaInvokeEndpoint
		if id == MediaCatalogCapability {
			endpoint = MediaCatalogEndpoint
		}
		if id == MediaTaskReadCapability || id == MediaTaskCancelCapability {
			endpoint = MediaTasksEndpoint
		}
		require.NoError(t, f.db.Create(&cm.AdapterEndpoint{RegistrationID: registration.ID, CapabilityID: id, TenantUUID: mediaTestTenant, AdapterID: id + "-core", TransportType: "core_internal", Endpoint: endpoint, Weight: 1, TimeoutMS: 5000, IsActive: true, Labels: datatypes.JSON(`{"method":"INVOKE"}`)}).Error)
	}
	allowed, _ := json.Marshal(map[string]any{"client_id": "media-client", "allowed_capabilities": ids})
	credential := setting.PluginInstanceConfig{TenantUUID: mediaTestTenant, PluginID: "media-plugin", Key: "auth.credentials", Enabled: true, ValueJSON: allowed}
	require.NoError(t, f.db.Create(&credential).Error)
	sts := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTestTenant), &reqctx.CoreXClaims{TenantUUID: mediaTestTenant, PluginID: "media-plugin", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:media-client", Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	invoker := &MediaInvoker{Service: f.s}
	request := cap.CoreCapabilityInvokeInput{CapabilityID: MediaCatalogCapability, TenantUUID: mediaTestTenant, Method: "INVOKE", Endpoint: MediaCatalogEndpoint, Body: map[string]any{"operation": "list_models", "provider": "qwen-cn", "capability": catalog.ImageGeneration}}
	_, err := invoker.InvokeCoreCapability(sts, request)
	require.NoError(t, err)
	bad := request
	bad.Body = map[string]any{"operation": "list_models", "endpoint": "/api/v1/admin/root"}
	_, err = invoker.InvokeCoreCapability(sts, bad)
	requireMediaCode(t, err, "AI_OPERATION_INVALID")
	key := gw.IntegrationGatewayAPIKey{TenantUUID: mediaTestTenant, ProfileID: 1, Name: "test", KeyPrefix: "test", KeyHash: "media-key-hash", Status: "active"}
	require.NoError(t, f.db.Create(&key).Error)
	meta, _ := json.Marshal(map[string]any{"capability_id": MediaCatalogCapability, "api_key_explicit": true, "api_key": map[string]any{"scope": "_scope.ai.catalog.models.read", "action": "read", "resource_type": "api", "resource_pattern": "ai-catalog-models", "effect": "allow"}})
	require.NoError(t, f.db.Create(&iam.Permission{Module: "ai", Resource: "catalog_models", Action: "read", Effect: "allow", Status: iam.PermissionStatusActive, Source: "platform_capability", AllowAPIKey: true, Meta: meta}).Error)
	grant := gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.ai.catalog.models.read", Action: "read", ResourceType: "api", ResourcePattern: "ai-catalog-models", Effect: "allow"}
	require.NoError(t, f.db.Create(&grant).Error)
	apiCtx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTestTenant), &reqctx.CoreXClaims{TenantUUID: mediaTestTenant, Platforms: []string{"api_key"}}), key.KeyHash)
	_, err = invoker.InvokeCoreCapability(apiCtx, request)
	require.NoError(t, err)

	grantTyped := func(id, scope, action, resource string) {
		metadata, _ := json.Marshal(map[string]any{"capability_id": id, "api_key_explicit": true, "api_key": map[string]any{"scope": scope, "action": action, "resource_type": "capability", "resource_pattern": resource, "effect": "allow"}})
		require.NoError(t, f.db.Create(&iam.Permission{Module: "ai", Resource: strings.ReplaceAll(id, ".", "_"), Action: action, Effect: "allow", Status: iam.PermissionStatusActive, Source: "platform_capability", AllowAPIKey: true, Meta: metadata}).Error)
		require.NoError(t, f.db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: scope, Action: action, ResourceType: "capability", ResourcePattern: resource, Effect: "allow"}).Error)
	}
	grantTyped(InvokeCapability(catalog.VideoGeneration), "_scope.ai.video.invoke", "invoke", "ai_video")
	grantTyped(MediaTaskReadCapability, "_scope.ai.media.tasks.read", "read", "ai_media_tasks")
	grantTyped(MediaTaskCancelCapability, "_scope.ai.media.tasks.cancel", "cancel", "ai_media_tasks")
	grantTyped(cap.GrantStatusCapabilityID, "_scope.capabilities.grant_status.read", "read", "grant-status")
	statuses, e := cap.NewGrantStatusService(f.db).CheckCurrentCredential(apiCtx, []string{MediaCatalogCapability, InvokeCapability(catalog.VideoGeneration), MediaTaskReadCapability})
	require.NoError(t, e)
	for _, status := range statuses {
		require.Equal(t, cap.GrantStatusGranted, status.Status)
	}
	statuses, e = cap.NewGrantStatusService(f.db).CheckCurrentCredential(sts, []string{MediaCatalogCapability, InvokeCapability(catalog.VideoGeneration), MediaTaskReadCapability})
	require.NoError(t, e)
	for _, status := range statuses {
		require.Equal(t, cap.GrantStatusGranted, status.Status)
	}
	invoke := cap.CoreCapabilityInvokeInput{CapabilityID: InvokeCapability(catalog.VideoGeneration), TenantUUID: mediaTestTenant, Method: "INVOKE", Endpoint: MediaInvokeEndpoint, Body: map[string]any{"operation": "invoke", "contract_version": 2, "model_key": videoInput().ModelKey, "inputs": []any{map[string]any{"type": "text", "content": "API Key video"}}, "idempotency_key": "api-key-attempt"}}
	gateway := cap.NewInvocationService(cap.InvocationServiceOptions{Catalog: cap.NewRegistryService(cap.RegistryServiceOptions{DB: f.db}), Router: caprouter.NewService(caprouter.ServiceOptions{DB: f.db}), CoreInvoker: invoker, ModelVerifier: cap.NewTenantModelKeyVerifier(f.db)})
	invoked, e := gateway.Invoke(apiCtx, cap.InvocationInput{CapabilityID: invoke.CapabilityID, TenantUUID: mediaTestTenant, PreferredProtocol: "core_internal", Payload: map[string]any{"body": invoke.Body}})
	require.NoError(t, e)
	task := invoked.Result["result"].(*MediaTaskView)
	f.consume(t, f.s)
	f.consume(t, f.s)
	result, e := f.s.Get(apiCtx, task.TaskUUID)
	require.NoError(t, e)
	require.Equal(t, "succeeded", result.Status)
	queried, e := gateway.Invoke(apiCtx, cap.InvocationInput{CapabilityID: MediaTaskReadCapability, TenantUUID: mediaTestTenant, PreferredProtocol: "core_internal", Payload: map[string]any{"body": map[string]any{"operation": "get", "task_uuid": task.TaskUUID}}})
	require.NoError(t, e)
	require.Equal(t, "succeeded", queried.Result["result"].(*MediaTaskView).Status)
	_, e = gateway.Invoke(sts, cap.InvocationInput{CapabilityID: MediaCatalogCapability, TenantUUID: mediaTestTenant, PreferredProtocol: "core_internal", Payload: map[string]any{"body": request.Body}})
	require.NoError(t, e)
	_, e = f.s.Get(sts, task.TaskUUID)
	requireMediaCode(t, e, "AI_TASK_NOT_FOUND")
	for _, caller := range []context.Context{apiCtx, sts} {
		queued, invokeErr := gateway.Invoke(caller, cap.InvocationInput{CapabilityID: invoke.CapabilityID, TenantUUID: mediaTestTenant, PreferredProtocol: "core_internal", Payload: map[string]any{"body": map[string]any{"operation": "invoke", "contract_version": 2, "model_key": videoInput().ModelKey, "inputs": []any{map[string]any{"type": "text", "content": "queued cancellation"}}, "idempotency_key": "cancelled-attempt"}}})
		require.NoError(t, invokeErr)
		owned := queued.Result["result"].(*MediaTaskView)
		_, readErr := gateway.Invoke(caller, cap.InvocationInput{CapabilityID: MediaTaskReadCapability, TenantUUID: mediaTestTenant, PreferredProtocol: "core_internal", Payload: map[string]any{"body": map[string]any{"operation": "get", "task_uuid": owned.TaskUUID}}})
		require.NoError(t, readErr)
		cancelled, cancelErr := gateway.Invoke(caller, cap.InvocationInput{CapabilityID: MediaTaskCancelCapability, TenantUUID: mediaTestTenant, PreferredProtocol: "core_internal", Payload: map[string]any{"body": map[string]any{"operation": "cancel", "task_uuid": owned.TaskUUID}}})
		require.NoError(t, cancelErr)
		require.Equal(t, "cancelled", cancelled.Result["result"].(*MediaTaskView).Status)
		require.Equal(t, "confirmed", cancelled.Result["result"].(*MediaTaskView).CancelStatus)
	}
	require.EqualValues(t, 1, f.generations.Load(), "queued cancellation must not generate")
	require.NoError(t, f.db.Delete(&grant).Error)
	_, err = invoker.InvokeCoreCapability(apiCtx, request)
	var denied *cap.DirectGrantError
	require.ErrorAs(t, err, &denied)
	require.Equal(t, 403, denied.Status)
	require.NoError(t, f.db.Model(&credential).Update("value_json", datatypes.JSON(`{"client_id":"media-client","allowed_capabilities":[]}`)).Error)
	_, err = invoker.InvokeCoreCapability(sts, request)
	require.ErrorAs(t, err, &denied)
	require.Equal(t, 403, denied.Status)
}

func TestMediaWaitingProviderDoesNotPublishUnchangedStatusAndCancelDoesNotDeleteRunning(t *testing.T) {
	f := newMediaFixture(t)
	var events atomic.Int64
	f.s.changed = func(context.Context, string, string) { events.Add(1) }
	f.providerState.Store("running")
	task, err := f.s.Submit(f.ctx, videoInput())
	require.NoError(t, err)
	f.consume(t, f.s)
	before := events.Load()
	f.consume(t, f.s)
	require.Equal(t, before, events.Load(), "supplier status checks must not publish unchanged snapshots")
	_, err = f.s.Cancel(f.ctx, task.TaskUUID)
	require.NoError(t, err)
	f.consume(t, f.s)
	state, err := f.s.Get(f.ctx, task.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, "unsupported", state.CancelStatus)
	require.Zero(t, f.deletes.Load())
	f.providerState.Store("succeeded")
	f.consume(t, f.s)
	state, err = f.s.Get(f.ctx, task.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", state.Status)
	require.EqualValues(t, 1, f.generations.Load())
}

func TestMediaRejectsProviderOutputOfWrongType(t *testing.T) {
	f := newMediaFixture(t)
	f.video = f.png
	task, err := f.s.Submit(f.ctx, videoInput())
	require.NoError(t, err)
	f.consume(t, f.s)
	f.consume(t, f.s)
	result, err := f.s.Get(f.ctx, task.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, "failed", result.Status)
	require.Equal(t, "AI_OUTPUT_TYPE_MISMATCH", result.ErrorCode)
	require.Empty(t, result.Assets)
	require.EqualValues(t, 1, f.generations.Load())
}
