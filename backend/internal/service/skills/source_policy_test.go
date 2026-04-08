package skills

import (
	"context"
	"encoding/json"
	"testing"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDBSourcePolicyResolver_ContextOverrideHighestPriority(t *testing.T) {
	db := setupSourcePolicyDB(t)
	resolver := NewDBSourcePolicyResolver(db)

	require.NoError(t, db.Create(&settingmodel.TenantSetting{
		TenantUUID: "tenant-a",
		Key:        TenantSettingKeySkillSourceAllowlist,
		ValueJSON:  datatypes.JSON([]byte(`["builtin"]`)),
		Group:      "ai",
		Editable:   true,
	}).Error)

	sources := resolver.ResolveAllowedSources(context.Background(), SourcePolicyInput{
		TenantUUID: "tenant-a",
		Context: map[string]interface{}{
			"skill_source_allowlist": []interface{}{"plugin", "third_party"},
		},
	})
	require.Equal(t, []string{"plugin", "third_party"}, sources)
}

func TestDBSourcePolicyResolver_AgentPolicyOverridesTenant(t *testing.T) {
	db := setupSourcePolicyDB(t)
	resolver := NewDBSourcePolicyResolver(db)

	require.NoError(t, db.Create(&settingmodel.TenantSetting{
		TenantUUID: "tenant-b",
		Key:        TenantSettingKeySkillSourceAllowlist,
		ValueJSON:  datatypes.JSON([]byte(`["builtin"]`)),
		Group:      "ai",
		Editable:   true,
	}).Error)

	quotaRaw, _ := json.Marshal(map[string]interface{}{
		"skill_source_allowlist": []string{"plugin"},
	})
	var quota datatypes.JSONMap
	require.NoError(t, json.Unmarshal(quotaRaw, &quota))

	tenant := "tenant-b"
	require.NoError(t, db.Create(&agentmodel.AgentSetting{
		Env:         "default",
		TenantUUID:  &tenant,
		AgentID:     99,
		QuotaPolicy: quota,
	}).Error)

	sources := resolver.ResolveAllowedSources(context.Background(), SourcePolicyInput{
		TenantUUID: "tenant-b",
		Env:        "default",
		AgentID:    99,
	})
	require.Equal(t, []string{"plugin"}, sources)
}

func TestDBSourcePolicyResolver_DefaultFallback(t *testing.T) {
	db := setupSourcePolicyDB(t)
	resolver := NewDBSourcePolicyResolver(db)

	sources := resolver.ResolveAllowedSources(context.Background(), SourcePolicyInput{
		TenantUUID: "tenant-c",
	})
	require.Equal(t, []string{"builtin", "plugin", "third_party"}, sources)
}

func setupSourcePolicyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&settingmodel.TenantSetting{},
	))
	require.NoError(t, db.Exec(`
		CREATE TABLE IF NOT EXISTS main.agent_settings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			env TEXT,
			tenant_uuid TEXT,
			agent_id INTEGER,
			provider TEXT,
			model TEXT,
			params JSON,
			override_flags JSON,
			quota_policy JSON,
			health_status TEXT,
			health_info JSON
		)
	`).Error)
	return db
}
