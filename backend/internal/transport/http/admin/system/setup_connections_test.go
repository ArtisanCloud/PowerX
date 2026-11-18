package system

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const (
	setupTestDBPassword    = "instance-db-secret"
	setupTestRedisPassword = "instance-redis-secret"
)

func containerSetupFixture(t *testing.T) (*gin.Engine, setupConfigPayload, string) {
	t.Helper()
	handler, draft, path := setupJWTFixture(t)
	t.Setenv("POWERX_MODE", "docker")
	require.NoError(t, os.Remove(filepath.Join(filepath.Dir(path), "setup.wizard.config.json")))
	require.NoError(t, os.WriteFile(path, []byte(`deployment: {env: test}
install: {status: uninstalled, allow_without_db: true}
database: {driver: postgres, host: postgres, port: 5432, database: powerx, username: powerx, password: instance-db-secret, ssl_mode: disable}
cache: {driver: redis, host: redis, port: 6379, password: instance-redis-secret, db: 2}
queue: {driver: redis, redis: {addr: 'redis:6379', password: instance-redis-secret, db: 3}}
event:
  bus: {type: redis, redis_addr: 'redis:6379', redis_password: instance-redis-secret}
  fabric:
    redis_addr: 'redis:6379'
    redis_password: instance-redis-secret
    authorization: {redis_addr: 'dedicated:6380', redis_password: dedicated-secret}
`), 0o600))
	router := gin.New()
	router.GET("/config", handler.GetConfig)
	router.PUT("/config", handler.SaveConfig)
	router.POST("/test/cache", handler.TestCacheConnection)
	router.POST("/test/database", handler.TestDatabaseConnection)
	router.POST("/provision", handler.Provision)
	draft.Database = setupDatabaseConfig{Source: setupSourceInstance}
	draft.Cache = setupCacheConfig{Source: setupSourceInstance}
	return router, draft, path
}

func setupConnectionRequest(t *testing.T, router http.Handler, method, path string, body any, status int) []byte {
	t.Helper()
	var input []byte
	if body != nil {
		var err error
		input, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(input))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	require.Equal(t, status, res.Code, res.Body.String())
	require.NotContains(t, res.Body.String(), setupTestDBPassword)
	require.NotContains(t, res.Body.String(), setupTestRedisPassword)
	return res.Body.Bytes()
}

func TestContainerSetupConfigPublishesOnlyConnectionMetadata(t *testing.T) {
	router, draft, path := containerSetupFixture(t)
	body := setupConnectionRequest(t, router, http.MethodGet, "/config", nil, 200)
	var response struct {
		Data struct {
			Config setupConfigPayload
		}
	}
	require.NoError(t, json.Unmarshal(body, &response))
	db, cache := response.Data.Config.Database, response.Data.Config.Cache
	require.Equal(t, setupDatabaseConfig{
		Source: "instance", PasswordConfigured: true, Type: "postgresql",
		Host: "postgres", Port: 5432, Name: "powerx", Username: "powerx", SSLMode: "disable",
	}, db)
	require.Equal(t, setupCacheConfig{
		Source: "instance", PasswordConfigured: true, Type: "redis",
		RedisHost: "redis", RedisPort: 6379, RedisDB: 2,
	}, cache)

	body = setupConnectionRequest(t, router, http.MethodPut, "/config", draft, 200)
	require.NotContains(t, string(body), draft.Admin.Password)
	stored, err := os.ReadFile(filepath.Join(filepath.Dir(path), "setup.wizard.config.json"))
	require.NoError(t, err)
	require.NotContains(t, string(stored), setupTestDBPassword)
	require.NotContains(t, string(stored), setupTestRedisPassword)
	var saved setupConfigPayload
	require.NoError(t, json.Unmarshal(stored, &saved))
	require.Equal(t, setupDatabaseConfig{Source: "instance"}, saved.Database)
	require.Equal(t, setupCacheConfig{Source: "instance"}, saved.Cache)
	body = setupConnectionRequest(t, router, http.MethodGet, "/config", nil, 200)
	require.NotContains(t, string(body), draft.Admin.Password)
}

