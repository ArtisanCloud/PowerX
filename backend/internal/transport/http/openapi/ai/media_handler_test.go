package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ai"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
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

func TestMediaSSEInitialSnapshotEventAndHeartbeatWithoutTaskPolling(t *testing.T) {
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	defer func() { core.PowerXSchema = old }()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/events.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, e := db.DB()
	require.NoError(t, e)
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&m.MediaTask{}))
	tenant := uuid.NewString()
	row := m.MediaTask{TenantUUID: tenant, Subject: "user:root", Capability: "video_generation", IdempotencyKey: "test", RequestDigest: "test", Env: "dev", ModelKey: "test/model", Status: "queued", Phase: "queued", Revision: 1, TraceID: "trace", Request: datatypes.JSON(`{}`), CancelStatus: "none"}
	require.NoError(t, db.Create(&row).Error)
	var reads atomic.Int64
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("count_media_reads", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && strings.Contains(tx.Statement.Schema.Table, "ai_media_tasks") {
			reads.Add(1)
		}
	}))
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	h := &aiHandler{media: aisvc.NewMediaRuntime(db, nil), mediaHeartbeat: 5 * time.Millisecond}
	router := gin.New()
	router.GET("/tasks/:task_uuid/events", func(c *gin.Context) {
		ctx := reqctx.WithClaims(reqctx.WithTenantUUID(c.Request.Context(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, IsRoot: true, RegisteredClaims: jwt.RegisteredClaims{Subject: "root", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
		c.Request = c.Request.WithContext(ctx)
		h.mediaTaskEvents(c, client)
	})
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/tasks/"+row.UUID.String()+"/events", nil)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	scanner := bufio.NewScanner(resp.Body)
	heartbeats := 0
	initial := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			initial = true
		}
		if line == ": keepalive" {
			heartbeats++
		}
		if initial && heartbeats >= 3 {
			break
		}
	}
	require.True(t, initial)
	require.EqualValues(t, 1, reads.Load(), "heartbeat must not read task details")
	require.NoError(t, db.Model(&row).Updates(map[string]any{"status": "running", "revision": 2}).Error)
	require.NoError(t, client.Publish(context.Background(), aisvc.MediaTaskEventChannel(tenant, row.UUID.String()), row.UUID.String()).Err())
	found := false
	for scanner.Scan() {
		if scanner.Text() == "id: "+row.UUID.String()+":2" {
			found = true
			break
		}
	}
	require.True(t, found)
	require.EqualValues(t, 2, reads.Load())
	// The task changes while Redis is disconnected; resubscribe must recover
	// the latest snapshot even though that notification could not be delivered.
	mini.Close()
	require.NoError(t, db.Model(&row).Updates(map[string]any{"status": "succeeded", "revision": 3}).Error)
	require.NoError(t, mini.Restart())
	found = false
	for scanner.Scan() {
		if scanner.Text() == "id: "+row.UUID.String()+":3" {
			found = true
			break
		}
	}
	require.True(t, found, "Redis resubscribe must synchronize missed state")
	require.EqualValues(t, 3, reads.Load())
}

func TestMediaRESTRejectsOldVersionAndFreeProxyPayload(t *testing.T) {
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	defer func() { core.PowerXSchema = old }()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/requests.db"), &gorm.Config{})
	require.NoError(t, err)
	pool, e := db.DB()
	require.NoError(t, e)
	defer pool.Close()
	h := &aiHandler{media: aisvc.NewMediaRuntime(db, nil)}
	for _, test := range []struct {
		body   string
		status int
	}{{`{"model_key":"test/model","inputs":[{"type":"text","content":"test"}]}`, 409}, {`{"contract_version":2,"method":"POST","endpoint":"https://example.com"}`, 400}} {
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/ai/image/invoke", strings.NewReader(test.body))
		h.invokeMedia(ctx, "image_generation")
		require.Equal(t, test.status, response.Code)
	}
}

func TestMediaRESTPreservesStructuredFailureDetails(t *testing.T) {
	for _, failure := range []struct {
		status  int
		code    string
		details map[string]any
	}{
		{422, "AI_PARAMETER_INVALID", map[string]any{"parameter": "temperature", "reason": "unsupported_parameter"}},
		{429, "AI_PROVIDER_REJECTED", map[string]any{"upstream_status": 429, "upstream_code": "RateLimit", "provider_trace_id": "provider-trace"}},
	} {
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/ai/image/invoke", nil)
		ctx.Request.Header.Set("X-Request-ID", "request-trace")
		business := aisvc.MediaError(failure.status, failure.code)
		business.Details = failure.details
		require.True(t, respondMediaError(ctx, business))
		require.Equal(t, failure.status, response.Code)
		var result map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		require.Equal(t, failure.code, result["error_code"])
		require.Equal(t, "request-trace", result["request_id"])
		want, err := json.Marshal(failure.details)
		require.NoError(t, err)
		actual, err := json.Marshal(result["details"])
		require.NoError(t, err)
		require.JSONEq(t, string(want), string(actual))
	}
}
