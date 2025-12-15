// internal/server/agent/persistence/repository/agent_chat_session_repo.go
package repository

import (
	"context"
	"errors"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type AgentChatSessionRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentChatSession]
	db *gorm.DB
}

func NewAgentChatSessionRepository(db *gorm.DB) *AgentChatSessionRepository {
	return &AgentChatSessionRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentChatSession](db),
		db:             db,
	}
}

// GetOrCreate：根据 singleton 策略获取或创建会话
// - singleton=true：按 env/tenant/agent 唯一的“活动会话”
// - singleton=false：总是创建新会话
func (r *AgentChatSessionRepository) GetOrCreate(
	ctx context.Context,
	env string, tenantUUID *string,
	agentID uint64, userID uint64,
	singleton bool,
	defaults dbmodel.AgentChatSession, // 允许传 Title/TTL/MaxKB/MaxTokens 等缺省
) (*dbmodel.AgentChatSession, error) {

	tx := r.db.WithContext(ctx)

	// 单例：查找当前活动会话
	if singleton {
		var old dbmodel.AgentChatSession
		err := tx.Scopes(dbmodel.WithScope(env, tenantUUID)).
			Where("agent_id = ? AND singleton = ? AND status = 'active'", agentID, true).
			First(&old).Error
		if err == nil {
			return &old, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	// 创建新会话（或单例未命中时创建）
	now := time.Now().UTC()
	sess := &dbmodel.AgentChatSession{
		Env:        env,
		TenantUUID: tenantUUID,
		AgentID:   agentID,
		UserID:    userID,
		Title:     defaults.Title,
		Singleton: singleton,
		TTLDays:   defaults.TTLDays,
		MaxKB:     defaults.MaxKB,
		MaxTokens: defaults.MaxTokens,
		Status:    "active",
		Meta:      defaults.Meta,
		LatestAt:  &now,
		Summary:   defaults.Summary,
		SummaryAt: defaults.SummaryAt,
	}

	// 默认策略兜底
	if sess.TTLDays == 0 {
		sess.TTLDays = 3
	}
	if sess.MaxKB == 0 {
		sess.MaxKB = 200
	}
	if sess.MaxTokens == 0 {
		sess.MaxTokens = 3000
	}
	// 计算过期时间
	exp := now.AddDate(0, 0, sess.TTLDays)
	sess.ExpiredAt = &exp

	if err := tx.Create(sess).Error; err != nil {
		return nil, err
	}
	return sess, nil
}

// FindByID（带作用域）
func (r *AgentChatSessionRepository) FindByID(
	ctx context.Context, env string, tenantUUID *string, id uint64,
) (*dbmodel.AgentChatSession, error) {
	var out dbmodel.AgentChatSession
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("id = ?", id).First(&out).Error
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListByAgent：按 Agent 列表会话（可选 status 过滤）
func (r *AgentChatSessionRepository) ListByAgent(
	ctx context.Context,
	env string, tenantUUID *string,
	agentID uint64,
	statuses []string,
	limit, offset int,
) ([]dbmodel.AgentChatSession, error) {
	tx := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("agent_id = ?", agentID)

	if len(statuses) > 0 {
		tx = tx.Where("status IN ?", statuses)
	}
	if limit <= 0 {
		limit = 50
	}
	var list []dbmodel.AgentChatSession
	if err := tx.
		Order("COALESCE(latest_at, updated_at) DESC").
		Limit(limit).
		Offset(offset).
		Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// TouchLatest：更新最近消息时间；并按 TTLDays 续期 ExpiredAt
func (r *AgentChatSessionRepository) TouchLatest(
	ctx context.Context, env string, tenantUUID *string, id uint64, t time.Time,
) error {
	// 先取 TTLDays，再更新
	var s dbmodel.AgentChatSession
	if err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Select("id", "ttldays").
		Where("id = ?", id).First(&s).Error; err != nil {
		return err
	}
	exp := t.AddDate(0, 0, s.TTLDays)
	return r.db.WithContext(ctx).
		Model(&dbmodel.AgentChatSession{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("id = ?", id).
		Updates(map[string]any{
			"latest_at":  t.UTC(),
			"expired_at": exp.UTC(),
			"updated_at": time.Now().UTC(),
		}).Error
}

// SetSummary：更新滚动摘要
func (r *AgentChatSessionRepository) SetSummary(
	ctx context.Context, env string, tenantUUID *string, id uint64, summary string,
) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&dbmodel.AgentChatSession{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("id = ?", id).
		Updates(map[string]any{
			"summary":    summary,
			"summary_at": &now,
			"updated_at": now,
		}).Error
}

// UpdatePolicy：更新会话策略（TTL/MaxKB/MaxTokens）
func (r *AgentChatSessionRepository) UpdatePolicy(
	ctx context.Context, env string, tenantUUID *string, id uint64,
	ttlDays, maxKB, maxTokens int,
) error {
	updates := map[string]any{}
	if ttlDays > 0 {
		updates["ttldays"] = ttlDays
		// 同时刷新过期时间
		exp := time.Now().UTC().AddDate(0, 0, ttlDays)
		updates["expired_at"] = exp
	}
	if maxKB > 0 {
		updates["max_kb"] = maxKB
	}
	if maxTokens > 0 {
		updates["max_tokens"] = maxTokens
	}
	if len(updates) == 0 {
		return nil
	}
	updates["updated_at"] = time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&dbmodel.AgentChatSession{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("id = ?", id).
		Updates(updates).Error
}

// Archive：归档
func (r *AgentChatSessionRepository) Archive(
	ctx context.Context, env string, tenantUUID *string, id uint64,
) error {
	return r.db.WithContext(ctx).
		Model(&dbmodel.AgentChatSession{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("id = ?", id).
		Update("status", "archived").Error
}

// DeleteSoft：软删（保持和 Agent 的 DeleteSoft 一致风格）
func (r *AgentChatSessionRepository) DeleteSoft(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&dbmodel.AgentChatSession{}, id).Error
}

// ListExpiredIDs：列出已过期会话 ID（用于服务层清理消息+会话）
func (r *AgentChatSessionRepository) ListExpiredIDs(
	ctx context.Context, env string, tenantUUID *string, now time.Time, limit int,
) ([]uint64, error) {
	if limit <= 0 {
		limit = 200
	}
	var ids []uint64
	err := r.db.WithContext(ctx).
		Model(&dbmodel.AgentChatSession{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("expired_at IS NOT NULL AND expired_at <= ?", now.UTC()).
		Limit(limit).
		Pluck("id", &ids).Error
	return ids, err
}

// 修改标题（e.g. 首条用户消息后生成主题）
func (r *AgentChatSessionRepository) UpdateTitle(
	ctx context.Context, env string, tenantUUID *string, id uint64, title string,
) error {
	return r.db.WithContext(ctx).
		Model(&dbmodel.AgentChatSession{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("id = ?", id).
		Updates(map[string]any{
			"title":      title,
			"updated_at": time.Now().UTC(),
		}).Error
}

// 将会话切换到另一个 Agent（可选）
func (r *AgentChatSessionRepository) SetAgent(
	ctx context.Context, env string, tenantUUID *string, id uint64, agentID uint64,
) error {
	return r.db.WithContext(ctx).
		Model(&dbmodel.AgentChatSession{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("id = ?", id).
		Updates(map[string]any{
			"agent_id":   agentID,
			"updated_at": time.Now().UTC(),
		}).Error
}

func (r *AgentChatSessionRepository) UpdateSessionTitle(
	ctx context.Context, env string, tenantUUID *string, sessionID uint64, title string,
) error {
	q := r.db.WithContext(ctx).Model(&dbmodel.AgentChatSession{}).
		Where("id = ?", sessionID)
	if tenantUUID != nil && *tenantUUID != "" {
		q = q.Where("tenant_uuid = ?", *tenantUUID)
	}
	if env != "" {
		q = q.Where("env = ?", env)
	}
	return q.Update("title", title).Error
}
