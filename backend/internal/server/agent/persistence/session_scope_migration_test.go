package persistence

import (
	"encoding/json"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func legacyTurn(userID uint64, team string, declared bool) []dbmodel.AgentChatMessage {
	task := map[string]any{"message_id": userID, "agent_id": "8"}
	if declared {
		task["team_id"] = team
	}
	return []dbmodel.AgentChatMessage{
		{PowerUUIDModel: coremodel.PowerUUIDModel{ID: userID}, Role: "user"},
		{Role: "assistant", Meta: datatypes.JSONMap{"run_state": map[string]any{"tasks": []any{task}}}},
	}
}

func TestSessionScopeMigrationIsAdditiveAndIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	for _, ddl := range []string{
		`CREATE TABLE main.agent_chat_sessions (id INTEGER PRIMARY KEY, env TEXT, tenant_uuid TEXT, agent_id INTEGER, summary TEXT, summary_at DATETIME, deleted_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE main.agent_chat_messages (id INTEGER PRIMARY KEY, env TEXT, tenant_uuid TEXT, session_id INTEGER, role TEXT, meta TEXT, content TEXT, deleted_at DATETIME)`,
		`CREATE TABLE main.agent_teams (id INTEGER PRIMARY KEY, tenant_uuid TEXT, parent_agent_id INTEGER, deleted_at DATETIME)`,
	} {
		require.NoError(t, db.Exec(ddl).Error)
	}
	require.NoError(t, db.Exec(`INSERT INTO main.agent_chat_sessions(id,env,tenant_uuid,agent_id) VALUES(1,'dev','tenant-a',8),(2,'dev','tenant-a',8),(3,'dev','tenant-a',8),(4,'dev','tenant-a',8)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO main.agent_teams(id,tenant_uuid,parent_agent_id) VALUES(3,'tenant-a',8)`).Error)
	require.NoError(t, db.Migrator().AddColumn(&dbmodel.AgentChatSession{}, "SessionType"))
	require.NoError(t, db.Migrator().AddColumn(&dbmodel.AgentChatSession{}, "TeamID"))
	for i, messages := range [][]dbmodel.AgentChatMessage{legacyTurn(1, "", true), legacyTurn(3, "3", true), legacyTurn(5, "", false), legacyTurn(7, "9", true)} {
		for j, message := range messages {
			meta, err := json.Marshal(message.Meta)
			require.NoError(t, err)
			require.NoError(t, db.Exec(`INSERT INTO main.agent_chat_messages(id,env,tenant_uuid,session_id,role,meta,content) VALUES(?,'dev','tenant-a',?,?,?,'preserved')`, i*2+j+1, i+1, message.Role, string(meta)).Error)
		}
	}
	for i := 0; i < 2; i++ {
		require.NoError(t, backfillAgentSessionScopes(db))
		var sessions []dbmodel.AgentChatSession
		require.NoError(t, db.Order("id").Find(&sessions).Error)
		require.Equal(t, "agent", sessions[0].SessionType)
		require.Equal(t, dbmodel.ChatSessionScope{SessionType: "team", TeamID: 3}, sessions[1].Scope())
		require.Equal(t, "legacy", sessions[2].SessionType)
		require.Equal(t, "legacy", sessions[3].SessionType)
		var count int64
		require.NoError(t, db.Model(&dbmodel.AgentChatMessage{}).Where("content = ?", "preserved").Count(&count).Error)
		require.EqualValues(t, 8, count)
	}
}

func TestInferLegacySessionScopeRequiresCompleteConsistentEvidence(t *testing.T) {
	session := dbmodel.AgentChatSession{AgentID: 8}
	for _, tc := range []struct {
		name     string
		messages []dbmodel.AgentChatMessage
		want     dbmodel.ChatSessionScope
	}{
		{"empty", nil, dbmodel.ChatSessionScope{SessionType: "legacy"}},
		{"explicit standalone", legacyTurn(1, "", true), dbmodel.ChatSessionScope{SessionType: "agent"}},
		{"team", legacyTurn(1, "3", true), dbmodel.ChatSessionScope{SessionType: "team", TeamID: 3}},
		{"missing team field is ambiguous", legacyTurn(1, "", false), dbmodel.ChatSessionScope{SessionType: "legacy"}},
		{"mixed modes", append(legacyTurn(1, "3", true), legacyTurn(3, "", true)...), dbmodel.ChatSessionScope{SessionType: "legacy"}},
		{"multiple teams", append(legacyTurn(1, "3", true), legacyTurn(3, "4", true)...), dbmodel.ChatSessionScope{SessionType: "legacy"}},
		{"unfinished turn", append(legacyTurn(1, "3", true), legacyTurn(3, "3", true)[0]), dbmodel.ChatSessionScope{SessionType: "legacy"}},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.want, inferLegacySessionScope(session, tc.messages)) })
	}
	session.Summary = "old compressed context"
	require.Equal(t, "legacy", inferLegacySessionScope(session, legacyTurn(1, "3", true)).SessionType)
}
