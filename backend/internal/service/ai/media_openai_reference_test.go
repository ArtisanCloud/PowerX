package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	caprouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ai"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func referenceServiceKey(t *testing.T, f *mediaFixture) (context.Context, *cap.InvocationService, *gw.IntegrationGatewayAPIKeyPermission) {
	t.Helper()
	key := gw.IntegrationGatewayAPIKey{TenantUUID: mediaTestTenant, ProfileID: 1, Name: "reference-service-only", KeyPrefix: "fixture", KeyHash: "reference-key-hash", Status: "active"}
	require.NoError(t, f.db.Create(&key).Error)
	var imageGrant *gw.IntegrationGatewayAPIKeyPermission
	for _, spec := range []struct{ id, scope, action, resourceType, resource, endpoint string }{
		{MediaCatalogCapability, "_scope.ai.catalog.models.read", "read", "api", "ai-catalog-models", MediaCatalogEndpoint},
		{InvokeCapability(catalog.ImageGeneration), "_scope.ai.image.invoke", "invoke", "capability", "ai_image", MediaInvokeEndpoint},
		{MediaTaskReadCapability, "_scope.ai.media.tasks.read", "read", "capability", "ai_media_tasks", MediaTasksEndpoint},
		{cap.GrantStatusCapabilityID, "_scope.capabilities.grant_status.read", "read", "capability", "grant-status", "core://capabilities/grant-status"},
	} {
		require.NoError(t, f.db.Create(&cm.CapabilityRecord{CapabilityID: spec.id, PluginID: "core", PluginVersion: "fixture", Title: spec.id, CapabilitiesHash: spec.id, ProtocolHash: spec.id, Status: "published"}).Error)
		policy := cm.RoutingPolicy{Strategy: "weighted_round_robin"}
		require.NoError(t, f.db.Create(&policy).Error)
		registration := cm.CapabilityRegistration{CapabilityID: spec.id, TenantUUID: mediaTestTenant, ContractRef: "fixture", Status: "published", Version: 1, RoutingPolicyID: policy.UUID}
		require.NoError(t, f.db.Create(&registration).Error)
		require.NoError(t, f.db.Create(&cm.AdapterEndpoint{RegistrationID: registration.ID, CapabilityID: spec.id, TenantUUID: mediaTestTenant, AdapterID: spec.id + "-reference-fixture", TransportType: "core_internal", Endpoint: spec.endpoint, Weight: 1, TimeoutMS: 5000, IsActive: true, Labels: datatypes.JSON(`{"method":"INVOKE"}`)}).Error)
		meta, _ := json.Marshal(map[string]any{"capability_id": spec.id, "api_key_explicit": true, "api_key": map[string]any{"scope": spec.scope, "action": spec.action, "resource_type": spec.resourceType, "resource_pattern": spec.resource, "effect": "allow"}})
		require.NoError(t, f.db.Create(&iam.Permission{Module: "ai", Resource: spec.resource, Action: spec.action, Effect: "allow", Status: iam.PermissionStatusActive, Source: "platform_capability", AllowAPIKey: true, Meta: meta}).Error)
		grant := &gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: spec.scope, Action: spec.action, ResourceType: spec.resourceType, ResourcePattern: spec.resource, Effect: "allow"}
		require.NoError(t, f.db.Create(grant).Error)
		if spec.action == "invoke" {
			imageGrant = grant
		}
	}
	ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), mediaTestTenant), &reqctx.CoreXClaims{TenantUUID: mediaTestTenant, Platforms: []string{"api_key"}}), key.KeyHash)
	gateway := cap.NewInvocationService(cap.InvocationServiceOptions{Catalog: cap.NewRegistryService(cap.RegistryServiceOptions{DB: f.db}), Router: caprouter.NewService(caprouter.ServiceOptions{DB: f.db}), CoreInvoker: &MediaInvoker{Service: f.s}, ModelVerifier: cap.NewTenantModelKeyVerifier(f.db)})
	return ctx, gateway, imageGrant
}