func TestContainerSetupUsesEachSelectedConnectionSource(t *testing.T) {
	router, draft, path := containerSetupFixture(t)
	instance := miniredis.RunT(t)
	instance.RequireAuth(setupTestRedisPassword)
	external := miniredis.RunT(t)
	external.RequireAuth("external-only")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, bytes.Replace(raw,
		[]byte("host: redis, port: 6379"), []byte("host: "+instance.Host()+", port: "+instance.Port()), 1), 0o600))
	port, err := strconv.Atoi(external.Port())
	require.NoError(t, err)
	target := setupCacheConfig{Source: setupSourceInstance, Type: "redis", RedisHost: external.Host(), RedisPort: port}
	setupConnectionRequest(t, router, http.MethodPost, "/test/cache", setupCacheTestRequest{Cache: target}, 200)

	target.Source = setupSourceManual
	call := func(code int) string {
		return string(setupConnectionRequest(t, router, http.MethodPost, "/test/cache", setupCacheTestRequest{Cache: target}, code))
	}
	require.Contains(t, call(400), "setup.connection.cachePasswordRequired")
	target.RedisPass = "wrong-external-password"
	res := call(400)
	require.Contains(t, res, "setup.connection.redisAuthInvalid")
	require.NotContains(t, res, target.RedisPass)
	target.RedisPass = "external-only"
	call(200)

	db := setupDatabaseConfig{Source: setupSourceManual, Type: "postgresql", Host: "external", Port: 5432, Name: "powerx", Username: "powerx"}
	res = string(setupConnectionRequest(t, router, http.MethodPost, "/test/database", setupDatabaseTestRequest{Database: db}, 400))
	require.Contains(t, res, "setup.connection.databasePasswordRequired")
	draft.Cache = target
	setupConnectionRequest(t, router, http.MethodPut, "/config", draft, 200)
	res = string(setupConnectionRequest(t, router, http.MethodGet, "/config", nil, 200))
	require.Contains(t, res, `"source":"manual"`)
	require.NotContains(t, res, target.RedisPass)

	// Another instance's password cannot authenticate to the configured Redis.
	instance.RequireAuth("another-instance-password")
	res = string(setupConnectionRequest(t, router, http.MethodPost, "/test/cache",
		setupCacheTestRequest{Cache: setupCacheConfig{Source: setupSourceInstance}}, 400))
	require.Contains(t, res, "setup.connection.redisAuthInvalid")
}

func TestContainerSetupStagesExternalConnectionsWithoutReplacingInstanceDefaults(t *testing.T) {
	_, draft, path := containerSetupFixture(t)
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	draft.Database = setupDatabaseConfig{
		Source: setupSourceManual, Type: "postgresql", Host: "external-db", Port: 5434,
		Name: "external", Username: "external-user", Password: "external :/@$password", SSLMode: "require",
	}
	draft.Cache = setupCacheConfig{
		Source: setupSourceManual, Type: "redis", RedisHost: "external-cache", RedisPort: 6380,
		RedisPass: "external-redis-secret", RedisDB: 4,
	}
	staged, err := stageSetupRuntimeConfig(path, original, draft)
	require.NoError(t, err)
	defer os.Remove(staged)
	cfg, err := config.Load(staged)
	require.NoError(t, err)
	require.Equal(t, draft.Database.Password, cfg.Database.Password)
	require.Equal(t, draft.Cache.RedisPass, cfg.Cache.Password)
	require.Equal(t, 4, cfg.Cache.DB)
	require.Equal(t, "external-cache:6380", cfg.Queue.Redis.Addr)
	require.Equal(t, draft.Cache.RedisPass, cfg.Queue.Redis.Password)
	require.Equal(t, 3, cfg.Queue.Redis.DB)
	require.Equal(t, cfg.Queue.Redis.Addr, cfg.Event.Bus.RedisAddr)
	require.Equal(t, cfg.Queue.Redis.Addr, cfg.Event.Fabric.RedisAddr)
	require.Equal(t, "dedicated:6380", cfg.Event.Fabric.Authorization.RedisAddr)
	require.Equal(t, "dedicated-secret", cfg.Event.Fabric.Authorization.RedisPassword)
	current, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, current)

	t.Setenv("CORE_X_DB_HOST", "somewhere-else")
	_, err = stageSetupRuntimeConfig(path, original, draft)
	require.EqualError(t, err, "setup.connection.runtimeConflict")
}

func TestContainerSetupMissingMountedCredentialsDoNotUseDefaults(t *testing.T) {
	router, draft, path := containerSetupFixture(t)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	raw = bytes.Replace(raw, []byte(", password: "+setupTestDBPassword), nil, 1)
	raw = bytes.Replace(raw, []byte(", password: "+setupTestRedisPassword), nil, 1)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	response := setupConnectionRequest(t, router, http.MethodGet, "/config", nil, 200)
	require.NotContains(t, string(response), `"password_configured":true`)
	response = setupConnectionRequest(t, router, http.MethodPost, "/test/database", setupDatabaseTestRequest{Database: draft.Database}, 400)
	require.Contains(t, string(response), "setup.connection.instanceDatabaseUnavailable")
	response = setupConnectionRequest(t, router, http.MethodPost, "/test/cache", setupCacheTestRequest{Cache: draft.Cache}, 400)
	require.Contains(t, string(response), "setup.connection.instanceCacheUnavailable")
}
