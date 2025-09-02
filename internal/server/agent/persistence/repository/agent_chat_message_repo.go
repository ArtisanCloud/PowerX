// internal/server/agent/persistence/repository/agent_chat_message_repo.go
package repository

import (
	"context"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type AgentChatMessageRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentChatMessage]
	db *gorm.DB
}

func NewAgentChatMessageRepository(db *gorm.DB) *AgentChatMessageRepository {
	return &AgentChatMessageRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentChatMessage](db),
		db:             db,
	}
}

// Append：追加一条消息（如果 SizeBytes==0，建议在 service 里按字节长度填好）
func (r *AgentChatMessageRepository) Append(ctx context.Context, in *dbmodel.AgentChatMessage) error {
	return r.db.WithContext(ctx).Create(in).Error
}

// BatchAppend：批量追加
func (r *AgentChatMessageRepository) BatchAppend(ctx context.Context, list []dbmodel.AgentChatMessage) error {
	if len(list) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&list).Error
}

// ListBySession：按会话分页拉取（升序返回，便于直接渲染）
// - afterID > 0 时，返回 id > afterID 的新消息
func (r *AgentChatMessageRepository) ListBySession(
	ctx context.Context,
	env string, tenantID *uint64,
	sessionID uint64,
	afterID uint64,
	limit int,
) ([]dbmodel.AgentChatMessage, error) {
	if limit <= 0 {
		limit = 200
	}
	tx := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantID)).
		Where("session_id = ?", sessionID)

	if afterID > 0 {
		tx = tx.Where("id > ?", afterID)
	}

	var out []dbmodel.AgentChatMessage
	if err := tx.Order("id ASC").Limit(limit).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// ListLatestN：取最近 N 条（用于构造上下文）
func (r *AgentChatMessageRepository) ListLatestN(
	ctx context.Context,
	env string, tenantID *uint64,
	sessionID uint64,
	n int,
) ([]dbmodel.AgentChatMessage, error) {
	if n <= 0 {
		n = 50
	}
	var items []dbmodel.AgentChatMessage
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantID)).
		Where("session_id = ?", sessionID).
		Order("id DESC").Limit(n).
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	// 逆序成时间正序
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, nil
}

// MarkPinned：标记/取消置顶
func (r *AgentChatMessageRepository) MarkPinned(
	ctx context.Context,
	env string, tenantID *uint64,
	msgID uint64, pinned bool,
) error {
	return r.db.WithContext(ctx).
		Model(&dbmodel.AgentChatMessage{}).
		Scopes(dbmodel.WithScope(env, tenantID)).
		Where("id = ?", msgID).
		Update("pinned", pinned).Error
}

// DeleteBySession：按会话批量删除消息（用于过期/归档清理）
func (r *AgentChatMessageRepository) DeleteBySession(
	ctx context.Context, env string, tenantID *uint64, sessionID uint64,
) error {
	return r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantID)).
		Where("session_id = ?", sessionID).
		Delete(&dbmodel.AgentChatMessage{}).Error
}

// StatsBySession：汇总消息体积/令牌数
type SessionMsgStats struct {
	Count       int64
	TotalTokens int64
	TotalSize   int64
}

func (r *AgentChatMessageRepository) StatsBySession(
	ctx context.Context, env string, tenantID *uint64, sessionID uint64,
) (*SessionMsgStats, error) {
	var s SessionMsgStats
	err := r.db.WithContext(ctx).
		Model(&dbmodel.AgentChatMessage{}).
		Scopes(dbmodel.WithScope(env, tenantID)).
		Where("session_id = ?", sessionID).
		Select("COUNT(*) AS count, COALESCE(SUM(tokens),0) AS total_tokens, COALESCE(SUM(size_bytes),0) AS total_size").
		Scan(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}