func TestOpenAIReferenceServiceKeyTaskIdentityAndAssets(t *testing.T) {
	f := newMediaFixture(t)
	first := "data:image/png;base64," + base64.StdEncoding.EncodeToString(f.png)
	var otherPNG bytes.Buffer
	otherImage := image.NewRGBA(image.Rect(0, 0, 2, 2))
	otherImage.Set(0, 0, color.RGBA{R: 200, A: 255})
	require.NoError(t, png.Encode(&otherPNG, otherImage))
	second := "data:image/png;base64," + base64.StdEncoding.EncodeToString(otherPNG.Bytes())
	var calls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-image-2"}]}`))
			return
		}
		calls.Add(1)
		require.Equal(t, "/v1/images/edits", r.URL.Path)
		var body struct {
			Images []struct {
				URL string `json:"image_url"`
			} `json:"images"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Len(t, body.Images, 2)
		require.Equal(t, first, body.Images[0].URL)
		require.Equal(t, second, body.Images[1].URL)
		w.Header().Set("X-Request-Id", "reference-provider-success")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(f.png)}}})
	}))
	defer provider.Close()
	tenant := mediaTestTenant
	require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", &tenant, &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "fixture-provider-only", "base_url": provider.URL + "/v1"}}))
	require.NoError(t, f.db.Create(&am.AIModelProfile{Env: "dev", TenantUUID: &tenant, Modality: "image", Provider: "openai", Model: "gpt-image-2", Defaults: datatypes.JSONMap{"format": "png", "quality": "auto", "size": "1024x1024"}}).Error)
	ctx, gateway, grant := referenceServiceKey(t, f)
	statuses, err := cap.NewGrantStatusService(f.db).CheckCurrentCredential(ctx, []string{MediaCatalogCapability, InvokeCapability(catalog.ImageGeneration), MediaTaskReadCapability})
	require.NoError(t, err)
	for _, status := range statuses {
		require.Equal(t, cap.GrantStatusGranted, status.Status)
	}
	items, err := f.s.ai.ListMediaModels(ctx, "dev", tenant, "openai", "", catalog.ImageGeneration)
	require.NoError(t, err)
	var model MediaModelItem
	for _, item := range items {
		if item.Model == "gpt-image-2" {
			model = item
		}
	}
	require.True(t, model.Configured)
	require.True(t, model.Available)
	op := model.Operations[catalog.ImageGeneration]
	require.Equal(t, 16, op.MaxImages)
	require.Contains(t, op.InputTypes, "image_url")
	require.EqualValues(t, 5<<20, op.MaxImageBytes)
	require.EqualValues(t, 20<<20, op.MaxTotalImageBytes)
	require.Equal(t, "auto", op.Defaults["quality"])
	require.NotEmpty(t, op.ParameterSchema)
	input := MediaSubmitInput{ModelKey: "openai/gpt-image-2", Capability: catalog.ImageGeneration, IdempotencyKey: "two-reference-attempt", Params: map[string]any{"quality": "auto", "format": "png", "size": "1024x1024"}, Inputs: []ContentItem{{Type: "text", Content: "保持人物和当前装束"}, {Type: "image_url", URL: first}, {Type: "image_url", URL: second}}}
	invoke := func(in MediaSubmitInput) (*MediaTaskView, error) {
		result, err := gateway.Invoke(ctx, cap.InvocationInput{CapabilityID: InvokeCapability(catalog.ImageGeneration), TenantUUID: tenant, PreferredProtocol: "core_internal", Payload: map[string]any{"body": map[string]any{"operation": "invoke", "contract_version": 2, "model_key": in.ModelKey, "idempotency_key": in.IdempotencyKey, "inputs": in.Inputs, "params": in.Params}}})
		if err != nil {
			return nil, err
		}
		return result.Result["result"].(*MediaTaskView), nil
	}
	task, err := invoke(input)
	require.NoError(t, err)
	same, err := invoke(input)
	require.NoError(t, err)
	require.Equal(t, task.TaskUUID, same.TaskUUID)
	changed := input
	changed.Inputs = append([]ContentItem{}, input.Inputs...)
	changed.Inputs[1], changed.Inputs[2] = changed.Inputs[2], changed.Inputs[1]
	_, err = invoke(changed)
	requireMediaCode(t, err, "AI_IDEMPOTENCY_CONFLICT")
	changed.Inputs[1].URL = first
	_, err = invoke(changed)
	requireMediaCode(t, err, "AI_IDEMPOTENCY_CONFLICT")
	f.consume(t, f.s)
	completed, err := f.s.Get(ctx, task.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", completed.Status)
	require.Greater(t, completed.Revision, task.Revision)
	require.Equal(t, "reference-provider-success", completed.ProviderTraceID)
	require.Equal(t, 200, completed.ProviderStatus)
	require.NotEmpty(t, completed.TraceID)
	require.Len(t, completed.Assets, 1)
	asset := completed.Assets[0]
	require.Equal(t, "image/png", asset.MimeType)
	require.EqualValues(t, len(f.png), asset.SizeBytes)
	require.NotEmpty(t, asset.Checksum)
	_, object, err := f.media.OpenAssetResource(ctx, tenant, asset.MediaAssetUUID)
	require.NoError(t, err)
	raw, err := io.ReadAll(object.Body)
	object.Body.Close()
	require.NoError(t, err)
	require.Equal(t, f.png, raw)
	// 模拟重连/恢复：只读取原任务，同一键仍返回原成果，不再次调用供应商。
	restarted := NewMediaRuntime(f.db, f.media)
	restarted.Configure(f.queue, nil)
	restored, err := restarted.Get(ctx, task.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, completed, restored)
	same, err = invoke(input)
	require.NoError(t, err)
	require.Equal(t, completed.Revision, same.Revision)
	require.EqualValues(t, 1, calls.Load())
	// 仍保留 live grant：撤权后目录明确不可调用，新任务也被拒绝。
	require.NoError(t, f.db.Delete(grant).Error)
	items, err = f.s.ai.ListMediaModels(ctx, "dev", tenant, "openai", "", catalog.ImageGeneration)
	require.NoError(t, err)
	for _, item := range items {
		if item.Model == "gpt-image-2" {
			require.False(t, item.Available)
			require.Equal(t, "AI_FORBIDDEN", item.ReasonCode)
		}
	}
	input.IdempotencyKey = "after-revoke"
	_, err = invoke(input)
	require.Error(t, err)
	require.EqualValues(t, 1, calls.Load())
}

func referenceCatalogProvider(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/models") {
			t.Errorf("unexpected provider call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-image-2"}]}`))
	}))
	t.Cleanup(server.Close)
	return server.URL + "/v1"
}

