package devhotloadintegration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	devhotload "github.com/ArtisanCloud/PowerX/internal/service/dev_hotload"
	devhotloadinstr "github.com/ArtisanCloud/PowerX/internal/service/dev_hotload/instrumentation"
	devhotloadstore "github.com/ArtisanCloud/PowerX/internal/service/dev_hotload/store"
	devhotloadHTTP "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/dev_hotload"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/dev_hotload"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDevAPIFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	env := newDevHotloadEnv(t)
	client := &http.Client{Timeout: 2 * time.Second}

	registerBody := map[string]interface{}{
		"pluginId":    "com.powerx.dev",
		"tenantId":    123,
		"developerId": 456,
		"buildHash":   "abc123",
		"entryPoints": []string{"backend", "frontend"},
		"metadata":    map[string]any{"branch": "main"},
	}
	registerResp := env.doJSONRequest(t, client, http.MethodPost, "/api/internal/dev/plugins/register", registerBody, http.StatusCreated)
	var session devhotload.RegisterResult
	decodeData(t, registerResp, &session)
	require.NotEmpty(t, session.SessionID)

	getResp := env.doJSONRequest(t, client, http.MethodGet, "/api/internal/dev/plugins/"+session.SessionID.String(), nil, http.StatusOK)
	var view map[string]any
	decodeData(t, getResp, &view)
	require.Equal(t, "com.powerx.dev", view["pluginId"])

	reloadBody := map[string]any{
		"sessionId":    session.SessionID.String(),
		"reloadToken":  session.ReloadToken,
		"sequence":     1,
		"durationMs":   1200,
		"changedFiles": []string{"main.go"},
		"artifacts":    []map[string]any{{"path": "dist.zip"}},
		"success":      true,
		"error":        "",
	}
	reloadResp := env.doJSONRequest(t, client, http.MethodPost, "/api/internal/dev/plugins/reload", reloadBody, http.StatusOK)
	reloadResp.Body.Close()

	delResp := env.doJSONRequest(t, client, http.MethodDelete, "/api/internal/dev/plugins/register/"+session.SessionID.String(), nil, http.StatusOK)
	delResp.Body.Close()

	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()
	streamReq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, env.server.URL+"/api/internal/dev/plugins/stream", nil)
	require.NoError(t, err)
	streamDone := make(chan string, 1)
	go func() {
		resp, err := client.Do(streamReq)
		if err != nil {
			streamDone <- ""
			return
		}
		defer resp.Body.Close()
		reader := bufio.NewReader(resp.Body)
		line, err := reader.ReadString('\n')
		if err == nil {
			streamDone <- line
		} else {
			streamDone <- ""
		}
	}()

	time.Sleep(50 * time.Millisecond)

	registerBody2 := map[string]interface{}{
		"pluginId":    "com.powerx.dev2",
		"tenantId":    999,
		"developerId": 2025,
	}
	registerResp2 := env.doJSONRequest(t, client, http.MethodPost, "/api/internal/dev/plugins/register", registerBody2, http.StatusCreated)
	var session2 devhotload.RegisterResult
	decodeData(t, registerResp2, &session2)

	select {
	case line := <-streamDone:
		require.Contains(t, line, session2.SessionID.String())
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE event")
	}

	delResp2 := env.doJSONRequest(t, client, http.MethodDelete, "/api/internal/dev/plugins/register/"+session2.SessionID.String(), nil, http.StatusOK)
	delResp2.Body.Close()
}

type devHotloadEnv struct {
	server *httptest.Server
}

func newDevHotloadEnv(t *testing.T) *devHotloadEnv {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(&model.DevHotloadSession{}, &model.DevHotloadSessionEvent{}))

	store := devhotloadstore.New(db, time.Now)
	opts := devhotload.Options{
		FeatureFlags: devhotload.FeatureFlags{Enabled: true, GatewayFlag: "PX_DEV_PLUGIN_HOTLOAD", SessionAuditFlag: "PX_DEV_SESSION_AUDIT"},
		Sessions:     devhotload.SessionOptions{TTL: 2 * time.Minute, MaxConcurrent: 5, CleanupInterval: time.Minute},
		Sandbox:      devhotload.SandboxOptions{Image: "dev", MaxCPUPercent: 25, MaxMemoryMB: 256, WatchFileLimit: 1000},
		Security:     devhotload.SecurityOptions{PATHeader: "X-Dev-Token", TokenTTL: time.Minute},
		Observability: devhotload.ObservabilityOptions{
			MetricsNamespace: "test.dev.hotload",
			SSEBufferSize:    16,
			AuditTopic:       "dev.hotload.test",
		},
	}

	registry := devhotload.NewRegistry(store, nil, devhotload.RegistryOptions{
		TTL:             opts.Sessions.TTL,
		MaxConcurrent:   opts.Sessions.MaxConcurrent,
		CleanupInterval: opts.Sessions.CleanupInterval,
	})
	service := devhotload.NewService(devhotload.ServiceDeps{
		Store:    store,
		Registry: registry,
		Options:  opts,
		Metrics:  devhotloadinstr.New("test.dev.hotload"),
		Notifier: devhotload.NewNotifier(0),
	})

	deps := &shared.Deps{
		DevHotloadService: service,
		DevHotloadOptions: shared.DevHotloadOptions{
			FeatureFlags: shared.DevHotloadFeatureFlagsOptions{
				Enabled:          true,
				GatewayFlag:      "PX_DEV_PLUGIN_HOTLOAD",
				SessionAuditFlag: "PX_DEV_SESSION_AUDIT",
			},
			Sessions: shared.DevHotloadSessionOptions{
				TTL:             opts.Sessions.TTL,
				MaxConcurrent:   opts.Sessions.MaxConcurrent,
				CleanupInterval: opts.Sessions.CleanupInterval,
			},
			Sandbox: shared.DevHotloadSandboxOptions{
				Image:          opts.Sandbox.Image,
				MaxCPUPercent:  opts.Sandbox.MaxCPUPercent,
				MaxMemoryMB:    opts.Sandbox.MaxMemoryMB,
				WatchFileLimit: opts.Sandbox.WatchFileLimit,
			},
			Security: shared.DevHotloadSecurityOptions{
				RequireMTLS:     opts.Security.RequireMTLS,
				AllowedSubjects: opts.Security.AllowedSubjects,
				PATHeader:       opts.Security.PATHeader,
				TokenTTL:        opts.Security.TokenTTL,
			},
			Observability: shared.DevHotloadObservabilityOptions{
				MetricsNamespace: opts.Observability.MetricsNamespace,
				SSEBufferSize:    opts.Observability.SSEBufferSize,
				AuditTopic:       opts.Observability.AuditTopic,
			},
		},
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &reqctx.CoreXClaims{
			IsRoot: true,
			Roles:  []string{"system_admin"},
		}
		ctx := reqctx.WithClaims(c.Request.Context(), claims)
		ctx = reqctx.WithIsRoot(ctx, true)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	api := router.Group("/api")
	devhotloadHTTP.RegisterAPIRoutes(nil, api, deps)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	return &devHotloadEnv{
		server: server,
	}
}

func (e *devHotloadEnv) doJSONRequest(t *testing.T, client *http.Client, method, path string, body interface{}, expect int) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.server.URL+path, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, expect, resp.StatusCode)
	return resp
}

type successEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func decodeData[T any](t *testing.T, resp *http.Response, target *T) {
	t.Helper()
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var env successEnvelope
	require.NoError(t, json.Unmarshal(payload, &env))
	require.Equal(t, resp.StatusCode, env.Code)
	require.NoError(t, json.Unmarshal(env.Data, target))
}
