package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	pxcrypto "github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRestoreRootRejectsTenantAdminAndServicePlanes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		root   bool
		issuer string
		planes []string
		status int
	}{
		{"tenant admin", false, "powerx", nil, 403}, {"STS", true, "powerx-sts", nil, 403},
		{"API key", true, "powerx", []string{"api_key"}, 403}, {"root user", true, "powerx", nil, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				ctx := reqctx.WithIsRoot(c.Request.Context(), tc.root)
				ctx = reqctx.WithClaims(ctx, &reqctx.CoreXClaims{Platforms: tc.planes})
				claims := reqctx.GetClaims(ctx)
				claims.Issuer = tc.issuer
				c.Request = c.Request.WithContext(ctx)
			})
			router.GET("/restore", RequireRestoreRoot(), func(c *gin.Context) { c.Status(200) })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", "/restore", nil))
			require.Equal(t, tc.status, response.Code)
		})
	}
}

func TestRestoreRequestRejectsFreeFormAndOverrides(t *testing.T) {
	router := gin.New()
	router.POST("/plan", func(c *gin.Context) {
		var target restoreCopyRequest
		if bindRestoreJSON(c, &target) {
			c.Status(200)
		}
	})
	for _, body := range []string{`{"dsn":"postgres://elsewhere"}`, `{"database":"existing"}`, `{"sql":"DROP DATABASE current"}`, `{} {}`, `null`} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("POST", "/plan", strings.NewReader(body)))
		require.Equal(t, 400, w.Code, body)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/plan?source_job_id=1", strings.NewReader(`{}`)))
	require.Equal(t, 400, w.Code)
}

