package manager

import (
	"os"
	"strings"
	"testing"

	coreconfig "github.com/ArtisanCloud/PowerX/config"
	corexdb "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/stretchr/testify/require"
)

// 仅允许显式指定的隔离库，避免回归验收触碰用户的备份验证库或生产库。
func TestPostgresPluginIsolationUsesConnectedDSNIdentity(t *testing.T) {
	dsn := os.Getenv("POWERX_TEST_PLUGIN_ISOLATION_DSN")
	if dsn == "" {
		t.Skip("set POWERX_TEST_PLUGIN_ISOLATION_DSN for disposable PostgreSQL acceptance")
	}
	cfg := corexdb.DatabaseConfig{
		Driver: "postgres", DSN: dsn, Host: "stale-host.invalid", Port: 1,
		Database: "stale_database_that_does_not_exist", UserName: "stale_core_role", LogLevel: "silent",
	}
	admin, closeAdmin, err := connectAdminDB(cfg)
	require.NoError(t, err)
	defer closeAdmin()
	var actualDatabase, actualUser string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&actualDatabase).Error)
	require.NoError(t, admin.Raw("SELECT current_user").Scan(&actualUser).Error)
	require.Equal(t, "powerx_plugin_isolation_verify", actualDatabase)

	pluginID := "com.powerx.plugins.dsn-identity-verify"
	schema := makePluginSchema(pluginID)
	// 模拟备份恢复后的业务对象，由实际 Core 管理角色持有。
	require.NoError(t, ensureSchemaExists(admin, "postgres", schema))
	table := quoteIdentifier("postgres", schema) + ".preserved_business_record"
	require.NoError(t, admin.Exec("CREATE TABLE "+table+" (id integer PRIMARY KEY, value text NOT NULL)").Error)
	require.NoError(t, admin.Exec("INSERT INTO "+table+" VALUES (1, 'preserved')").Error)
	m := &managerImpl{opts: Options{CoreConfig: &coreconfig.Config{
		Deployment: coreconfig.DeploymentConfig{Env: coreconfig.DeploymentEnvDev}, Database: cfg,
	}}}
	section, err := m.buildDatabaseSection(pluginID)
	require.NoError(t, err)
	require.NotContains(t, section.DSN, "stale-host.invalid")
	require.NotContains(t, section.DSN, "stale_database_that_does_not_exist")
	pluginDB, err := database.Connect(corexdb.DatabaseConfig{Driver: "postgres", DSN: section.DSN, LogLevel: "silent"})
	require.NoError(t, err)
	sqlDB, err := pluginDB.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	var dbName, userName, searchPath, value string
	require.NoError(t, pluginDB.Raw("SELECT current_database()").Scan(&dbName).Error)
	require.NoError(t, pluginDB.Raw("SELECT current_user").Scan(&userName).Error)
	require.NoError(t, pluginDB.Raw("SHOW search_path").Scan(&searchPath).Error)
	require.Equal(t, actualDatabase, dbName)
	require.Equal(t, section.User, userName)
	require.NotEqual(t, actualUser, userName)
	require.True(t, strings.Contains(searchPath, schema))
	require.NoError(t, pluginDB.Raw("SELECT value FROM preserved_business_record WHERE id=1").Scan(&value).Error)
	require.Equal(t, "preserved", value)
	// 新插件 Role 能对自身已有表执行迁移，而不是只得到 DML 权限。
	require.NoError(t, pluginDB.Exec("ALTER TABLE preserved_business_record ADD COLUMN migration_check integer").Error)
}
