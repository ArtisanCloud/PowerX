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
	ctx context.Context, env string, tenantUUID *string,
	agentID uint64, userID uint64, singleton bool,
	defaults *dbmodel.AgentChatSession,
) (*dbmodel.AgentChatSession, error) {

	// 安全拿到一份“值类型”的默认策略
	def := s.defaultPolicy()
	if defaults != nil {
		def = *defaults
		s.ensureDefaults(&def)
	}

	out, err := s.sess.GetOrCreate(ctx, env, tenantUUID, agentID, userID, singleton, def)
	if err != nil {
		return nil, err
	}

	// 触发一次最近活跃续期
	_ = s.sess.TouchLatest(ctx, env, tenantUUID, out.ID, time.Now().UTC())
	return out, nil
}

// FindSessionByID：带作用域读取
func (s *ChatHistoryService) FindSessionByID(
	ctx context.Context, env string, tenantUUID *string, id uint64,
) (*dbmodel.AgentChatSession, error) {
	return s.sess.FindByID(ctx, env, tenantUUID, id)
}

func (s *ChatHistoryService) FindSessionByUUID(
	ctx context.Context, env string, tenantUUID *string, uid string,
) (*dbmodel.AgentChatSession, error) {
	return s.sess.FindByUUID(ctx, env, tenantUUID, uid)
}

// ListSessions：按 Agent 维度分页查询（statuses 可为空）
func (s *ChatHistoryService) ListSessions(
	ctx context.Context, env string, tenantUUID *string,
	agentID uint64, statuses []string,
	limit, offset int,
) ([]dbmodel.AgentChatSession, error) {
	return s.sess.ListByAgent(ctx, env, tenantUUID, agentID, statuses, limit, offset)
}

// UpdateSessionPolicy：可选更新 title / TTL / MaxKB / MaxTokens（部分字段更新）
// 注意：repo 里没有改 title 的方法，这里用 gorm 直接更新 title
func (s *ChatHistoryService) UpdateSessionPolicy(
	ctx context.Context, env string, tenantUUID *string, id uint64,
	title *string, ttlDays, maxKB, maxTokens *int,
) error {
	// 1) title
	if title != nil {
		err := s.db.WithContext(ctx).
			Model(&dbmodel.AgentChatSession{}).
			Scopes(dbmodel.WithScope(env, tenantUUID)).
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
		if err := s.sess.UpdatePolicy(ctx, env, tenantUUID, id, t, kb, tk); err != nil {
			return err
		}
		// 更新策略后，顺带续期
		_ = s.sess.TouchLatest(ctx, env, tenantUUID, id, time.Now().UTC())
	}
	return nil
}

// ArchiveSession：归档（软删除前常用）
func (s *ChatHistoryService) ArchiveSession(
	ctx context.Context, env string, tenantUUID *string, id uint64,
) error {
	return s.sess.Archive(ctx, env, tenantUUID, id)
}

// DeleteSession：先删消息再软删会话
func (s *ChatHistoryService) DeleteSession(
	ctx context.Context, env string, tenantUUID *string, id uint64,
) error {
	// 先软删会话，保证前端“删除”动作能快速返回（从列表中消失）。
	// 会话内消息清理由后台异步 best-effort 完成，避免消息量大时阻塞 HTTP 请求直至超时。
	if err := s.sess.DeleteSoft(ctx, id); err != nil {
		return err
	}

	go func() {
		var tenantCopy *string
		if tenantUUID != nil {
			v := *tenantUUID
			tenantCopy = &v
		}
		ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = s.msg.DeleteBySession(ctx2, env, tenantCopy, id)
	}()

	return nil
}

// ListMessages：会话内分页拉取消息（支持 afterID 游标）
func (s *ChatHistoryService) ListMessages(
	ctx context.Context, env string, tenantUUID *string,
	sessionID uint64, afterID uint64, limit int,
) ([]dbmodel.AgentChatMessage, error) {
	return s.msg.ListBySession(ctx, env, tenantUUID, sessionID, afterID, limit)
}

// AppendMessage：追加一条消息，并刷新会话“最近活跃/过期时间”
func (s *ChatHistoryService) AppendMessage(
	ctx context.Context, env string, tenantUUID *string,
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
		TenantUUID:    tenantUUID,
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
	_ = s.sess.TouchLatest(ctx, env, tenantUUID, sessionID, time.Now().UTC())
	return m, nil
}

// FindMessageByID：读取单条消息（带 scope），用于“从某条 user 消息重新生成”等场景。
func (s *ChatHistoryService) FindMessageByID(
	ctx context.Context, env string, tenantUUID *string, id uint64,
) (*dbmodel.AgentChatMessage, error) {
	var out dbmodel.AgentChatMessage
	err := s.db.WithContext(ctx).
		Model(&dbmodel.AgentChatMessage{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("id = ?", id).
		First(&out).Error
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// TruncateMessagesAfter：删除会话内 id > afterID 的消息，用于“从此问题重新生成”（裁剪后续对话）。
func (s *ChatHistoryService) TruncateMessagesAfter(
	ctx context.Context, env string, tenantUUID *string, sessionID uint64, afterID uint64,
) (int64, error) {
	affected, err := s.msg.DeleteAfterID(ctx, env, tenantUUID, sessionID, afterID)
	if err != nil {
		return 0, err
	}
	_ = s.sess.TouchLatest(ctx, env, tenantUUID, sessionID, time.Now().UTC())
	return affected, nil
}

// UpdateMessageContent：更新单条消息内容（带 scope），用于“编辑问题后重新生成”等场景。
func (s *ChatHistoryService) UpdateMessageContent(
	ctx context.Context, env string, tenantUUID *string, id uint64, content string,
) error {
	content = strings.TrimSpace(content)
	ct := "text/plain"
	if content == "" {
		ct = "text/plain"
	}
	return s.db.WithContext(ctx).
		Model(&dbmodel.AgentChatMessage{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("id = ?", id).
		Updates(map[string]any{
			"content":     content,
			"size_bytes":  len([]byte(content)),
			"updated_at":  time.Now().UTC(),
			"is_error":    false,
			"tokens":      0,
			"content_type": ct,
		}).Error
}

// SummarizeIfNeeded：当消息/体量超过阈值或会话过期时，生成滚动摘要并续期
// 这里给一个“轻量无 LLM”的实现：拼接最近 N 条做简要摘要；后续你可替换为真实 LLM 总结。
func (s *ChatHistoryService) SummarizeIfNeeded(
	ctx context.Context, env string, tenantUUID *string, session *dbmodel.AgentChatSession,
) (bool, error) {
	if session == nil {
		return false, errors.New("nil session")
	}
	// 确保策略默认值
	s.ensureDefaults(session)

	stats, err := s.msg.StatsBySession(ctx, env, tenantUUID, session.ID)
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
	latest, _ := s.msg.ListLatestN(ctx, env, tenantUUID, session.ID, N)
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

	if err := s.sess.SetSummary(ctx, env, tenantUUID, session.ID, sum); err != nil {
		return false, err
	}
	// 摘要后，从现在起续期
	_ = s.sess.TouchLatest(ctx, env, tenantUUID, session.ID, time.Now().UTC())
	return true, nil
}

func (s *ChatHistoryService) RenameSession(
	ctx context.Context, env string, tenantUUID *string, sessionID uint64, title string,
) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	return s.sess.UpdateSessionTitle(ctx, env, tenantUUID, sessionID, title)
}
