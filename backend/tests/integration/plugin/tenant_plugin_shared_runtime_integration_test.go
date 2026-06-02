package pluginintegration

import (
	"testing"

	pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	modelbase "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTenantPluginInstancesShareSingleGlobalRuntimeProcess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	dr := router.NewDynamicRouter("/_p", engine)
	sup := supervisor.New()
	mgr := pmimpl.New(pmimpl.Options{
		HTTP:       dr,
		Supervisor: sup,
	})

	dr.MountAPIProxy("com.powerx.plugins.shared", nil, "/api/v1", "/healthz")
	dr.MountAdminProxy("com.powerx.plugins.shared", nil)

	processes, ok := pmimpl.TryRuntimeProcesses(mgr, "com.powerx.plugins.shared")
	require.True(t, ok)
	require.Empty(t, processes)

	db := setupTenantPluginRuntimeDB(t)
	svc := pluginservice.NewTenantPluginInstanceServiceWithRepository(reposetting.NewPluginInstanceConfigRepository(db))
	plugin := plugin_mgr.Plugin{ID: "com.powerx.plugins.shared", Version: "0.1.0"}

	tenantA := "00000000-0000-0000-0000-00000000000a"
	tenantB := "00000000-0000-0000-0000-00000000000b"
	_, _, _, err := svc.Enable(t.Context(), tenantA, plugin, map[string]any{"plan": "a"})
	require.NoError(t, err)
	_, _, _, err = svc.Enable(t.Context(), tenantB, plugin, map[string]any{"plan": "b"})
	require.NoError(t, err)

	processes, ok = pmimpl.TryRuntimeProcesses(mgr, "com.powerx.plugins.shared")
	require.True(t, ok)
	require.Empty(t, processes, "tenant plugin instance enable must not create tenant-scoped runtime processes")

	count, err := svc.CountByPlugin(t.Context(), plugin.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
}

func setupTenantPluginRuntimeDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevSchema := modelbase.PowerXSchema
	modelbase.PowerXSchema = "main"
	t.Cleanup(func() {
		modelbase.PowerXSchema = prevSchema
	})
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&modelsetting.PluginInstanceConfig{}))
	return db
}
