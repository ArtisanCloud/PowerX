package opsintegration

import (
	"context"
	"testing"

	deployops "github.com/ArtisanCloud/PowerX/internal/service/deploy_ops"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPluginLifecycleSwitchRollbackFlow(t *testing.T) {
	db := setupPluginLifecycleDB(t)
	svc := deployops.NewPluginLifecycleService(db)
	ctx := context.Background()

	switchAudit, err := svc.TriggerAction(ctx, deployops.PluginLifecycleActionRequest{
		PluginID:    "plugin.mediax",
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Action:      "switch",
		Reason:      "upgrade for hotfix",
		Operator:    "integration",
		TraceID:     "trace-plugin-switch",
	})
	require.NoError(t, err)
	require.Equal(t, modelops.PluginLifecycleActionSwitch, switchAudit.Action)
	require.Equal(t, modelops.PluginLifecycleResultSuccess, switchAudit.Result)

	rollbackAudit, err := svc.TriggerAction(ctx, deployops.PluginLifecycleActionRequest{
		PluginID:    "plugin.mediax",
		FromVersion: "1.1.0",
		ToVersion:   "1.0.0",
		Action:      "rollback",
		Reason:      "fallback validation",
		Operator:    "integration",
		TraceID:     "trace-plugin-rollback",
	})
	require.NoError(t, err)
	require.Equal(t, modelops.PluginLifecycleActionRollback, rollbackAudit.Action)
	require.Equal(t, modelops.PluginLifecycleResultSuccess, rollbackAudit.Result)

	items, total, err := svc.ListAudits(ctx, deployops.PluginLifecycleListOptions{PluginID: "plugin.mediax", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, items, 2)
	require.Equal(t, "plugin.mediax", items[0].PluginID)
}

func setupPluginLifecycleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(&modelops.PluginLifecycleAudit{}))
	return db
}
