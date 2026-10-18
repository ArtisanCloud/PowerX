package manager

import (
	"github.com/ArtisanCloud/PowerX/config"
	dbcfg "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRecoveryPluginUnixSocketDSN(t *testing.T) {
	section := &databaseSection{User: "pxu_dev_fixture", Password: "p'ass\\word", Schema: "px_fixture"}
	dsn := buildPostgresPluginDSN(dbcfg.DatabaseConfig{Host: "/private/tmp/recovery path/s", Port: 15432, Database: "powerx", Timezone: "Asia/Shanghai", DSN: "host='/private/tmp/recovery path/s' port=15432 user=admin dbname=powerx sslmode=disable"}, section)
	parsed, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	require.Equal(t, "/private/tmp/recovery path/s", parsed.Host)
	require.Equal(t, section.User, parsed.User)
	require.Equal(t, section.Password, parsed.Password)
	require.Equal(t, "px_fixture", parsed.RuntimeParams["search_path"])
	require.EqualValues(t, 15432, parsed.Port)
	require.Equal(t, "Asia/Shanghai", parsed.RuntimeParams["TimeZone"])
	require.Contains(t, dsn, "TimeZone=Asia/Shanghai")
	require.NotContains(t, dsn, "TimeZone='Asia/Shanghai'")
}

func TestRecoveryUsesEncryptedDeployedSTSIdentity(t *testing.T) {
	values := map[string]string{"POWERX_STS_CLIENT_ID": "registry-old", "POWERX_STS_CLIENT_SECRET": "registry-old", "POWERX_DB_DSN": "target-database"}
	err := restoreSTSIdentity(values, []byte("env:\n  POWERX_STS_CLIENT_ID: deployed-client\n  POWERX_STS_CLIENT_SECRET: encrypted-package-secret\n  POWERX_STS_TENANT_UUID: deployed-tenant\n  POWERX_DB_DSN: old-source-database\n"))
	require.NoError(t, err)
	require.Equal(t, "deployed-client", values["POWERX_STS_CLIENT_ID"])
	require.Equal(t, "encrypted-package-secret", values["POWERX_STS_CLIENT_SECRET"])
	require.Equal(t, "deployed-tenant", values["POWERX_STS_TENANT_UUID"])
	require.Equal(t, "target-database", values["POWERX_DB_DSN"])
}

func TestRecoveryPluginWorkersFollowOwnedLocalRehearsalPolicy(t *testing.T) {
	cfg := config.GetDefaults()
	cfg.Deployment.Env = "dev"
	cfg.Recovery.Mode = "active"
	manager := &managerImpl{opts: Options{CoreConfig: cfg}}
	plugin := plugin_mgr.Plugin{HostConfig: &plugin_mgr.HostConfig{Values: map[string]string{"POWERX_DISABLE_BACKGROUND_WORKERS": "false"}}}
	t.Setenv("POWERX_LOCAL_REHEARSAL", "1")
	require.Equal(t, "true", manager.hostEnvForPlugin(plugin)["POWERX_DISABLE_BACKGROUND_WORKERS"])
	t.Setenv("POWERX_LOCAL_REHEARSAL", "")
	require.Equal(t, "false", manager.hostEnvForPlugin(plugin)["POWERX_DISABLE_BACKGROUND_WORKERS"])
	cfg.Recovery.Mode = "verify"
	require.Equal(t, "true", manager.hostEnvForPlugin(plugin)["POWERX_DISABLE_BACKGROUND_WORKERS"])
}

func TestRecoveryRedisCurrentBindingMustBelongToCapturedInstance(t *testing.T) {
	raw := []byte("env:\n  POWERX_REDIS_ADDR: localhost:6379\n  POWERX_RECOVERY_REDIS_SOURCE_ADDR: older-source:9999\n")
	require.NoError(t, validateRecoveryRedisSource(raw, "127.0.0.1:6379"))
	require.ErrorContains(t, validateRecoveryRedisSource(raw, "127.0.0.1:6380"), "RECOVERY_PLUGIN_REDIS_EXTERNAL_UNSUPPORTED")
	require.ErrorContains(t, validateRecoveryRedisSource(raw, "another-instance:6379"), "RECOVERY_PLUGIN_REDIS_EXTERNAL_UNSUPPORTED")
	values := map[string]string{"POWERX_REDIS_ADDR": "new-target:16222"}
	require.NoError(t, restoreSTSIdentity(values, raw))
	require.Equal(t, "older-source:9999", values["POWERX_RECOVERY_REDIS_SOURCE_ADDR"])
	require.Equal(t, "new-target:16222", values["POWERX_REDIS_ADDR"])
}
