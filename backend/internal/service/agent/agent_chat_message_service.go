package agent

import (
	"context"
	"unicode/utf8"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"gorm.io/gorm"
)

type AgentChatMessageService struct {
	db  *gorm.DB
	msg *repo.AgentChatMessageRepository
}

func NewAgentChatMessageService(db *gorm.DB) *AgentChatMessageService {
	return &AgentChatMessageService{
		db:  db,
		msg: repo.NewAgentChatMessageRepository(db),
	}
}

func (s *AgentChatMessageService) AppendUser(
	ctx context.Context, env string, tenantID *uint64, sessionID uint64, text string,
) (*dbmodel.AgentChatMessage, error) {
	m := &dbmodel.AgentChatMessage{
		Env:       env,
		TenantID:  tenantID,
		SessionID: sessionID,
		Role:      "user",
		Content:   text,
	}
	fillSizing(m)
	if err := s.msg.Append(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *AgentChatMessageService) AppendAssistant(
	ctx context.Context, env string, tenantID *uint64, sessionID uint64, text string, meta map[string]any,
) (*dbmodel.AgentChatMessage, error) {
	m := &dbmodel.AgentChatMessage{
		Env:       env,
		TenantID:  tenantID,
		SessionID: sessionID,
		Role:      "assistant",
		Content:   text,
		Meta:      meta,
	}
	fillSizing(m)
	if err := s.msg.Append(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// ===== helpers =====

// 简易统计：按 UTF-8 字节与近似 tokens（4 字符 ≈ 1 token）
func fillSizing(m *dbmodel.AgentChatMessage) {
	if m == nil {
		return
	}
	b := []byte(m.Content)
	m.SizeBytes = len(b)
	runes := utf8.RuneCount(b)
	if runes <= 0 {
		m.Tokens = 0
	} else {
		m.Tokens = ((runes + 3) / 4) // ceil
	}
}
