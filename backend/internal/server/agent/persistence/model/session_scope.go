package model

import "fmt"

const (
	SessionTypeAgent  = "agent"
	SessionTypeTeam   = "team"
	SessionTypeLegacy = "legacy"
)

// ChatSessionScope 是持久化的会话归属，不从负责人或页面名称推断团队。
type ChatSessionScope struct {
	SessionType string
	TeamID      uint64
}

func (s ChatSessionScope) Validate() error {
	switch s.SessionType {
	case SessionTypeAgent, SessionTypeLegacy:
		if s.TeamID == 0 {
			return nil
		}
	case SessionTypeTeam:
		if s.TeamID > 0 {
			return nil
		}
	}
	return fmt.Errorf("agent.session_scope_invalid")
}

func (s *AgentChatSession) Scope() ChatSessionScope {
	return ChatSessionScope{SessionType: s.SessionType, TeamID: s.TeamID}
}
