package repository

import (
	"context"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	shareStatusActive = "active"
)

// AgentShareRepository 负责代理共享记录持久化。
type AgentShareRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentShareRecord]
}

// NewAgentShareRepository 构造仓储。
func NewAgentShareRepository(db *gorm.DB) *AgentShareRepository {
	return &AgentShareRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentShareRecord](db),
	}
}

// Create 插入共享记录。
func (r *AgentShareRepository) Create(ctx context.Context, record *dbmodel.AgentShareRecord) (*dbmodel.AgentShareRecord, error) {
	if err := r.DB.WithContext(ctx).Create(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

// Save 更新共享记录。
func (r *AgentShareRepository) Save(ctx context.Context, record *dbmodel.AgentShareRecord) (*dbmodel.AgentShareRecord, error) {
	if err := r.DB.WithContext(ctx).Save(record).Error; err != nil {
		return nil, err
	}
	return record, nil
}

// GetByUUID 根据 ID 查询共享记录。
func (r *AgentShareRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*dbmodel.AgentShareRecord, error) {
	var record dbmodel.AgentShareRecord
	if err := r.DB.WithContext(ctx).Where("uuid = ?", id).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// FindActiveByAgentTenant 查找 active 状态共享。
func (r *AgentShareRepository) FindActiveByAgentTenant(ctx context.Context, agentID uuid.UUID, tenantID string) (*dbmodel.AgentShareRecord, error) {
	var record dbmodel.AgentShareRecord
	if err := r.DB.WithContext(ctx).
		Where("agent_uuid = ? AND target_tenant_id = ? AND status = ?", agentID, tenantID, "active").
		First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// ListActive 返回代理当前所有 active 共享。
func (r *AgentShareRepository) ListActive(ctx context.Context, agentID uuid.UUID) ([]dbmodel.AgentShareRecord, error) {
	var records []dbmodel.AgentShareRecord
	if err := r.DB.WithContext(ctx).Where("agent_uuid = ? AND status = ?", agentID, "active").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// ListDueForReview 返回到期需要复核的共享记录。
func (r *AgentShareRepository) ListDueForReview(ctx context.Context, now time.Time, limit int) ([]dbmodel.AgentShareRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var records []dbmodel.AgentShareRecord
	err := r.DB.WithContext(ctx).
		Where("status = ? AND (next_review_at IS NULL OR next_review_at <= ?)", shareStatusActive, now).
		Order("next_review_at NULLS FIRST, updated_at ASC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

// MarkValidationFailure 标记共享为验证失败并记录原因。
func (r *AgentShareRepository) MarkValidationFailure(ctx context.Context, id uuid.UUID, reason string) error {
	return r.DB.WithContext(ctx).Model(&dbmodel.AgentShareRecord{}).
		Where("uuid = ?", id).
		Updates(map[string]any{
			"status":            "error",
			"validation_failed": true,
			"validation_error":  reason,
			"next_review_at":    nil,
		}).Error
}
