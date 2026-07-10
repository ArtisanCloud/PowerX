package agent

import (
	"context"
	"testing"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestReplacePluginRegistryGrantsFromSkillsCreatesEnabledGrants(t *testing.T) {
	db := setupPluginGrantDB(t)
	svc := NewAgentService(db)
	ctx := context.Background()
	tenantUUID := "tenant-a"
	agentUUID := uuid.New()
	capabilityUUID := uuid.New()

	require.NoError(t, db.Exec(`
		INSERT INTO skills_capability_bindings (skill_id, capability_id, binding_status)
		VALUES (?, ?, ?)
	`, "powerxplugin.template.basic.local", "com.powerx.plugins.base.local.template.create", "active").Error)
	require.NoError(t, db.Create(&capmodels.CapabilityRecord{
		UUID:         capabilityUUID,
		CapabilityID: "com.powerx.plugins.base.local.template.create",
		PluginID:     "com.powerx.plugins.base.local",
		Title:        "Template Create",
		ToolScope:    datatypes.JSON([]byte(`["com.powerx.plugins.base.local.template:create"]`)),
		Annotations:  datatypes.JSON([]byte(`{"permission_codes":["com.powerx.plugins.base.local.template:create"],"risk_level":"low"}`)),
		Status:       "published",
	}).Error)

	err := svc.ReplacePluginRegistryGrantsFromSkills(ctx, "dev", &tenantUUID, agentUUID, []string{"powerxplugin.template.basic.local"}, "user-a")
	require.NoError(t, err)

	var rows []dbmodel.AgentCapabilityGrant
	require.NoError(t, db.Where("agent_uuid = ?", agentUUID).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, capabilityUUID, rows[0].CapabilityUUID)
	require.Equal(t, "com.powerx.plugins.base.local.template.create", rows[0].CapabilityID)
	require.Equal(t, "com.powerx.plugins.base.local.template:create", rows[0].PermissionCode)
	require.Equal(t, dbmodel.AgentCapabilityGrantStatusEnabled, rows[0].Status)
	require.Equal(t, dbmodel.AgentCapabilityGrantSourcePlugin, rows[0].Source)
	require.Equal(t, "low", rows[0].RiskLevel)
}

func setupPluginGrantDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })
	require.NoError(t, db.AutoMigrate(&dbmodel.AgentCapabilityGrant{}, &capmodels.CapabilityRecord{}))
	require.NoError(t, db.Exec(`
		CREATE TABLE skills_capability_bindings (
			id integer primary key autoincrement,
			skill_id text not null,
			capability_id text not null,
			binding_status text not null
		)
	`).Error)
	return db
}
