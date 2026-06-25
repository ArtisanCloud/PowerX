package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAgentChatSessionFindByUUID(t *testing.T) {
	db := setupAgentChatSessionDB(t)
	repo := NewAgentChatSessionRepository(db)
	ctx := context.Background()
	tenant := "tenant-a"
	sessionUUID := uuid.New()

	session := dbmodel.AgentChatSession{
		UUID:       sessionUUID,
		Env:        "dev",
		TenantUUID: &tenant,
		AgentID:    8,
		UserID:     1,
		Title:      "find me",
		Status:     "active",
	}
	require.NoError(t, db.Create(&session).Error)

	found, err := repo.FindByUUID(ctx, "dev", &tenant, sessionUUID.String())
	require.NoError(t, err)
	require.Equal(t, session.ID, found.ID)
	require.Equal(t, sessionUUID, found.UUID)
}

func TestAgentChatSessionDeleteSoftHidesActiveSession(t *testing.T) {
	db := setupAgentChatSessionDB(t)
	repo := NewAgentChatSessionRepository(db)
	ctx := context.Background()
	tenant := "tenant-a"
	now := time.Now().UTC()

	session := dbmodel.AgentChatSession{
		Env:        "dev",
		TenantUUID: &tenant,
		AgentID:    8,
		UserID:     1,
		Title:      "delete me",
		Status:     "active",
		LatestAt:   &now,
	}
	require.NoError(t, db.Create(&session).Error)

	require.NoError(t, repo.DeleteSoft(ctx, "dev", &tenant, session.ID))

	active, err := repo.ListByAgent(ctx, "dev", &tenant, 8, []string{"active"}, 10, 0)
	require.NoError(t, err)
	require.Empty(t, active)

	var deleted dbmodel.AgentChatSession
	require.NoError(t, db.Unscoped().First(&deleted, session.ID).Error)
	require.Equal(t, "deleted", deleted.Status)
	require.True(t, deleted.DeletedAt.Valid)
}

func setupAgentChatSessionDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared&parseTime=true&_loc=UTC", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })
	require.NoError(t, db.Exec(`
		CREATE TABLE agent_chat_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			env TEXT,
			tenant_uuid TEXT,
			agent_id INTEGER NOT NULL,
			user_id INTEGER,
			title TEXT,
			singleton BOOLEAN,
			ttl_days INTEGER,
			max_kb INTEGER,
			max_tokens INTEGER,
			summary TEXT,
			summary_at DATETIME,
			status TEXT,
			latest_at DATETIME,
			expired_at DATETIME,
			meta TEXT
		)
	`).Error)
	return db
}
