package repository

import (
	"context"
	"testing"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgentSkillBindingReplaceIsIdempotent(t *testing.T) {
	db := setupAgentSkillBindingDB(t)
	repo := NewAgentSkillBindingRepository(db)
	ctx := context.Background()
	tenant := "tenant-a"

	require.NoError(t, repo.Replace(ctx, "dev", &tenant, 8, []string{"powerxplugin.template.basic"}))
	require.NoError(t, repo.Replace(ctx, "dev", &tenant, 8, []string{"powerxplugin.template.basic"}))

	rows, err := repo.ListByAgent(ctx, "dev", &tenant, 8)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "powerxplugin.template.basic", rows[0].SkillID)
	require.True(t, rows[0].Enabled)
	require.Equal(t, 10, rows[0].Priority)
}

func TestAgentSkillBindingReplaceRestoresSoftDeletedBinding(t *testing.T) {
	db := setupAgentSkillBindingDB(t)
	repo := NewAgentSkillBindingRepository(db)
	ctx := context.Background()
	tenant := "tenant-a"

	require.NoError(t, repo.Replace(ctx, "dev", &tenant, 8, []string{"powerxplugin.template.basic"}))
	require.NoError(t, db.Where("env = ? AND tenant_uuid = ? AND agent_id = ? AND skill_id = ?", "dev", tenant, 8, "powerxplugin.template.basic").Delete(&dbmodel.AgentSkillBinding{}).Error)

	require.NoError(t, repo.Replace(ctx, "dev", &tenant, 8, []string{"powerxplugin.template.basic"}))

	var count int64
	require.NoError(t, db.Unscoped().Model(&dbmodel.AgentSkillBinding{}).
		Where("env = ? AND tenant_uuid = ? AND agent_id = ? AND skill_id = ?", "dev", tenant, 8, "powerxplugin.template.basic").
		Count(&count).Error)
	require.Equal(t, int64(1), count)

	rows, err := repo.ListByAgent(ctx, "dev", &tenant, 8)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].DeletedAt.Time.IsZero())
	require.True(t, rows[0].Enabled)
}

func setupAgentSkillBindingDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })
	require.NoError(t, db.AutoMigrate(&dbmodel.AgentSkillBinding{}))
	return db
}
