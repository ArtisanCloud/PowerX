package agent

import (
	"context"
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
	env string, tenantID *uint64,
	agentID uint64, userID uint64,
	singleton bool,
	defaults dbmodel.AgentChatSession,
) (*dbmodel.AgentChatSession, bool, error) {
	out, err := s.sess.GetOrCreate(ctx, env, tenantID, agentID, userID, singleton, defaults)
	if err != nil {
		return nil, false, err
	}
	created := out.LatestAt != nil && out.CreatedAt.Equal(*out.LatestAt) // 仅供参考
	return out, created, nil
}

// Touch：更新最近时间并续期
func (s *AgentChatSessionService) Touch(ctx context.Context, env string, tenantID *uint64, id uint64) error {
	return s.sess.TouchLatest(ctx, env, tenantID, id, time.Now().UTC())
}

// TrimByPolicy：根据 MaxKB/MaxTokens 裁剪最旧消息（跳过 pinned）
func (s *AgentChatSessionService) TrimByPolicy(
	ctx context.Context, env string, tenantID *uint64, sess *dbmodel.AgentChatSession,
) error {
	if sess == nil {
		return nil
	}
	stats, err := s.msg.StatsBySession(ctx, env, tenantID, sess.ID)
	if err != nil {
		return err
	}
	limitBytes := int(sess.MaxKB) * 1024
	limitTokens := sess.MaxTokens
	if (limitBytes <= 0 || stats.TotalSize <= limitBytes) &&
		(limitTokens <= 0 || stats.TotalTokens <= limitTokens) {
		return nil
	}

	// 简易策略：按最旧开始删除非 pinned 直至低于阈值（可扩展为先摘要后删除）
	var deleted int64
	for i := 0; i < 10; i++ { // 最多 10 批
		items, e := s.msg.ListOldestN(ctx, env, tenantID, sess.ID, 200, true)
		if e != nil {
			return e
		}
		if len(items) == 0 {
			break
		}
		var ids []uint64
		var sz, tk int
		for _, it := range items {
			ids = append(ids, it.ID)
			sz += it.SizeBytes
			tk += it.Tokens
			stats.TotalSize -= it.SizeBytes
			stats.TotalTokens -= it.Tokens
			if (limitBytes > 0 && stats.TotalSize <= limitBytes) &&
				(limitTokens > 0 && stats.TotalTokens <= limitTokens) {
				break
			}
		}
		n, e := s.msg.DeleteByIDs(ctx, env, tenantID, ids)
		if e != nil {
			return e
		}
		deleted += n
		if (limitBytes <= 0 || stats.TotalSize <= limitBytes) &&
			(limitTokens <= 0 || stats.TotalTokens <= limitTokens) {
			break
		}
	}
	// TODO: 可在此触发摘要：把剩余上下文做 summary，写回 SetSummary(...)
	return nil
}
