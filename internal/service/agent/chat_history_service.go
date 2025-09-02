package agent

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"gorm.io/gorm"
)

type ChatHistoryService struct {
	db       *gorm.DB
	sessRepo *repo.AgentChatSessionRepository
	msgRepo  *repo.AgentChatMessageRepository
}

func NewChatHistoryService(db *gorm.DB) *ChatHistoryService {
	return &ChatHistoryService{
		db:       db,
		sessRepo: repo.NewAgentChatSessionRepository(db),
		msgRepo:  repo.NewAgentChatMessageRepository(db),
	}
}

// GetOrCreateSession: 单例则按 agent/env/tenant 复用，否则创建新会话
func (s *ChatHistoryService) GetOrCreateSession(
	ctx context.Context,
	env string, tenantID *uint64,
	agentID uint64, userID string,
	singleton bool,
	defaults dbmodel.AgentChatSession,
) (*dbmodel.AgentChatSession, error) {
	return s.sessRepo.GetOrCreate(ctx, env, tenantID, agentID, userID, singleton, defaults)
}

func (s *ChatHistoryService) FindSessionByID(ctx context.Context, env string, tenantID *uint64, id uint64) (*dbmodel.AgentChatSession, error) {
	return s.sessRepo.FindByID(ctx, env, tenantID, id)
}

func (s *ChatHistoryService) ListSessions(
	ctx context.Context, env string, tenantID *uint64,
	agentID uint64, statuses []string, limit, offset int,
) ([]dbmodel.AgentChatSession, error) {
	return s.sessRepo.ListByAgent(ctx, env, tenantID, agentID, statuses, limit, offset)
}

func (s *ChatHistoryService) UpdateSessionPolicy(
	ctx context.Context, env string, tenantID *uint64, id uint64,
	title *string, ttlDays, maxKB, maxTokens *int,
) error {
	if title != nil {
		if err := s.db.WithContext(ctx).
			Model(&dbmodel.AgentChatSession{}).
			Scopes(dbmodel.WithScope(env, tenantID)).
			Where("id = ?", id).
			Update("title", *title).Error; err != nil {
			return err
		}
	}
	var ttl, kb, toks int
	if ttlDays != nil {
		ttl = *ttlDays
	}
	if maxKB != nil {
		kb = *maxKB
	}
	if maxTokens != nil {
		toks = *maxTokens
	}
	if ttl > 0 || kb > 0 || toks > 0 {
		if err := s.sessRepo.UpdatePolicy(ctx, env, tenantID, id, ttl, kb, toks); err != nil {
			return err
		}
	}
	return nil
}

func (s *ChatHistoryService) ArchiveSession(ctx context.Context, env string, tenantID *uint64, id uint64) error {
	return s.sessRepo.Archive(ctx, env, tenantID, id)
}

func (s *ChatHistoryService) DeleteSession(ctx context.Context, id uint64) error {
	return s.sessRepo.DeleteSoft(ctx, id)
}

func (s *ChatHistoryService) ListMessages(
	ctx context.Context, env string, tenantID *uint64,
	sessionID uint64, afterID uint64, limit int,
) ([]dbmodel.AgentChatMessage, error) {
	return s.msgRepo.ListBySession(ctx, env, tenantID, sessionID, afterID, limit)
}

func (s *ChatHistoryService) AppendMessage(
	ctx context.Context, env string, tenantID *uint64,
	sessionID, agentID uint64,
	role, content, format string,
	tokens, sizeBytes int,
	pinned bool,
	meta map[string]any,
) (*dbmodel.AgentChatMessage, error) {
	if role == "" {
		return nil, errors.New("role required")
	}
	if format == "" {
		format = "text"
	}
	if sizeBytes <= 0 {
		sizeBytes = len([]byte(content))
	}
	if tokens <= 0 {
		// 简单估算：4 字节≈1 token（粗略），避免引入额外依赖；后续可替换为真 tokenizer
		tokens = utf8.RuneCountInString(content) / 4
	}
	msg := &dbmodel.AgentChatMessage{
		Env:       env,
		TenantID:  tenantID,
		SessionID: sessionID,
		AgentID:   agentID,
		Role:      role,
		Content:   content,
		Format:    format,
		Tokens:    tokens,
		SizeBytes: sizeBytes,
		Pinned:    pinned,
		Meta:      meta,
	}
	if err := s.msgRepo.Append(ctx, msg); err != nil {
		return nil, err
	}
	_ = s.sessRepo.TouchLatest(ctx, env, tenantID, sessionID, time.Now())
	return msg, nil
}

// AppendPair：一轮对话写入（user+assistant）
func (s *ChatHistoryService) AppendPair(
	ctx context.Context, env string, tenantID *uint64,
	sessionID, agentID uint64,
	userText, assistantText string,
) error {
	list := make([]dbmodel.AgentChatMessage, 0, 2)
	if userText != "" {
		list = append(list, dbmodel.AgentChatMessage{
			Env: env, TenantID: tenantID, SessionID: sessionID, AgentID: agentID,
			Role: "user", Content: userText, Format: "text",
			Tokens: utf8.RuneCountInString(userText) / 4, SizeBytes: len([]byte(userText)),
		})
	}
	if assistantText != "" {
		list = append(list, dbmodel.AgentChatMessage{
			Env: env, TenantID: tenantID, SessionID: sessionID, AgentID: agentID,
			Role: "assistant", Content: assistantText, Format: "text",
			Tokens: utf8.RuneCountInString(assistantText) / 4, SizeBytes: len([]byte(assistantText)),
		})
	}
	if len(list) == 0 {
		return nil
	}
	if err := s.msgRepo.BatchAppend(ctx, list); err != nil {
		return err
	}
	return s.sessRepo.TouchLatest(ctx, env, tenantID, sessionID, time.Now())
}

// Stats & 简易摘要触发（可选）
func (s *ChatHistoryService) SummarizeIfNeeded(
	ctx context.Context, env string, tenantID *uint64, session *dbmodel.AgentChatSession,
) (bool, error) {
	stats, err := s.msgRepo.StatsBySession(ctx, env, tenantID, session.ID)
	if err != nil {
		return false, err
	}
	if (session.MaxTokens > 0 && stats.TotalTokens > int64(session.MaxTokens)) ||
		(session.MaxKB > 0 && stats.TotalSize > int64(session.MaxKB*1024)) {

		// 极简“摘要”：取最近 N 条的前 2k 字符拼接（占位，后续可接入 LLM 摘要）
		items, err := s.msgRepo.ListLatestN(ctx, env, tenantID, session.ID, 40)
		if err != nil {
			return false, err
		}
		var buf []rune
		for _, it := range items {
			if it.Role == "summary" || it.Pinned {
				continue
			}
			r := []rune(it.Content)
			buf = append(buf, r...)
			if len(buf) > 2000 {
				buf = buf[:2000]
				break
			}
		}
		if err := s.sessRepo.SetSummary(ctx, env, tenantID, session.ID, string(buf)); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
