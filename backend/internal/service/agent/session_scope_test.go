package agent

import (
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestValidateSessionScope(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stored  dbmodel.ChatSessionScope
		request dbmodel.ChatSessionScope
		agentID uint64
		write   bool
		want    error
	}{
		{"agent", dbmodel.ChatSessionScope{SessionType: "agent"}, dbmodel.ChatSessionScope{SessionType: "agent"}, 8, true, nil},
		{"team", dbmodel.ChatSessionScope{SessionType: "team", TeamID: 3}, dbmodel.ChatSessionScope{SessionType: "team", TeamID: 3}, 8, true, nil},
		{"team as agent", dbmodel.ChatSessionScope{SessionType: "team", TeamID: 3}, dbmodel.ChatSessionScope{SessionType: "agent"}, 8, true, ErrSessionScopeMismatch},
		{"agent as team", dbmodel.ChatSessionScope{SessionType: "agent"}, dbmodel.ChatSessionScope{SessionType: "team", TeamID: 3}, 8, true, ErrSessionScopeMismatch},
		{"another team same leader", dbmodel.ChatSessionScope{SessionType: "team", TeamID: 3}, dbmodel.ChatSessionScope{SessionType: "team", TeamID: 4}, 8, true, ErrSessionScopeMismatch},
		{"another agent", dbmodel.ChatSessionScope{SessionType: "agent"}, dbmodel.ChatSessionScope{SessionType: "agent"}, 9, true, ErrSessionScopeMismatch},
		{"legacy read", dbmodel.ChatSessionScope{SessionType: "legacy"}, dbmodel.ChatSessionScope{SessionType: "legacy"}, 8, false, nil},
		{"legacy write", dbmodel.ChatSessionScope{SessionType: "legacy"}, dbmodel.ChatSessionScope{SessionType: "legacy"}, 8, true, ErrSessionLegacyReadOnly},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := &dbmodel.AgentChatSession{AgentID: 8, SessionType: tc.stored.SessionType, TeamID: tc.stored.TeamID}
			require.ErrorIs(t, ValidateSessionScope(session, tc.agentID, tc.request, tc.write), tc.want)
		})
	}
}
