package plugin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	install "github.com/ArtisanCloud/PowerX/internal/service/plugin_install"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_install"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type httpInstallStub struct {
	mu               sync.Mutex
	p                pm.Plugin
	started, release chan struct{}
	once             sync.Once
}

func (f *httpInstallStub) InstallFromFile(ctx context.Context, path string, o pm.InstallOptions) (pm.Plugin, error) {
	p := pm.Plugin{ID: "com.powerx.plugins.http-install", Version: "1.0.0", Revision: o.InstallationRevision}
	if e := o.Progress(ctx, "migrating", p); e != nil {
		return p, e
	}
	f.once.Do(func() { close(f.started) })
	select {
	case <-f.release:
	case <-ctx.Done():
		return p, ctx.Err()
	}
	f.mu.Lock()
	f.p = p
	f.mu.Unlock()
	return p, nil
}
func (f *httpInstallStub) InstallFromURL(ctx context.Context, u, sha, sig string, o pm.InstallOptions) (pm.Plugin, error) {
	return f.InstallFromFile(ctx, "", o)
}
func (f *httpInstallStub) Get(ctx context.Context, id string) (pm.Plugin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.p.ID == "" {
		return pm.Plugin{}, pm.NewError(pm.CodeNotFound)
	}
	return f.p, nil
}
func (f *httpInstallStub) Enable(ctx context.Context, id string) error { return ctx.Err() }
func TestInstallationHTTPAndSSESurviveClientDisconnect(t *testing.T) {
	dsn := os.Getenv("POWERX_INSTALL_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL and Redis required")
	}
	u, e := url.Parse(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "powerx_install_test_"))
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	old := core.PowerXSchema
	schema := "install_http_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	core.PowerXSchema = schema
	require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	require.NoError(t, db.AutoMigrate(&m.Installation{}, &ef.TaskHistory{}))
	t.Cleanup(func() {
		require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
		core.PowerXSchema = old
		pool, _ := db.DB()
		pool.Close()
	})
	client := redis.NewClient(&redis.Options{Addr: os.Getenv("POWERX_INSTALL_TEST_REDIS")})
	defer client.Close()
	base, e := event_bus.NewRedisStreamTaskDriver(client, time.Minute)
	require.NoError(t, e)
	queue := taskqueue.NewStreamDriver(db, base)
	mgr := &httpInstallStub{started: make(chan struct{}), release: make(chan struct{})}
	tenant, actor := uuid.NewString(), uuid.NewString()
	var checks atomic.Int32
	svc, e := install.NewService(db, install.Options{Root: filepath.Join(t.TempDir(), "staging"), InstanceID: schema, Secret: []byte(strings.Repeat("x", 32)), Queue: queue, Manager: mgr, Authorize: func(ctx context.Context, _ string, _ uint64) error {
		if claims := reqctx.GetClaims(ctx); claims != nil && claims.Issuer == "http-test" {
			checks.Add(1)
		}
		return nil
	}, Finalize: func(context.Context, string, pm.Plugin) error { return nil }, Notify: func(ctx context.Context, ch string) { _ = client.Publish(ctx, ch, "changed").Err() }})
	require.NoError(t, e)
	deps := &shared.Deps{DB: db, PluginInstallations: svc, EventFabric: &shared.EventFabricDeps{RedisClient: client}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(c.Request.Context(), tenant)
		ctx = reqctx.WithIsRoot(ctx, true)
		claims := &reqctx.CoreXClaims{UserID: 1, UserUUID: actor, IsRoot: true, TenantUUID: tenant}
		claims.Issuer = "http-test"
		ctx = reqctx.WithClaims(ctx, claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/installations", PluginInstallationCreate(deps))
	router.PUT("/installations/:task_id/package", PluginInstallationUpload(deps))
	router.GET("/installations/:task_id/events", PluginInstallationEvents(deps))
	router.GET("/installations/:task_id", PluginInstallationGet(deps))
	server := httptest.NewServer(router)
	defer server.Close()
	t.Logf("owned in-process HTTP fixture pid=%d listener=%s; httptest cleanup closes listener", os.Getpid(), server.Listener.Addr())
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = svc.Run(ctx) }()
	defer func() { stop(); <-done }()
	raw := fmt.Sprintf(`{"request_id":%q,"source_kind":"local","plugin_id":"com.powerx.plugins.http-install","version":"1.0.0","enable":true}`, uuid.NewString())
	response, e := http.Post(server.URL+"/installations", "application/json", strings.NewReader(raw))
	require.NoError(t, e)
	require.Equal(t, 200, response.StatusCode)
	var envelope struct {
		Data install.View `json:"data"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&envelope))
	response.Body.Close()
	id := envelope.Data.TaskID
	require.NotEmpty(t, id)
	streamCtx, disconnect := context.WithCancel(context.Background())
	defer disconnect()
	req, e := http.NewRequestWithContext(streamCtx, "GET", server.URL+"/installations/"+id+"/events", nil)
	require.NoError(t, e)
	stream, e := http.DefaultClient.Do(req)
	require.NoError(t, e)
	require.Equal(t, "text/event-stream", stream.Header.Get("Content-Type"))
	reader := bufio.NewReader(stream.Body)
	readFrame := func() string {
		t.Helper()
		var frame strings.Builder
		for {
			line, e := reader.ReadString('\n')
			require.NoError(t, e)
			frame.WriteString(line)
			if line == "\n" {
				return frame.String()
			}
		}
	}
	require.Contains(t, readFrame(), "awaiting_upload")
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, e := w.CreateFormFile("files", "plugin.yaml")
	require.NoError(t, e)
	_, e = io.WriteString(part, "id: com.powerx.plugins.http-install\nversion: 1.0.0\n")
	require.NoError(t, e)
	require.NoError(t, w.WriteField("file_paths", "linux/plugin.yaml"))
	require.NoError(t, w.Close())
	uploadCtx, cancelUpload := context.WithCancel(context.Background())
	req, e = http.NewRequestWithContext(uploadCtx, "PUT", server.URL+"/installations/"+id+"/package", &body)
	require.NoError(t, e)
	req.Header.Set("Content-Type", w.FormDataContentType())
	response, e = http.DefaultClient.Do(req)
	require.NoError(t, e)
	require.Equal(t, 200, response.StatusCode)
	response.Body.Close()
	cancelUpload()
	select {
	case <-mgr.started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	for !strings.Contains(readFrame(), `"phase":"migrating"`) {
	}
	// No change -> one heartbeat, with no status/authorization database read.
	before := checks.Load()
	for {
		frame := readFrame()
		if strings.Contains(frame, ": keepalive") {
			require.Equal(t, before, checks.Load())
			break
		}
		before = checks.Load()
	}
	disconnect()
	stream.Body.Close()
	close(mgr.release)
	require.Eventually(t, func() bool {
		r, e := http.Get(server.URL + "/installations/" + id)
		if e != nil {
			return false
		}
		defer r.Body.Close()
		var out struct {
			Data install.View `json:"data"`
		}
		return json.NewDecoder(r.Body).Decode(&out) == nil && out.Data.Status == "succeeded"
	}, 5*time.Second, 30*time.Millisecond)
}