// 原生 PostgreSQL + 实际 JWT 中间件 + HTTP handlers；不启动日常 Core，不使用 Docker。
func TestRestoreCopyHTTPNativePostgres(t *testing.T) {
	dsn := os.Getenv("POWERX_RESTORE_HTTP_TEST_DSN")
	archive := os.Getenv("POWERX_RESTORE_HTTP_TEST_ARCHIVE")
	if dsn == "" || archive == "" {
		t.Skip("opt-in native HTTP restore acceptance")
	}
	ctx := context.Background()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	defer pool.Close()
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	admin, err := pgx.ConnectConfig(ctx, cfg)
	require.NoError(t, err)
	defer admin.Close(ctx)
	business := func() string {
		var counts string
		err := admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM px_com_powerx_plugins_ai_craft.ai_craft_design_tasks)::text||','||(SELECT count(*) FROM px_com_powerx_plugins_ai_craft.ai_craft_orders)::text||','||(SELECT count(*) FROM px_com_powerx_plugins_ai_craft.ai_craft_sample_rounds)::text`).Scan(&counts)
		require.NoError(t, err)
		return counts
	}
	before := business()
	key := make([]byte, 32)
	_, err = rand.Read(key)
	require.NoError(t, err)
	oldKey := pxcrypto.GetGlobalKeyB64()
	require.NoError(t, pxcrypto.SetGlobalKeyB64(base64.StdEncoding.EncodeToString(key)))
	defer pxcrypto.SetGlobalKeyB64(oldKey)
	svc := backupops.NewRestoreCopyService(ctx, db, backupops.RestoreCopyOptions{Directory: t.TempDir()})
	defer svc.Close()
	rootCtx := reqctx.WithIsRoot(ctx, true)
	defer func() {
		rows, err := svc.List(rootCtx)
		require.NoError(t, err)
		for _, row := range rows {
			if row.Status != "deleted" {
				require.Eventually(t, func() bool {
					r, e := svc.Get(rootCtx, row.ID)
					return e == nil && r.Status != "running" && r.Status != "queued"
				}, 40*time.Second, 100*time.Millisecond)
				require.NoError(t, svc.Delete(rootCtx, row.ID, row.Database))
			}
		}
		require.Equal(t, before, business())
	}()
	secret := make([]byte, 32)
	_, err = rand.Read(secret)
	require.NoError(t, err)
	token, err := auth.GenerateAccessJWT(reqctx.CoreXClaims{IsRoot: true, UserID: 1, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString()}, "restore-test", []string{"admin"}, time.Hour, secret)
	require.NoError(t, err)
	router := gin.New()
	router.Use(gin.Recovery(), authmw.JwtMiddleware(secret, "restore-test", []string{"admin"}, []string{"access"}, nil), RequireRestoreRoot())
	h := &handler{copySvc: svc}
	g := router.Group("/api/v1/admin/ops/backup/restore-copies")
	g.POST("/imports", h.ImportRestoreCopy)
	g.POST("/plan", h.PlanRestoreCopy)
	g.POST("", h.CreateRestoreCopy)
	g.GET("", h.ListRestoreCopies)
	g.GET("/:copy_id", h.GetRestoreCopy)
	g.POST("/:copy_id/connection", h.RestoreCopyConnection)
	g.DELETE("/:copy_id", h.DeleteRestoreCopy)
	server := httptest.NewServer(router)
	defer server.Close()
	fixture := os.Getenv("POWERX_RESTORE_HTTP_FIXTURE")
	if fixture != "" {
		payload, _ := json.Marshal(map[string]any{"url": server.URL, "token": token, "pid": os.Getpid(), "cwd": mustCwd(), "owner": "Codex restore-copy native HTTP fixture", "command": "go test ./internal/transport/http/admin/backup -run TestRestoreCopyHTTPNativePostgres", "current_database": cfg.Database, "archive": archive, "source_counts": before, "auth_boundary": "isolated signed Root test JWT; production login callback not exercised"})
		require.NoError(t, os.WriteFile(fixture, payload, 0600))
		t.Logf("owned native HTTP fixture ready: pid=%d url=%s metadata=%s", os.Getpid(), server.URL, fixture)
		require.Eventually(t, func() bool { _, e := os.Stat(fixture + ".done"); return e == nil }, 15*time.Minute, 200*time.Millisecond)
		t.Log("browser completed; cleaning owned copies, roles and fixture server")
		return
	}
	client := server.Client()
	call := func(method, path string, body io.Reader, headers map[string]string) (int, []byte, http.Header) {
		req, e := http.NewRequest(method, server.URL+g.BasePath()+path, body)
		require.NoError(t, e)
		req.Header.Set("Authorization", "Bearer "+token)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, e := client.Do(req)
		require.NoError(t, e)
		defer resp.Body.Close()
		raw, e := io.ReadAll(resp.Body)
		require.NoError(t, e)
		return resp.StatusCode, raw, resp.Header
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, err := writer.CreateFormFile("file", filepath.Base(archive))
	require.NoError(t, err)
	f, err := os.Open(archive)
	require.NoError(t, err)
	_, err = io.Copy(part, f)
	f.Close()
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	status, raw, _ := call("POST", "/imports", &buffer, map[string]string{"Content-Type": writer.FormDataContentType()})
	require.Equal(t, 200, status, string(raw))
	var upload struct {
		Data struct {
			Import backupops.RestoreImport `json:"import"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &upload))
	payload := fmt.Sprintf(`{"import_id":%q}`, upload.Data.Import.ID)
	status, raw, _ = call("POST", "/plan", strings.NewReader(payload), nil)
	require.Equal(t, 200, status, string(raw))
	var planning struct {
		Data struct {
			Plan backupops.RestoreCopyPlan `json:"plan"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &planning))
	require.True(t, planning.Data.Plan.Ready, string(raw))
	payload = fmt.Sprintf(`{"import_id":%q,"allow_older_server":true}`, upload.Data.Import.ID)
	status, raw, _ = call("POST", "", strings.NewReader(payload), map[string]string{"Idempotency-Key": uuid.NewString()})
	require.Equal(t, 202, status, string(raw))
	var created struct {
		Data struct {
			Copy backupops.RestoreCopy `json:"copy"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &created))
	row := created.Data.Copy
	require.Eventually(t, func() bool {
		current, e := svc.Get(rootCtx, row.ID)
		return e == nil && (current.Status == "success" || current.Status == "failed")
	}, 60*time.Second, 100*time.Millisecond)
	current, err := svc.Get(rootCtx, row.ID)
	require.NoError(t, err)
	require.Equal(t, "success", current.Status, current.ErrorMessage)
	require.EqualValues(t, 19, current.BusinessCounts["orders"])
	status, raw, headers := call("POST", "/"+row.ID+"/connection", strings.NewReader(`{}`), nil)
	require.Equal(t, 200, status, string(raw))
	require.Equal(t, "no-store", headers.Get("Cache-Control"))
	var credentials struct {
		Data struct {
			Connection backupops.RestoreCopyConnection `json:"connection"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &credentials))
	c := credentials.Data.Connection
	copyCfg := cfg.Copy()
	copyCfg.Database = c.Database
	copyCfg.User = c.Username
	copyCfg.Password = c.Password
	reader, err := pgx.ConnectConfig(ctx, copyCfg)
	require.NoError(t, err)
	var orders int
	require.NoError(t, reader.QueryRow(ctx, "SELECT count(*) FROM px_com_powerx_plugins_ai_craft.ai_craft_orders").Scan(&orders))
	require.Equal(t, 19, orders)
	_, err = reader.Exec(ctx, "DELETE FROM px_com_powerx_plugins_ai_craft.ai_craft_orders")
	require.Error(t, err)
	reader.Close(ctx)
	status, raw, _ = call("DELETE", "/"+row.ID, nil, map[string]string{"X-Restore-Confirmation": cfg.Database})
	require.Equal(t, 400, status, string(raw))
	status, raw, _ = call("DELETE", "/"+row.ID, nil, map[string]string{"X-Restore-Confirmation": row.Database})
	require.Equal(t, 200, status, string(raw))
	require.Equal(t, before, business())
	t.Logf("HTTP actual archive restored: tables=%d business=%v source unchanged=%s; owned copy deleted", current.TableCount, current.BusinessCounts, before)
}

func mustCwd() string { dir, _ := os.Getwd(); return dir }
