package agent

import (
	"context"
	"errors"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
)

var (
	ErrSessionScopeMismatch  = errors.New("agent.session_scope_mismatch")
	ErrSessionLegacyReadOnly = errors.New("agent.session_legacy_read_only")
)

// ValidateSessionScope 在任何写消息、截断重试或恢复执行之前校验固定归属。
func ValidateSessionScope(session *dbmodel.AgentChatSession, agentID uint64, scope dbmodel.ChatSessionScope, writable bool) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if session == nil || session.AgentID != agentID || session.Scope() != scope {
		return ErrSessionScopeMismatch
	}
	if writable && scope.SessionType == dbmodel.SessionTypeLegacy {
		return ErrSessionLegacyReadOnly
	}
	return nil
}

func (s *ChatHistoryService) validateNewSessionScope(ctx context.Context, tenantUUID *string, agentID uint64, scope dbmodel.ChatSessionScope) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if scope.SessionType == dbmodel.SessionTypeLegacy {
		return ErrSessionLegacyReadOnly
	}
	if scope.SessionType == dbmodel.SessionTypeTeam {
		if tenantUUID == nil {
			return ErrTeamInvalidTenant
		}
		team, err := NewTeamService(s.db).ValidateTeamTenant(ctx, scope.TeamID, *tenantUUID)
		if err != nil {
			return err
		}
		if team.ParentAgentID != agentID {
			return ErrSessionScopeMismatch
		}
	}
	return nil
}
