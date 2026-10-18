package media

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	permissions "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	svc "github.com/ArtisanCloud/PowerX/internal/service/media"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	mm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type derivativeHostFixture struct {
	db                     *gorm.DB
	svc                    *svc.MediaService
	manager                *mediamgr.MediaManager
	client                 *redis.Client
	engine                 *gin.Engine
	server                 *httptest.Server
	tenant, prefix, plugin string
	credential             *setting.PluginInstanceConfig
	key                    gm.IntegrationGatewayAPIKey
	requests               atomic.Int32
}

func newDerivativeHostFixture(t *testing.T, prefix string) *derivativeHostFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previous := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/media.db?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	models := []any{&mm.MediaAsset{}, &mm.MediaAssetVariant{}, &ef.TaskHistory{}, &cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &setting.PluginInstanceConfig{}, &im.APIKeyProfile{}, &im.Permission{}, &im.APIKeyProfilePermission{}, &gm.IntegrationGatewayAPIKey{}, &gm.IntegrationGatewayAPIKeyPermission{}}
	for _, model := range models {
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
	store, err := local.New(local.Options{BasePath: t.TempDir()})
	require.NoError(t, err)
	manager := mediamgr.New("local")
	manager.RegisterDriver(store)
	service := svc.NewMediaService(db, nil, manager, nil, time.Hour)
	service.SetPublicResourceTokenSecret("isolated-derivative-host-test-secret")
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	queue, err := event_bus.NewRedisStreamTaskDriver(client, 2*time.Minute)
	require.NoError(t, err)
	service.ConfigureDerivativeQueue(taskqueue.NewStreamDriver(db, queue))
	service.SetDerivativeEvents(func(ctx context.Context, tenant, asset string) {
		_ = client.Publish(ctx, svc.ImageDerivativeEventChannel(tenant, asset), asset).Err()
	})
	f := &derivativeHostFixture{db: db, svc: service, manager: manager, client: client, tenant: uuid.NewString(), plugin: "media.host.test", prefix: prefix}
	ids := []string{svc.MediaAssetsReadCapabilityID, svc.MediaAssetsManageCapabilityID, svc.MediaAssetsTransferCapabilityID, svc.MediaAssetsVariantsCapabilityID, svc.ImageDerivativesReadCapability}
	for _, id := range ids {
		require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: id, PluginID: "core", PluginVersion: "1", Status: "published"}).Error)
		require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: id, TenantUUID: f.tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	}
	f.credential = &setting.PluginInstanceConfig{TenantUUID: f.tenant, PluginID: f.plugin, Key: "auth.credentials", Enabled: true}
	raw, err := json.Marshal(map[string]any{"client_id": f.plugin, "allowed_capabilities": ids[:4]})
	require.NoError(t, err)
	f.credential.ValueJSON = datatypes.JSON(raw)
	require.NoError(t, db.Create(f.credential).Error)
	f.key = gm.IntegrationGatewayAPIKey{TenantUUID: f.tenant, ProfileID: 1, Name: "test", KeyPrefix: "test", KeyHash: "isolated-media-host-key-hash", Status: "active"}
	require.NoError(t, db.Create(&f.key).Error)
	dir, err := filepath.Abs("../../../../../config/platform_capabilities")
	require.NoError(t, err)
	t.Setenv("PLATFORM_CAPABILITIES_DIR", dir)
	rows, err := permissions.BuildPlatformCapabilityPermissions()
	require.NoError(t, err)
	for _, row := range rows {
		var metadata map[string]any
		require.NoError(t, json.Unmarshal(row.Meta, &metadata))
		if metadata["capability_id"] == svc.ImageDerivativesReadCapability {
			require.NoError(t, db.Create(&row).Error)
		}
	}
	f.engine = gin.New()
	// Inject authenticated identities only; live grants, publication, tenant
	// registration, HTTP, storage and the official TaskBus driver are real.
	f.engine.Use(func(c *gin.Context) {
		f.requests.Add(1)
		tenant := f.tenant
		if override := c.GetHeader("X-Test-Tenant"); override != "" {
			tenant = override
		}
		var claims *reqctx.CoreXClaims
		switch c.GetHeader("X-Test-Actor") {
		case "sts":
			claims = &reqctx.CoreXClaims{TenantUUID: tenant, PluginID: f.plugin, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:" + f.plugin, Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
		case "key":
			claims = &reqctx.CoreXClaims{TenantUUID: tenant, Platforms: []string{"api_key"}}
		case "admin":
			claims = &reqctx.CoreXClaims{TenantUUID: tenant, MemberID: 1}
		}
		if claims != nil {
			ctx := reqctx.WithClaims(reqctx.WithTenantUUID(c.Request.Context(), tenant), claims)
			if c.GetHeader("X-Test-Actor") == "key" {
				ctx = reqctx.WithAuthenticatedAPIKeyHash(ctx, f.key.KeyHash)
				c.Set("auth_api_key_hash", f.key.KeyHash)
			}
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	})
	deps := &shared.Deps{DB: db, MediaSvc: service, EventFabric: &shared.EventFabricDeps{RedisClient: client}}
	registerHostContract(f.engine.Group(prefix), deps)
	RegisterPublicResource(f.engine, deps)
	f.server = httptest.NewServer(f.engine)
	t.Cleanup(f.server.Close)
	return f
}

func (f *derivativeHostFixture) call(t *testing.T, method, path, body, actor string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("X-Test-Actor", actor)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, r)
	return w
}

