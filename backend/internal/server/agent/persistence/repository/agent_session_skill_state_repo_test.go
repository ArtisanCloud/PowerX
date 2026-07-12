package repository

import (
	"context"
	"testing"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgentSessionSkillStateUpsertIsIdempotentAndVersions(t *testing.T) {
	db := setupAgentSessionSkillStateDB(t)
	repo := NewAgentSessionSkillStateRepository(db)
	ctx := context.Background()
	tenant := "tenant-a"

	first, err := repo.Upsert(ctx, SkillStateUpsert{
		Env:        "dev",
		TenantUUID: &tenant,
		SessionID:  81,
		AgentID:    18,
		SkillID:    "powerxplugin.template.basic",
		StateKey:   "template.create",
		Status:     "collecting",
		State: datatypes.JSONMap{
			"collected": map[string]any{"template.title": "测试模板"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), first.Version)

	second, err := repo.Upsert(ctx, SkillStateUpsert{
		Env:        "dev",
		TenantUUID: &tenant,
		SessionID:  81,
		AgentID:    18,
		SkillID:    "powerxplugin.template.basic",
		StateKey:   "template.create",
		Status:     "ready",
		State: datatypes.JSONMap{
			"collected": map[string]any{
				"template.title":       "测试模板",
				"template.description": "用于测试",
				"template.content":     "内容",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), second.Version)
	require.Equal(t, "ready", second.Status)

	var count int64
	require.NoError(t, db.Model(&dbmodel.AgentSessionSkillState{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestAgentSessionSkillStateLatestBySessionFiltersBoundSkills(t *testing.T) {
	db := setupAgentSessionSkillStateDB(t)
	repo := NewAgentSessionSkillStateRepository(db)
	ctx := context.Background()
	tenant := "tenant-a"

	_, err := repo.Upsert(ctx, SkillStateUpsert{
		Env:        "dev",
		TenantUUID: &tenant,
		SessionID:  81,
		AgentID:    18,
		SkillID:    "other.skill",
		StateKey:   "other.run",
		Status:     "collecting",
	})
	require.NoError(t, err)
	_, err = repo.Upsert(ctx, SkillStateUpsert{
		Env:        "dev",
		TenantUUID: &tenant,
		SessionID:  81,
		AgentID:    18,
		SkillID:    "powerxplugin.template.basic",
		StateKey:   "template.create",
		Status:     "awaiting_confirmation",
	})
	require.NoError(t, err)

	latest, err := repo.LatestBySession(ctx, "dev", &tenant, 81, 18, []string{"powerxplugin.template.basic"})
	require.NoError(t, err)
	require.Equal(t, "powerxplugin.template.basic", latest.SkillID)
	require.Equal(t, "template.create", latest.StateKey)
}

func setupAgentSessionSkillStateDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })
	require.NoError(t, db.AutoMigrate(&dbmodel.AgentSessionSkillState{}))
	return db
}
