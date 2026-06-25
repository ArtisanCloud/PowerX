package agent

import (
	"context"
	"errors"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"gorm.io/gorm"
)

type AgentChatSessionService struct {
	db   *gorm.DB
	sess *repo.AgentChatSessionRepository
	msg  *repo.AgentChatMessageRepository
}

func NewAgentChatSessionService(db *gorm.DB) *AgentChatSessionService {
	return &AgentChatSessionService{
		db:   db,
		sess: repo.NewAgentChatSessionRepository(db),
		msg:  repo.NewAgentChatMessageRepository(db),
	}
}

// Ensure：按 singleton 策略获取或创建
func (s *AgentChatSessionService) Ensure(
	ctx context.Context,
	env string, tenantUUID *string,
	agentID uint64, userID uint64,
	singleton bool,
	defaults dbmodel.AgentChatSession,
) (*dbmodel.AgentChatSession, bool, error) {
	out, err := s.sess.GetOrCreate(ctx, env, tenantUUID, agentID, userID, singleton, defaults)
	if err != nil {
		return nil, false, err
	}
	created := out.LatestAt != nil && out.CreatedAt.Equal(*out.LatestAt) // 仅供参考
	return out, created, nil
}

// Touch：更新最近时间并续期
func (s *AgentChatSessionService) Touch(ctx context.Context, env string, tenantUUID *string, id uint64) error {
	return s.sess.TouchLatest(ctx, env, tenantUUID, id, time.Now().UTC())
}

// TrimByPolicy：根据 MaxKB/MaxTokens 裁剪最旧消息（跳过 pinned）
func (s *AgentChatSessionService) TrimByPolicy(
	ctx context.Context, env string, tenantUUID *string, sess *dbmodel.AgentChatSession,
) error {
	if sess == nil {
		return nil
	}
	stats, err := s.msg.StatsBySession(ctx, env, tenantUUID, sess.ID)
	if err != nil {
		return err
	}
	limitBytes := int(sess.MaxKB) * 1024
	limitTokens := sess.MaxTokens
	if (limitBytes <= 0 || stats.TotalSize <= limitBytes) &&
		(limitTokens <= 0 || stats.TotalTokens <= limitTokens) {
		return nil
	}

	history := NewChatHistoryService(s.db)
	res, err := history.RollingCompressIfNeeded(ctx, env, tenantUUID, sess, RollingContextCompressionPolicy{
		RecentMessages: 20,
		MaxMessages:    500,
		DeleteCovered:  true,
	})
	if err != nil {
		return err
	}
	if res == nil || !res.Compressed {
		return errors.New("agent chat session exceeds context policy but no compressible messages are available")
	}
	return nil
}
