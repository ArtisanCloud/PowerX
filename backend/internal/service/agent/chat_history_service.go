// internal/service/agent/chat_history_service.go
package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func readJSONMapString(meta datatypes.JSONMap, key string) string {
	if meta == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(meta[key]))
}

func readJSONMapStringList(meta datatypes.JSONMap, key string) []string {
	if meta == nil {
		return nil
	}
	raw, ok := meta[key]
	if !ok || raw == nil {
		return nil
	}
	return readAnyStringList(raw)
}

func readAnyStringList(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return nil
		}
		var arr []string
		if err := json.Unmarshal([]byte(text), &arr); err == nil {
			return arr
		}
		var anyArr []any
		if err := json.Unmarshal([]byte(text), &anyArr); err == nil {
			return readAnyStringList(anyArr)
		}
		return []string{text}
	default:
		return nil
	}
}

func readJSONMapNestedStringList(meta datatypes.JSONMap, objectKey string, listKey string) []string {
	if meta == nil {
		return nil
	}
	raw, ok := meta[objectKey]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case map[string]any:
		return readAnyStringList(v[listKey])
	case datatypes.JSONMap:
		return readAnyStringList(v[listKey])
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return nil
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(text), &obj); err == nil {
			return readAnyStringList(obj[listKey])
		}
	}
	return nil
}

func readSessionMetaString(meta datatypes.JSONMap, key string) string {
	if meta == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(meta[key]))
}

type ChatHistoryService struct {
	db      *gorm.DB
	sess    *repo.AgentChatSessionRepository
	msg     *repo.AgentChatMessageRepository
	summary *repo.AgentChatContextSummaryRepository
}

type RollingContextCompressionPolicy struct {
	RecentMessages int
	MaxMessages    int
	DeleteCovered  bool
}

type RollingContextCompressionResult struct {
	Compressed          bool
	CompressedMessages  int
	DeletedMessages     int64
	FromMessageID       uint64
	ToMessageID         uint64
	RecentMessagesKept  int
	PreviousSummaryUsed bool
	Summary             SessionStructuredSummary
}

func NewChatHistoryService(db *gorm.DB) *ChatHistoryService {
	return &ChatHistoryService{
		db:      db,
		sess:    repo.NewAgentChatSessionRepository(db),
		msg:     repo.NewAgentChatMessageRepository(db),
		summary: repo.NewAgentChatContextSummaryRepository(db),
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
	if err := s.sess.DeleteSoft(ctx, env, tenantUUID, id); err != nil {
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
		TenantUUID:  tenantUUID,
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

func (s *ChatHistoryService) HasRecentCapabilityIntro(
	ctx context.Context,
	env string,
	tenantUUID *string,
	sessionID uint64,
	capabilityIDs []string,
	limit int,
) (bool, error) {
	if sessionID == 0 {
		return false, nil
	}
	if limit <= 0 {
		limit = 12
	}
	capSet := map[string]struct{}{}
	for _, id := range capabilityIDs {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" {
			capSet[id] = struct{}{}
		}
	}
	msgs, err := s.msg.ListLatestN(ctx, env, tenantUUID, sessionID, limit)
	if err != nil {
		return false, err
	}
	for _, msg := range msgs {
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "assistant") {
			continue
		}
		if !strings.EqualFold(readJSONMapString(msg.Meta, "response_mode"), "capability_intro") {
			continue
		}
		if len(capSet) == 0 {
			return true, nil
		}
		ids := readJSONMapStringList(msg.Meta, "capability_ids")
		if len(ids) == 0 {
			ids = readJSONMapNestedStringList(msg.Meta, "response_plan", "target_capability_ids")
		}
		for _, id := range ids {
			if _, ok := capSet[strings.ToLower(strings.TrimSpace(id))]; ok {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *ChatHistoryService) LatestPendingTask(
	ctx context.Context,
	env string,
	tenantUUID *string,
	sessionID uint64,
	limit int,
) (datatypes.JSONMap, bool, error) {
	if sessionID == 0 {
		return nil, false, nil
	}
	if limit <= 0 {
		limit = 12
	}
	msgs, err := s.msg.ListLatestN(ctx, env, tenantUUID, sessionID, limit)
	if err != nil {
		return nil, false, err
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "assistant") {
			continue
		}
		task := readJSONMapNestedMap(msg.Meta, "pending_task")
		if len(task) == 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(task["status"])), "awaiting_params") {
			return task, true, nil
		}
	}
	return nil, false, nil
}

func readJSONMapNestedMap(meta datatypes.JSONMap, key string) datatypes.JSONMap {
	if meta == nil {
		return nil
	}
	raw, ok := meta[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case datatypes.JSONMap:
		return v
	case map[string]any:
		out := datatypes.JSONMap{}
		for k, item := range v {
			out[k] = item
		}
		return out
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return nil
		}
		var out datatypes.JSONMap
		if err := json.Unmarshal([]byte(text), &out); err == nil {
			return out
		}
		var generic map[string]any
		if err := json.Unmarshal([]byte(text), &generic); err == nil {
			out = datatypes.JSONMap{}
			for k, item := range generic {
				out[k] = item
			}
			return out
		}
	}
	return nil
}

