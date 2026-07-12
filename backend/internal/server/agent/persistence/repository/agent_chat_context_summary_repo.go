package repository

import (
	"context"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type AgentChatContextSummaryRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentChatContextSummary]
	db *gorm.DB
}

func NewAgentChatContextSummaryRepository(db *gorm.DB) *AgentChatContextSummaryRepository {
	return &AgentChatContextSummaryRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentChatContextSummary](db),
		db:             db,
	}
}

func (r *AgentChatContextSummaryRepository) Create(ctx context.Context, in *dbmodel.AgentChatContextSummary) error {
	return r.db.WithContext(ctx).Create(in).Error
}

func (r *AgentChatContextSummaryRepository) LatestBySession(
	ctx context.Context,
	env string,
	tenantUUID *string,
	sessionID uint64,
) (*dbmodel.AgentChatContextSummary, error) {
	var out dbmodel.AgentChatContextSummary
	if err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("session_id = ?", sessionID).
		Order("id DESC").
		First(&out).Error; err != nil {
		return nil, err
	}
	return &out, nil
}
