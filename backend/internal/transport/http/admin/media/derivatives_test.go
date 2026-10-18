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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	svc "github.com/ArtisanCloud/PowerX/internal/service/media"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 实际 Gin -> Service -> SQLite/本地存储 -> TaskBus -> Worker -> SSE。
// 测试仅注入认证上下文，不声称验收真实 JWT 签名或线上插件客户端。
func TestDerivativeHTTPWorkerAndPushWithConfiguredPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, prefix := range []string{"/api/v1", "/custom/v2"} {
		t.Run(prefix, func(t *testing.T) {
			previous := core.PowerXSchema
			core.PowerXSchema = "main"
			t.Cleanup(func() { core.PowerXSchema = previous })
			db, err := gorm.Open(sqlite.Open(t.TempDir()+"/media.db?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			pool, err := db.DB()
			require.NoError(t, err)
			pool.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = pool.Close() })
			for _, model := range []any{&m.MediaAsset{}, &m.MediaAssetVariant{}, &ef.TaskHistory{}} {
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
			require.NoError(t, db.AutoMigrate(&m.MediaAsset{}, &m.MediaAssetVariant{}, &ef.TaskHistory{}))
			store, err := local.New(local.Options{BasePath: t.TempDir()})
			require.NoError(t, err)
			manager := mediamgr.New("local")
			manager.RegisterDriver(store)
			service := svc.NewMediaService(db, nil, manager, nil, time.Hour)
			mini := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			queue, err := event_bus.NewRedisStreamTaskDriver(client, 2*time.Minute)
			require.NoError(t, err)
			service.ConfigureDerivativeQueue(taskqueue.NewStreamDriver(db, queue))
			service.SetDerivativeEvents(func(ctx context.Context, tenant, asset string) {
				require.NoError(t, client.Publish(ctx, svc.ImageDerivativeEventChannel(tenant, asset), asset).Err())
			})
			tenant := uuid.NewString()
			ctx := context.Background()
			var raw bytes.Buffer
			require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 10, 20))))
			sum := sha256.Sum256(raw.Bytes())
			checksum := hex.EncodeToString(sum[:])
			a, err := service.CreateAsset(ctx, svc.CreateAssetInput{TenantUUID: tenant, Name: "picture.png", MimeType: "image/png", SizeBytes: int64(raw.Len()), ContentSHA256: checksum, UploadMethod: svc.UploadMethodPresign})
			require.NoError(t, err)
			_, err = manager.Put(ctx, "local", driver.PutObjectInput{ObjectKey: a.StorageKey, Body: bytes.NewReader(raw.Bytes()), Size: int64(raw.Len()), ContentType: "image/png"})
			require.NoError(t, err)
			_, err = service.CompleteAssetUpload(ctx, svc.CompleteAssetUploadInput{TenantUUID: tenant, UUID: a.UUID, Checksum: checksum})
			require.NoError(t, err)
			var requests atomic.Int32
			engine := gin.New()
			protected := engine.Group(prefix, func(c *gin.Context) {
				requests.Add(1)
				if c.GetHeader("X-Test-Actor") != "admin" {
					c.AbortWithStatus(401)
					return
				}
				currentTenant := tenant
				if c.GetHeader("X-Test-Tenant") != "" {
					currentTenant = c.GetHeader("X-Test-Tenant")
				}
				c.Request = c.Request.WithContext(reqctx.WithClaims(reqctx.WithTenantUUID(c.Request.Context(), currentTenant), &reqctx.CoreXClaims{TenantUUID: currentTenant, MemberID: 1}))
				c.Next()
			})
			RegisterAPIRoutes(engine.Group(prefix), protected, &shared.Deps{DB: db, MediaSvc: service, EventFabric: &shared.EventFabricDeps{RedisClient: client}})
			server := httptest.NewServer(engine)
			defer server.Close()
			path := prefix + "/admin/media/assets/" + a.UUID + "/derivatives"
			call := func(method, path, body, actor, tenantOverride string) *http.Response {
				request, e := http.NewRequest(method, server.URL+path, strings.NewReader(body))
				require.NoError(t, e)
				request.Header.Set("X-Test-Actor", actor)
				request.Header.Set("X-Test-Tenant", tenantOverride)
				response, e := http.DefaultClient.Do(request)
				require.NoError(t, e)
				return response
			}
			resp := call("GET", path, "", "", " ")
			require.Equal(t, 401, resp.StatusCode)
			_ = resp.Body.Close()
			resp = call("GET", path, "", "admin", uuid.NewString())
			require.Equal(t, 404, resp.StatusCode)
			_ = resp.Body.Close()
			resp = call("POST", path+"/ensure", `{"tenant_uuid":"forbidden"}`, "admin", "")
			require.Equal(t, 400, resp.StatusCode)
			_ = resp.Body.Close()
			resp = call("POST", path+"/ensure", `{}`, "admin", "")
			require.Equal(t, 200, resp.StatusCode)
			_ = resp.Body.Close()
			streamCtx, streamCancel := context.WithTimeout(ctx, 10*time.Second)
			defer streamCancel()
			request, err := http.NewRequestWithContext(streamCtx, "GET", server.URL+path+"/events", nil)
			require.NoError(t, err)
			request.Header.Set("X-Test-Actor", "admin")
			resp, err = http.DefaultClient.Do(request)
			require.NoError(t, err)
			require.Equal(t, 200, resp.StatusCode)
			defer resp.Body.Close()
			scanner := bufio.NewScanner(resp.Body)
			readStatus := func() string {
				for scanner.Scan() {
					line := scanner.Text()
					if strings.HasPrefix(line, "data: ") {
						var state svc.ImageDerivatives
						require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &state))
						return state.Status
					}
				}
				t.Fatalf("SSE closed: %v", scanner.Err())
				return ""
			}
			require.Equal(t, "queued", readStatus())
			workerCtx, workerCancel := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() { done <- service.RunImageDerivativeWorker(workerCtx) }()
			defer func() {
				workerCancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("media worker did not stop")
				}
			}()
			before := requests.Load()
			for status := readStatus(); status != "ready"; status = readStatus() {
				require.Contains(t, []string{"queued", "processing"}, status)
			}
			require.Equal(t, before, requests.Load(), "状态由 SSE 推送，没有请求轮询")
			streamCancel()
			_ = resp.Body.Close()
			resp = call("GET", prefix+"/admin/media/assets/"+a.UUID+"/variants/preview/resource", "", "admin", "")
			data, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			_ = resp.Body.Close()
			require.Equal(t, 200, resp.StatusCode, string(data))
			_, err = png.Decode(bytes.NewReader(data))
			require.NoError(t, err)
		})
	}
}
