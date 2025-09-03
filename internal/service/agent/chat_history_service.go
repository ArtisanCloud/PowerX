// internal/service/agent/chat_history_service.go
package agent

import (
	"context"
	"errors"
	"strings"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ChatHistoryService struct {
	db   *gorm.DB
	sess *repo.AgentChatSessionRepository
	msg  *repo.AgentChatMessageRepository
}

func NewChatHistoryService(db *gorm.DB) *ChatHistoryService {
	return &ChatHistoryService{
		db:   db,
		sess: repo.NewAgentChatSessionRepository(db),
		msg:  repo.NewAgentChatMessageRepository(db),
	}
}

// ---- 默认策略（可与前端一致）----
const (
	defaultTTLDays   = 3
	defaultMaxKB     = 200
	defaultMaxTokens = 3000
)

func (s *ChatHistoryService) defaultPolicy() dbmodel.AgentChatSession {
	return dbmodel.AgentChatSession{
		TTLDays:   defaultTTLDays,
		MaxKB:     defaultMaxKB,
		MaxTokens: defaultMaxTokens,
	}
}

// 将缺省值填充到已有对象上；nil 时安全返回（不做任何写入）
func (s *ChatHistoryService) ensureDefaults(in *dbmodel.AgentChatSession) {
	if in == nil {
		// 如果你希望在传 nil 时也应用默认值，可以在调用方用：
		//   def := s.defaultPolicy()
		//   in = &def
		return
	}
	if in.TTLDays == 0 {
		in.TTLDays = defaultTTLDays
	}
	if in.MaxKB == 0 {
		in.MaxKB = defaultMaxKB
	}
	if in.MaxTokens == 0 {
		in.MaxTokens = defaultMaxTokens
	}
}

// GetOrCreateSession：按 (env/tenant/agentID/userID) 获取或创建；支持 singleton
func (s *ChatHistoryService) GetOrCreateSession(
	ctx context.Context, env string, tenantID *uint64,
	agentID uint64, userID uint64, singleton bool,
	defaults *dbmodel.AgentChatSession,
) (*dbmodel.AgentChatSession, error) {

	// 安全拿到一份“值类型”的默认策略
	def := s.defaultPolicy()
	if defaults != nil {
		def = *defaults
		s.ensureDefaults(&def)
	}

	out, err := s.sess.GetOrCreate(ctx, env, tenantID, agentID, userID, singleton, def)
	if err != nil {
		return nil, err
	}

	// 触发一次最近活跃续期
	_ = s.sess.TouchLatest(ctx, env, tenantID, out.ID, time.Now().UTC())
	return out, nil
}

// FindSessionByID：带作用域读取
func (s *ChatHistoryService) FindSessionByID(
	ctx context.Context, env string, tenantID *uint64, id uint64,
) (*dbmodel.AgentChatSession, error) {
	return s.sess.FindByID(ctx, env, tenantID, id)
}

// ListSessions：按 Agent 维度分页查询（statuses 可为空）
func (s *ChatHistoryService) ListSessions(
	ctx context.Context, env string, tenantID *uint64,
	agentID uint64, statuses []string,
	limit, offset int,
) ([]dbmodel.AgentChatSession, error) {
	return s.sess.ListByAgent(ctx, env, tenantID, agentID, statuses, limit, offset)
}

// UpdateSessionPolicy：可选更新 title / TTL / MaxKB / MaxTokens（部分字段更新）
// 注意：repo 里没有改 title 的方法，这里用 gorm 直接更新 title
func (s *ChatHistoryService) UpdateSessionPolicy(
	ctx context.Context, env string, tenantID *uint64, id uint64,
	title *string, ttlDays, maxKB, maxTokens *int,
) error {
	// 1) title
	if title != nil {
		err := s.db.WithContext(ctx).
			Model(&dbmodel.AgentChatSession{}).
			Scopes(dbmodel.WithScope(env, tenantID)).
			Where("id = ?", id).
			Updates(map[string]any{
				"title":      strings.TrimSpace(*title),
				"updated_at": time.Now().UTC(),
			}).Error
		if err != nil {
			return err
		}
	}
	// 2) 策略（允许只给部分）
	var t, kb, tk int
	if ttlDays != nil {
		t = *ttlDays
	}
	if maxKB != nil {
		kb = *maxKB
	}
	if maxTokens != nil {
		tk = *maxTokens
	}
	if ttlDays != nil || maxKB != nil || maxTokens != nil {
		if err := s.sess.UpdatePolicy(ctx, env, tenantID, id, t, kb, tk); err != nil {
			return err
		}
		// 更新策略后，顺带续期
		_ = s.sess.TouchLatest(ctx, env, tenantID, id, time.Now().UTC())
	}
	return nil
}

// ArchiveSession：归档（软删除前常用）
func (s *ChatHistoryService) ArchiveSession(
	ctx context.Context, env string, tenantID *uint64, id uint64,
) error {
	return s.sess.Archive(ctx, env, tenantID, id)
}

// DeleteSession：先删消息再软删会话
func (s *ChatHistoryService) DeleteSession(
	ctx context.Context, env string, tenantID *uint64, id uint64,
) error {
	if err := s.msg.DeleteBySession(ctx, env, tenantID, id); err != nil {
		return err
	}
	return s.sess.DeleteSoft(ctx, id)
}

// ListMessages：会话内分页拉取消息（支持 afterID 游标）
func (s *ChatHistoryService) ListMessages(
	ctx context.Context, env string, tenantID *uint64,
	sessionID uint64, afterID uint64, limit int,
) ([]dbmodel.AgentChatMessage, error) {
	return s.msg.ListBySession(ctx, env, tenantID, sessionID, afterID, limit)
}

// AppendMessage：追加一条消息，并刷新会话“最近活跃/过期时间”
func (s *ChatHistoryService) AppendMessage(
	ctx context.Context, env string, tenantID *uint64,
	sessionID, agentID uint64,
	role, content, contentType string,
	tokensIn, tokensOut int,
	isError bool, meta datatypes.JSONMap,
) (*dbmodel.AgentChatMessage, error) {
	if contentType == "" {
		contentType = "text/plain"
	}
	if meta == nil {
		meta = datatypes.JSONMap{}
	}
	tokens := tokensIn + tokensOut
	m := &dbmodel.AgentChatMessage{
		Env:         env,
		TenantID:    tenantID,
		SessionID:   sessionID,
		AgentID:     agentID,
		Role:        role,
		Content:     content,
		ContentType: contentType,
		Tokens:      tokens,
		SizeBytes:   len([]byte(content)),
		IsError:     isError,
		Meta:        meta,
	}
	if err := s.msg.Append(ctx, m); err != nil {
		return nil, err
	}
	// 续期 latest/expired_at
	_ = s.sess.TouchLatest(ctx, env, tenantID, sessionID, time.Now().UTC())
	return m, nil
}

// SummarizeIfNeeded：当消息/体量超过阈值或会话过期时，生成滚动摘要并续期
// 这里给一个“轻量无 LLM”的实现：拼接最近 N 条做简要摘要；后续你可替换为真实 LLM 总结。
func (s *ChatHistoryService) SummarizeIfNeeded(
	ctx context.Context, env string, tenantID *uint64, session *dbmodel.AgentChatSession,
) (bool, error) {
	if session == nil {
		return false, errors.New("nil session")
	}
	// 确保策略默认值
	s.ensureDefaults(session)

	stats, err := s.msg.StatsBySession(ctx, env, tenantID, session.ID)
	if err != nil {
		return false, err
	}

	overTokens := session.MaxTokens > 0 && int(stats.TotalTokens) >= session.MaxTokens
	overSize := session.MaxKB > 0 && int(stats.TotalSize)/1024 >= session.MaxKB
	expired := session.ExpiredAt != nil && time.Now().UTC().After(*session.ExpiredAt)

	if !(overTokens || overSize || expired) {
		return false, nil
	}

	// 取最近 N 条做一个“轻量摘要”（占位）
	const N = 8
	latest, _ := s.msg.ListLatestN(ctx, env, tenantID, session.ID, N)
	var b strings.Builder
	for i := range latest {
		r := latest[i].Role
		if r == "" {
			r = "msg"
		}
		content := latest[i].Content
		if len(content) > 300 {
			content = content[:300] + "…"
		}
		b.WriteString("- ")
		b.WriteString(r)
		b.WriteString(": ")
		b.WriteString(content)
		b.WriteByte('\n')
	}
	sum := b.String()
	if strings.TrimSpace(sum) == "" {
		sum = "（自动摘要占位）"
	}

	if err := s.sess.SetSummary(ctx, env, tenantID, session.ID, sum); err != nil {
		return false, err
	}
	// 摘要后，从现在起续期
	_ = s.sess.TouchLatest(ctx, env, tenantID, session.ID, time.Now().UTC())
	return true, nil
}
