package integration_gateway

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// IntegrationInvocationLogRepository 负责调用日志的持久化。
type IntegrationInvocationLogRepository struct {
	*baseRepo.BaseRepository[models.IntegrationInvocationLog]
	db *gorm.DB
}

func NewIntegrationInvocationLogRepository(db *gorm.DB) *IntegrationInvocationLogRepository {
	if db == nil {
		panic("integration invocation log repository requires non-nil db")
	}
	return &IntegrationInvocationLogRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.IntegrationInvocationLog](db),
		db:             db,
	}
}

func (r *IntegrationInvocationLogRepository) Create(ctx context.Context, log *models.IntegrationInvocationLog) (*models.IntegrationInvocationLog, error) {
	if log == nil {
		return nil, gorm.ErrInvalidData
	}
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return nil, err
	}
	return log, nil
}

func (r *IntegrationInvocationLogRepository) ListByRoute(ctx context.Context, routeUUID uuid.UUID, limit int) ([]models.IntegrationInvocationLog, error) {
	query := r.db.WithContext(ctx).
		Where("route_uuid = ?", routeUUID).
		Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var records []models.IntegrationInvocationLog
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
