package agent

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

func TestSkillStateServiceLatestPendingTaskUsesStoredState(t *testing.T) {
	db := setupSkillStateServiceDB(t)
	svc := NewSkillStateService(db)
	ctx := context.Background()
	tenant := "tenant-a"

	_, err := svc.Upsert(ctx, SkillStateUpsertInput{
		Env:        "dev",
		TenantUUID: &tenant,
		SessionID:  81,
		AgentID:    18,
		SkillID:    "powerxplugin.template.basic",
		StateKey:   "template.create",
		Status:     "collecting",
		Action:     "create",
		State: datatypes.JSONMap{
			"collected": map[string]any{"template.title": "测试模板"},
			"missing":   []any{"template.description", "template.content"},
			"capability_request": map[string]any{
				"capability_id": "com.powerx.plugins.base.local.template.create",
			},
		},
		Meta: datatypes.JSONMap{"trace_id": "trace-1"},
	})
	require.NoError(t, err)

	task, ok, err := svc.LatestPendingTask(ctx, "dev", &tenant, 81, 18, []string{"powerxplugin.template.basic"})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "powerxplugin.template.basic", task["skill_id"])
	require.Equal(t, "template.create", task["state_key"])
	require.Equal(t, "create", task["action"])
	require.Equal(t, "com.powerx.plugins.base.local.template.create", task["capability_id"])
	require.Equal(t, "trace-1", task["trace_id"])
	require.Equal(t, []string{"template.description", "template.content"}, task["missing_fields"])
}

func TestSkillStateServiceIgnoresCompletedState(t *testing.T) {
	db := setupSkillStateServiceDB(t)
	svc := NewSkillStateService(db)
	ctx := context.Background()
	tenant := "tenant-a"

	_, err := svc.Upsert(ctx, SkillStateUpsertInput{
		Env:        "dev",
		TenantUUID: &tenant,
		SessionID:  81,
		AgentID:    18,
		SkillID:    "powerxplugin.template.basic",
		StateKey:   "template.create",
		Status:     "completed",
	})
	require.NoError(t, err)

	_, ok, err := svc.LatestPendingTask(ctx, "dev", &tenant, 81, 18, []string{"powerxplugin.template.basic"})
	require.NoError(t, err)
	require.False(t, ok)
}

func setupSkillStateServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })
	require.NoError(t, db.AutoMigrate(&dbmodel.AgentSessionSkillState{}))
	return db
}
