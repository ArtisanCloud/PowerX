package agent

import (
	"context"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"gorm.io/gorm"
)

type AgentChatService struct {
	sessions *AgentChatSessionService
	msgs     *AgentChatMessageService
}

func NewAgentChatService(db *gorm.DB) *AgentChatService {
	return &AgentChatService{
		sessions: NewAgentChatSessionService(db),
		msgs:     NewAgentChatMessageService(db),
	}
}

// 打通最小闭环：确保会话 → 落用户消息 → 根据策略裁剪
func (s *AgentChatService) EnsureAndAppendUser(
	ctx context.Context,
	env string, tenantID *uint64,
	agentID uint64, userID uint64,
	singleton bool,
	defaults dbmodel.AgentChatSession,
	text string,
) (*dbmodel.AgentChatSession, *dbmodel.AgentChatMessage, error) {
	sess, _, err := s.sessions.Ensure(ctx, env, tenantID, agentID, userID, singleton, defaults)
	if err != nil {
		return nil, nil, err
	}
	msg, err := s.msgs.AppendUser(ctx, env, tenantID, sess.ID, text)
	if err != nil {
		return nil, nil, err
	}
	_ = s.sessions.Touch(ctx, env, tenantID, sess.ID)
	_ = s.sessions.TrimByPolicy(ctx, env, tenantID, sess)
	return sess, msg, nil
}