func TestOpenAIReferenceValidationPrecedesTaskCreation(t *testing.T) {
	f := newMediaFixture(t)
	endpoint := referenceCatalogProvider(t)
	tenant := mediaTestTenant
	require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", &tenant, &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "fixture-only", "base_url": endpoint}}))
	require.NoError(t, f.db.Create(&am.AIModelProfile{Env: "dev", TenantUUID: &tenant, Modality: "image", Provider: "openai", Model: "gpt-image-2"}).Error)
	for _, test := range []struct {
		code   string
		inputs []ContentItem
		params map[string]any
	}{
		{"AI_INPUT_INVALID", []ContentItem{{Type: "image_url", URL: "data:image/png;base64,broken!"}}, nil},
		{"AI_INPUT_LIMIT_EXCEEDED", func() []ContentItem {
			result := []ContentItem{}
			for i := 0; i < 17; i++ {
				result = append(result, ContentItem{Type: "image_url", URL: "data:image/png;base64,AA=="})
			}
			return result
		}(), nil},
		{"AI_PARAMETER_INVALID", nil, map[string]any{"quality": "standard"}},
	} {
		_, err := f.s.Submit(f.ctx, MediaSubmitInput{ModelKey: "openai/gpt-image-2", Capability: catalog.ImageGeneration, IdempotencyKey: test.code, Inputs: append([]ContentItem{{Type: "text", Content: "fixture"}}, test.inputs...), Params: test.params})
		requireMediaCode(t, err, test.code)
	}
	var count int64
	require.NoError(t, f.db.Model(&m.MediaTask{}).Count(&count).Error)
	require.Zero(t, count)
	require.Zero(t, f.generations.Load())
}