func hostData[T any](t *testing.T, w *httptest.ResponseRecorder, code int) T {
	t.Helper()
	require.Equal(t, code, w.Code, w.Body.String())
	var out struct {
		Data T `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out.Data
}

func (f *derivativeHostFixture) grantEvents(t *testing.T) {
	t.Helper()
	ids := []string{svc.MediaAssetsReadCapabilityID, svc.MediaAssetsManageCapabilityID, svc.MediaAssetsTransferCapabilityID, svc.MediaAssetsVariantsCapabilityID, svc.ImageDerivativesReadCapability}
	raw, err := json.Marshal(map[string]any{"client_id": f.plugin, "allowed_capabilities": ids})
	require.NoError(t, err)
	require.NoError(t, f.db.Model(f.credential).Update("value_json", datatypes.JSON(raw)).Error)
	require.NoError(t, f.db.Create(&gm.IntegrationGatewayAPIKeyPermission{APIKeyUUID: f.key.UUID, Scope: "_scope.media.image_derivatives.read", Action: "read", ResourceType: "capability", ResourcePattern: "media_image_derivatives", Effect: "allow"}).Error)
}

func (f *derivativeHostFixture) stream(t *testing.T, asset, actor string) (*http.Response, *bufio.Scanner) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	r, err := http.NewRequestWithContext(ctx, "GET", f.server.URL+f.prefix+"/tenant/media/assets/"+asset+"/derivatives/events", nil)
	require.NoError(t, err)
	r.Header.Set("X-Test-Actor", actor)
	resp, err := http.DefaultClient.Do(r)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, 200, resp.StatusCode)
	return resp, bufio.NewScanner(resp.Body)
}

func nextMediaEvent(t *testing.T, scanner *bufio.Scanner) (string, svc.ImageDerivatives) {
	t.Helper()
	var name string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			name = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			var state svc.ImageDerivatives
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &state))
			return name, state
		}
	}
	t.Fatalf("media stream ended: %v", scanner.Err())
	return "", svc.ImageDerivatives{}
}

func TestHostUploadAutomaticallyGeneratesAndRepairsPreviews(t *testing.T) {
	for _, prefix := range []string{"/api/v1", "/custom/v2"} {
		t.Run(prefix, func(t *testing.T) {
			f := newDerivativeHostFixture(t, prefix)
			base := prefix + "/tenant/media/assets"
			var raw bytes.Buffer
			require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 24, 48))))
			sum := sha256.Sum256(raw.Bytes())
			checksum := hex.EncodeToString(sum[:])
			body, _ := json.Marshal(map[string]any{"name": "image.png", "mime_type": "image/png", "size_bytes": raw.Len(), "checksum": checksum})
			created := hostData[hostAssetResponse](t, f.call(t, "POST", base, string(body), "sts"), 201)
			require.Equal(t, "pending_upload", created.Status)
			assetPath := base + "/" + created.AssetUUID
			ticket := hostData[map[string]any](t, f.call(t, "POST", assetPath+"/presign-upload", "{}", "sts"), 200)
			upload := httptest.NewRequest("PUT", ticket["url"].(string), bytes.NewReader(raw.Bytes()))
			upload.Header.Set("Content-Length", strconv.FormatInt(int64(raw.Len()), 10))
			upload.Header.Set("Content-Type", "image/png")
			for k, v := range ticket["headers"].(map[string]any) {
				upload.Header.Set(k, v.(string))
			}
			w := httptest.NewRecorder()
			f.engine.ServeHTTP(w, upload)
			require.Equal(t, 204, w.Code, w.Body.String())
			completed := hostData[hostAssetResponse](t, f.call(t, "POST", assetPath+"/complete-upload", `{"checksum":"`+checksum+`"}`, "sts"), 200)
			require.Equal(t, "ready", completed.Status)
			require.Equal(t, "queued", completed.Derivatives.Status)
			// Neither read nor ensure derivative grant is present during upload.
			eventPath := assetPath + "/derivatives/events"
			require.Equal(t, 403, f.call(t, "GET", eventPath, "", "sts").Code)
			require.Equal(t, 403, f.call(t, "GET", eventPath, "", "key").Code)
			f.grantEvents(t)
			resp, scanner := f.stream(t, created.AssetUUID, "sts")
			name, state := nextMediaEvent(t, scanner)
			require.Equal(t, "status", name)
			require.Equal(t, "queued", state.Status)
			workerCtx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- f.svc.RunImageDerivativeWorker(workerCtx) }()
			defer func() {
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("worker failed to stop")
				}
			}()
			requests := f.requests.Load()
			for state.Status != "ready" {
				name, state = nextMediaEvent(t, scanner)
				require.Equal(t, "status", name)
			}
			require.Equal(t, requests, f.requests.Load(), "completion uses events, without status polling")
			_ = resp.Body.Close()
			view := hostData[hostAssetResponse](t, f.call(t, "GET", assetPath, "", "sts"), 200)
			require.Equal(t, "ready", view.Status)
			require.Equal(t, "ready", view.Derivatives.Status)
			for _, item := range view.Derivatives.Items {
				require.NotEmpty(t, item.VariantUUID)
				variant := hostData[map[string]any](t, f.call(t, "GET", base+"/variants/"+item.VariantUUID, "", "sts"), 200)
				require.Equal(t, "image/png", variant["mime_type"])
				require.Greater(t, variant["size_bytes"].(float64), float64(0))
			}
			// Existing API repairs a lost object, preserving the variant UUID.
			var preview mm.MediaAssetVariant
			require.NoError(t, f.db.Where("asset_uuid=? AND variant=?", created.AssetUUID, "preview").First(&preview).Error)
			require.NoError(t, f.manager.Delete(context.Background(), preview.Driver, driver.DeleteObjectInput{Bucket: preview.Bucket, ObjectKey: preview.StorageKey}))
			resp, scanner = f.stream(t, created.AssetUUID, "sts")
			_, _ = nextMediaEvent(t, scanner)
			reused := hostData[hostAssetResponse](t, f.call(t, "POST", base, string(body), "sts"), 201)
			require.Equal(t, created.AssetUUID, reused.AssetUUID)
			for {
				name, state = nextMediaEvent(t, scanner)
				require.Equal(t, "status", name)
				if state.Status == "ready" {
					break
				}
			}
			_ = resp.Body.Close()
			var repaired mm.MediaAssetVariant
			require.NoError(t, f.db.Where("asset_uuid=? AND variant=?", created.AssetUUID, "preview").First(&repaired).Error)
			require.Equal(t, preview.UUID, repaired.UUID)
			_, object, err := f.svc.OpenAssetResource(context.Background(), f.tenant, created.AssetUUID)
			require.NoError(t, err)
			original, err := io.ReadAll(object.Body)
			require.NoError(t, err)
			_ = object.Body.Close()
			require.Equal(t, raw.Bytes(), original)
		})
	}
}

func TestDerivativeServiceEventsRequireLiveGrants(t *testing.T) {
	f := newDerivativeHostFixture(t, "/custom/v2")
	a, err := f.svc.CreateAsset(context.Background(), svc.CreateAssetInput{TenantUUID: f.tenant, Name: "waiting.png", MimeType: "image/png", SizeBytes: 10, ContentSHA256: strings.Repeat("1", 64), UploadMethod: svc.UploadMethodPresign})
	require.NoError(t, err)
	path := f.prefix + "/tenant/media/assets/" + a.UUID + "/derivatives/events"
	require.Equal(t, 401, f.call(t, "GET", path, "", "").Code)
	require.Equal(t, 401, f.call(t, "GET", path, "", "admin").Code)
	f.grantEvents(t)
	foreign, err := f.svc.CreateAsset(context.Background(), svc.CreateAssetInput{TenantUUID: uuid.NewString(), Name: "other.png", MimeType: "image/png", SizeBytes: 10, ContentSHA256: strings.Repeat("2", 64), UploadMethod: svc.UploadMethodPresign})
	require.NoError(t, err)
	require.Equal(t, 404, f.call(t, "GET", strings.Replace(path, a.UUID, foreign.UUID, 1), "", "sts").Code)
	require.NoError(t, f.db.Model(&cm.CapabilityRegistration{}).Where("tenant_uuid=? AND capability_id=?", f.tenant, svc.ImageDerivativesReadCapability).Update("status", "disabled").Error)
	require.Equal(t, 403, f.call(t, "GET", path, "", "sts").Code)
	require.NoError(t, f.db.Model(&cm.CapabilityRegistration{}).Where("tenant_uuid=? AND capability_id=?", f.tenant, svc.ImageDerivativesReadCapability).Update("status", "published").Error)
	for _, actor := range []string{"sts", "key"} {
		t.Run(actor, func(t *testing.T) {
			require.Equal(t, 400, f.call(t, "GET", path+"?token=forbidden", "", actor).Code)
			require.Equal(t, 404, f.call(t, "GET", strings.Replace(path, a.UUID, uuid.NewString(), 1), "", actor).Code)
			resp, scanner := f.stream(t, a.UUID, actor)
			_, state := nextMediaEvent(t, scanner)
			require.Equal(t, "waiting_upload", state.Status)
			if actor == "sts" {
				require.NoError(t, f.db.Model(f.credential).Update("enabled", false).Error)
			} else {
				require.NoError(t, f.db.Model(&f.key).Update("status", "disabled").Error)
			}
			require.NoError(t, f.client.Publish(context.Background(), svc.ImageDerivativeEventChannel(f.tenant, a.UUID), a.UUID).Err())
			name, _ := nextMediaEvent(t, scanner)
			require.Equal(t, "invalidate", name)
			for scanner.Scan() {
				require.Empty(t, scanner.Text(), "revoked stream may only finish the SSE frame before closing")
			}
			_ = resp.Body.Close()
			if actor == "sts" {
				require.NoError(t, f.db.Model(f.credential).Update("enabled", true).Error)
			}
		})
	}
}
