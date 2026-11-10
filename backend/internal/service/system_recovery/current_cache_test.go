package system_recovery

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCurrentCacheRestartsKeepScopeButRecoveryCreatesFreshScope(t *testing.T) {
	r, cfg, _ := currentFixture(t)
	cfg.Cache.Driver = "redis"
	a, err := CurrentCacheNamespace(cfg, r.spec.ConfigFile)
	require.NoError(t, err)
	again, err := CurrentCacheNamespace(cfg, r.spec.ConfigFile)
	require.NoError(t, err)
	require.Equal(t, a, again)
	other, err := CurrentCacheNamespace(cfg, filepath.Join(t.TempDir(), "other.yaml"))
	require.NoError(t, err)
	require.Equal(t, a, other, "配置路径不是另一套 Redis 身份")
	cfg.Database.DSN = "postgres://rotated@database/current_fixture?sslmode=disable"
	other, err = CurrentCacheNamespace(cfg, r.spec.ConfigFile)
	require.NoError(t, err)
	require.Equal(t, a, other, "数据库连接变化不改变环境身份")
	originalID := cfg.EnvironmentID
	cfg.EnvironmentID = "another-existing-environment"
	other, err = CurrentCacheNamespace(cfg, r.spec.ConfigFile)
	require.NoError(t, err)
	require.NotEqual(t, a, other)
	cfg.EnvironmentID = originalID
	require.NoError(t, r.rotateCurrentCache())
	after, err := CurrentCacheNamespace(cfg, r.spec.ConfigFile)
	require.NoError(t, err)
	require.NotEqual(t, a, after)
	again, err = CurrentCacheNamespace(cfg, r.spec.ConfigFile)
	require.NoError(t, err)
	require.Equal(t, after, again)
}

func TestCurrentRedisDoesNotTreatDisposableSharedCacheAsPersistentTasks(t *testing.T) {
	_, cfg, _ := currentFixture(t)
	cfg.Cache.Driver, cfg.Cache.Host, cfg.Cache.Port, cfg.Cache.DB = "redis", "shared-cache.example", 6379, 0
	cfg.Queue.Driver, cfg.Event.Bus.Type = "local", "local"
	endpoints, err := currentRedisEndpoints(cfg)
	require.NoError(t, err)
	require.Empty(t, endpoints)
	require.NoError(t, checkCurrentRedis(context.Background(), cfg), "缓存库不应建立连接或要求独占")
	cfg.Queue.Driver, cfg.Queue.Redis.Addr, cfg.Queue.Redis.DB = "redis", "tasks.example:6379", 5
	endpoints, err = currentRedisEndpoints(cfg)
	require.NoError(t, err)
	require.Len(t, endpoints, 1)
	require.Equal(t, "queue", endpoints[0].role)
	require.Equal(t, 5, endpoints[0].options.DB)
}
