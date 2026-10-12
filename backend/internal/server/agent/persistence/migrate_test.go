package persistence

import (
	"testing"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBackfillAgentChatMessageUUIDsAssignsStableUUIDOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	require.NoError(t, db.Exec(`CREATE TABLE public.agent_chat_messages (
		id integer primary key, uuid text, created_at datetime, updated_at datetime, deleted_at datetime,
		env text, tenant_uuid text, session_id integer not null, agent_id integer not null,
		role text, content text, content_type text, format text, tokens integer, size_bytes integer,
		pinned boolean, is_error boolean, meta json
	)`).Error)

	message := &dbmodel.AgentChatMessage{Env: "test", SessionID: 1, AgentID: 1, Role: "user", Content: "message", ContentType: "text"}
	require.NoError(t, db.Create(message).Error)
	require.NoError(t, db.Model(&dbmodel.AgentChatMessage{}).Where("id = ?", message.ID).Update("uuid", nil).Error)

	require.NoError(t, backfillAgentChatMessageUUIDs(db))
	var restored dbmodel.AgentChatMessage
	require.NoError(t, db.Where("id = ?", message.ID).First(&restored).Error)
	require.NotEmpty(t, restored.UUID.String())
	firstUUID := restored.UUID

	require.NoError(t, backfillAgentChatMessageUUIDs(db))
	require.NoError(t, db.Where("id = ?", message.ID).First(&restored).Error)
	require.Equal(t, firstUUID, restored.UUID)
}
