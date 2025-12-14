package repository

import (
	"context"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

type AIUsageLogRepository struct {
	*coreRepo.BaseRepository[dbmodel.AIUsageLog]
	db *gorm.DB
}

func NewAIUsageLogRepository(db *gorm.DB) *AIUsageLogRepository {
	return &AIUsageLogRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AIUsageLog](db),
		db:             db,
	}
}

func (r *AIUsageLogRepository) CreateOne(ctx context.Context, row *dbmodel.AIUsageLog) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *AIUsageLogRepository) ListRecent(
	ctx context.Context, env string, tenantUUID *string, modality string, limit int,
) ([]dbmodel.AIUsageLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var list []dbmodel.AIUsageLog
	err := r.db.WithContext(ctx).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Where("modality = ?", modality).
		Order("id DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

type UsageAggRow struct {
	Provider  string  `json:"provider"`
	Model     string  `json:"model"`
	Count     int64   `json:"count"`
	TokensIn  int64   `json:"tokensIn"`
	TokensOut int64   `json:"tokensOut"`
	CostUSD   float64 `json:"costUSD"`
}

func (r *AIUsageLogRepository) AggregateByModel(
	ctx context.Context, env string, tenantUUID *string, modality string,
) ([]UsageAggRow, error) {
	var rows []UsageAggRow
	err := r.db.WithContext(ctx).Model(&dbmodel.AIUsageLog{}).
		Scopes(dbmodel.WithScope(env, tenantUUID)).
		Select(`
			provider, model,
			COUNT(*)                       AS count,
			COALESCE(SUM(tokens_in),  0)   AS tokens_in,
			COALESCE(SUM(tokens_out), 0)   AS tokens_out,
			COALESCE(SUM(cost_usd),   0.0) AS cost_usd`).
		Where("modality = ?", modality).
		Group("provider, model").
		Scan(&rows).Error
	return rows, err
}