func TestOpenAIReferenceCatalogHonorsSavedChannelLimit(t *testing.T) {
	f := newMediaFixture(t)
	endpoint := referenceCatalogProvider(t)
	tenant := mediaTestTenant
	require.NoError(t, f.db.Create(&am.AIModelProfile{Env: "dev", TenantUUID: &tenant, Modality: "image", Provider: "openai", Model: "gpt-image-2"}).Error)
	for _, limit := range []int{1, 0} {
		require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", &tenant, &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "fixture-only", "base_url": endpoint, "image_edit_max_images": limit}}))
		items, err := f.s.ai.ListMediaModels(f.ctx, "dev", tenant, "openai", "", catalog.ImageGeneration)
		require.NoError(t, err)
		for _, item := range items {
			if item.Model == "gpt-image-2" {
				op := item.Operations[catalog.ImageGeneration]
				require.True(t, item.Available)
				require.Equal(t, limit, op.MaxImages)
				if limit == 0 {
					require.Equal(t, []string{"text"}, op.InputTypes)
				}
			}
		}
		in := MediaSubmitInput{ModelKey: "openai/gpt-image-2", Capability: catalog.ImageGeneration, IdempotencyKey: "channel-cap", Inputs: []ContentItem{{Type: "text", Content: "fixture"}, {Type: "image_url", URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(f.png)}, {Type: "image_url", URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(f.png)}}}
		_, err = f.s.Submit(f.ctx, in)
		code := "AI_INPUT_LIMIT_EXCEEDED"
		if limit == 0 {
			code = "AI_INPUT_UNSUPPORTED"
		}
		requireMediaCode(t, err, code)
	}
}

func TestOpenAIReferenceLargePairFitsMediaRequestEnvelope(t *testing.T) {
	f := newMediaFixture(t)
	endpoint := referenceCatalogProvider(t)
	tenant := mediaTestTenant
	require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", &tenant, &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "fixture-only", "base_url": endpoint}}))
	require.NoError(t, f.db.Create(&am.AIModelProfile{Env: "dev", TenantUUID: &tenant, Modality: "image", Provider: "openai", Model: "gpt-image-2"}).Error)
	data := append(append([]byte{}, f.png...), bytes.Repeat([]byte{0}, (5<<20)-len(f.png))...)
	part := ContentItem{Type: "image_url", URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)}
	task, err := f.s.Submit(f.ctx, MediaSubmitInput{ModelKey: "openai/gpt-image-2", Capability: catalog.ImageGeneration, IdempotencyKey: "large-pair", Inputs: []ContentItem{{Type: "text", Content: "fixture"}, part, part}})
	require.NoError(t, err)
	require.Equal(t, "queued", task.Status)
	var row m.MediaTask
	require.NoError(t, f.db.First(&row, "uuid = ?", task.TaskUUID).Error)
	require.Greater(t, len(row.Request), 8<<20)
	require.True(t, strings.Contains(string(row.Request), "image_url"))
	require.Zero(t, f.generations.Load())
}

func TestOpenAIReferenceUnknownSubmissionNeverGeneratesAgain(t *testing.T) {
	f := newMediaFixture(t)
	var calls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-image-2"}]}`))
			return
		}
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		connection, _, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)
		connection.Close() // 请求已被供应商收到，响应丢失。
	}))
	defer provider.Close()
	tenant := mediaTestTenant
	require.NoError(t, f.s.ai.settings.SaveCredentialOnly(f.ctx, "dev", &tenant, &am.AIProviderCredential{Name: "dev-openai", Provider: "openai", Data: datatypes.JSONMap{"api_key": "fixture-only", "base_url": provider.URL + "/v1"}}))
	require.NoError(t, f.db.Create(&am.AIModelProfile{Env: "dev", TenantUUID: &tenant, Modality: "image", Provider: "openai", Model: "gpt-image-2"}).Error)
	in := MediaSubmitInput{ModelKey: "openai/gpt-image-2", Capability: catalog.ImageGeneration, IdempotencyKey: "lost-provider-response", Inputs: []ContentItem{{Type: "text", Content: "fixture"}, {Type: "image_url", URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(f.png)}}}
	task, err := f.s.Submit(f.ctx, in)
	require.NoError(t, err)
	f.consume(t, f.s)
	state, err := f.s.Get(f.ctx, task.TaskUUID)
	require.NoError(t, err)
	require.Equal(t, "failed", state.Status)
	require.Equal(t, "AI_SUBMISSION_OUTCOME_UNKNOWN", state.ErrorCode)
	same, err := f.s.Submit(f.ctx, in)
	require.NoError(t, err)
	require.Equal(t, task.TaskUUID, same.TaskUUID)
	require.EqualValues(t, 1, calls.Load())
}