func (s *ChatHistoryService) RollingCompressIfNeeded(
	ctx context.Context,
	env string,
	tenantUUID *string,
	session *dbmodel.AgentChatSession,
	policy RollingContextCompressionPolicy,
) (*RollingContextCompressionResult, error) {
	if session == nil {
		return nil, errors.New("nil session")
	}
	keepLatest := policy.RecentMessages
	if keepLatest <= 0 {
		keepLatest = 20
	}
	maxMessages := policy.MaxMessages
	if maxMessages <= 0 {
		maxMessages = 500
	}
	items, err := s.msg.ListCompressibleBeforeLatestN(ctx, env, tenantUUID, session.ID, keepLatest, maxMessages)
	if err != nil {
		return nil, err
	}
	result := &RollingContextCompressionResult{RecentMessagesKept: keepLatest}
	if len(items) == 0 {
		return result, nil
	}
	previous, previousOK := parseStructuredSessionSummary(session.Summary)
	next := mergeRollingSummary(previous, previousOK, items, keepLatest)
	raw, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	summaryID := buildContextSummaryID(session.ID, next.ToMessageID, raw)
	var sourceSummaryID *string
	if previousOK {
		if oldID := strings.TrimSpace(readSessionMetaString(session.Meta, "active_context_summary_id")); oldID != "" {
			sourceSummaryID = &oldID
		}
	}
	summaryRecord := &dbmodel.AgentChatContextSummary{
		Env:                env,
		TenantUUID:         tenantUUID,
		SessionID:          session.ID,
		AgentID:            session.AgentID,
		UserID:             session.UserID,
		SummaryID:          summaryID,
		SourceSummaryID:    sourceSummaryID,
		Schema:             structuredSummarySchemaV1,
		FromMessageID:      next.FromMessageID,
		ToMessageID:        next.ToMessageID,
		CompressedMessages: next.CompressedMessages,
		RecentMessagesKept: next.RecentMessagesKept,
		CompressionPolicy:  next.CompressionPolicy,
		SummaryJSON:        summaryToJSONMap(next),
		SummaryText:        renderStructuredSummaryText(next),
		Checksum:           fmt.Sprintf("%x", sha256.Sum256(raw)),
		Meta: datatypes.JSONMap{
			"covered_message_count": len(items),
		},
	}
	if err := s.summary.Create(ctx, summaryRecord); err != nil {
		return nil, err
	}
	if err := s.sess.SetSummary(ctx, env, tenantUUID, session.ID, string(raw)); err != nil {
		return nil, err
	}
	if err := s.updateSessionContextSummaryMeta(ctx, env, tenantUUID, session.ID, summaryID); err != nil {
		return nil, err
	}
	ids := make([]uint64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	var deleted int64
	if policy.DeleteCovered {
		deleted, err = s.msg.DeleteByIDs(ctx, env, tenantUUID, ids)
		if err != nil {
			return nil, err
		}
	}
	session.Summary = string(raw)
	if session.Meta == nil {
		session.Meta = datatypes.JSONMap{}
	}
	session.Meta["active_context_summary_id"] = summaryID
	now := time.Now().UTC()
	session.SummaryAt = &now
	result.Compressed = true
	result.CompressedMessages = len(items)
	result.DeletedMessages = deleted
	result.FromMessageID = items[0].ID
	result.ToMessageID = items[len(items)-1].ID
	result.PreviousSummaryUsed = previousOK
	result.Summary = next
	return result, nil
}

func (s *ChatHistoryService) updateSessionContextSummaryMeta(
	ctx context.Context,
	env string,
	tenantUUID *string,
	sessionID uint64,
	summaryID string,
) error {
	var session dbmodel.AgentChatSession
	if err := s.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("id = ?", sessionID).
		First(&session).Error; err != nil {
		return err
	}
	meta := session.Meta
	if meta == nil {
		meta = datatypes.JSONMap{}
	}
	meta["active_context_summary_id"] = summaryID
	meta["active_context_summary_at"] = time.Now().UTC().Format(time.RFC3339)
	return s.db.WithContext(ctx).
		Model(&dbmodel.AgentChatSession{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("id = ?", sessionID).
		Updates(map[string]any{
			"meta":       meta,
			"updated_at": time.Now().UTC(),
		}).Error
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
			"content":      content,
			"size_bytes":   len([]byte(content)),
			"updated_at":   time.Now().UTC(),
			"is_error":     false,
			"tokens":       0,
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

	res, err := s.RollingCompressIfNeeded(ctx, env, tenantUUID, session, RollingContextCompressionPolicy{
		RecentMessages: 20,
		MaxMessages:    500,
		DeleteCovered:  true,
	})
	if err != nil {
		return false, err
	}
	// 摘要后，从现在起续期
	_ = s.sess.TouchLatest(ctx, env, tenantUUID, session.ID, time.Now().UTC())
	return res != nil && res.Compressed, nil
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

func parseStructuredSessionSummary(raw string) (SessionStructuredSummary, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SessionStructuredSummary{}, false
	}
	var st SessionStructuredSummary
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return SessionStructuredSummary{}, false
	}
	if strings.TrimSpace(st.Schema) != structuredSummarySchemaV1 {
		return SessionStructuredSummary{}, false
	}
	return st, true
}

func mergeRollingSummary(previous SessionStructuredSummary, previousOK bool, items []dbmodel.AgentChatMessage, keepLatest int) SessionStructuredSummary {
	now := time.Now().UTC().Format(time.RFC3339)
	next := SessionStructuredSummary{
		Schema:             structuredSummarySchemaV1,
		UpdatedAt:          now,
		RecentMessagesKept: keepLatest,
		CompressionPolicy:  "rolling_summary_v1",
	}
	if previousOK {
		next.Facts = append(next.Facts, previous.Facts...)
		next.Decisions = append(next.Decisions, previous.Decisions...)
		next.OpenIssues = append(next.OpenIssues, previous.OpenIssues...)
		next.Constraints = append(next.Constraints, previous.Constraints...)
		next.SourceSummaryIDs = append(next.SourceSummaryIDs, previous.SourceSummaryIDs...)
		next.PreviousSummaryAt = previous.UpdatedAt
		if previous.FromMessageID > 0 {
			next.FromMessageID = previous.FromMessageID
		}
		next.CompressedMessages = previous.CompressedMessages
	}
	if len(items) > 0 {
		if next.FromMessageID == 0 {
			next.FromMessageID = items[0].ID
		}
		next.ToMessageID = items[len(items)-1].ID
		next.CompressedMessages += len(items)
	}
	for _, item := range items {
		entry := summarizeMessageForMemory(item)
		if entry == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(item.Role)) {
		case "assistant":
			next.Decisions = append(next.Decisions, entry)
		case "system":
			next.Constraints = append(next.Constraints, entry)
		case "tool":
			next.Facts = append(next.Facts, entry)
		default:
			next.OpenIssues = append(next.OpenIssues, entry)
		}
	}
	next.Facts = boundedUniqueStrings(next.Facts, 30)
	next.Decisions = boundedUniqueStrings(next.Decisions, 30)
	next.OpenIssues = boundedUniqueStrings(next.OpenIssues, 30)
	next.Constraints = boundedUniqueStrings(next.Constraints, 20)
	next.SourceSummaryIDs = boundedUniqueStrings(next.SourceSummaryIDs, 20)
	return next
}

func summarizeMessageForMemory(item dbmodel.AgentChatMessage) string {
	content := strings.TrimSpace(item.Content)
	if content == "" {
		return ""
	}
	role := strings.TrimSpace(item.Role)
	if role == "" {
		role = "msg"
	}
	return fmt.Sprintf("%s#%d: %s", role, item.ID, trimRunes(content, 220))
}

func buildContextSummaryID(sessionID uint64, toMessageID uint64, raw []byte) string {
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("ctxsum_%d_%d_%x", sessionID, toMessageID, sum[:6])
}

func summaryToJSONMap(summary SessionStructuredSummary) datatypes.JSONMap {
	raw, err := json.Marshal(summary)
	if err != nil {
		return datatypes.JSONMap{}
	}
	var out datatypes.JSONMap
	if err := json.Unmarshal(raw, &out); err != nil {
		return datatypes.JSONMap{}
	}
	return out
}

func renderStructuredSummaryText(summary SessionStructuredSummary) string {
	parts := make([]string, 0, 4)
	if len(summary.Facts) > 0 {
		parts = append(parts, "facts: "+strings.Join(summary.Facts, " | "))
	}
	if len(summary.Decisions) > 0 {
		parts = append(parts, "decisions: "+strings.Join(summary.Decisions, " | "))
	}
	if len(summary.OpenIssues) > 0 {
		parts = append(parts, "open_issues: "+strings.Join(summary.OpenIssues, " | "))
	}
	if len(summary.Constraints) > 0 {
		parts = append(parts, "constraints: "+strings.Join(summary.Constraints, " | "))
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func boundedUniqueStrings(items []string, limit int) []string {
	if limit <= 0 {
		limit = 20
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, minInt(len(items), limit))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
