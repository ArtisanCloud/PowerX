package integration_gateway

import (
	"context"

	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type IntegrationGatewayAPIKeyAuditLogRepository struct {
	*baseRepo.BaseRepository[models.IntegrationGatewayAPIKeyAuditLog]
	db *gorm.DB
}

func NewIntegrationGatewayAPIKeyAuditLogRepository(db *gorm.DB) *IntegrationGatewayAPIKeyAuditLogRepository {
	if db == nil {
		panic("integration gateway api key audit repository requires non-nil db")
	}
	return &IntegrationGatewayAPIKeyAuditLogRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.IntegrationGatewayAPIKeyAuditLog](db),
		db:             db,
	}
}

func (r *IntegrationGatewayAPIKeyAuditLogRepository) Create(ctx context.Context, item *models.IntegrationGatewayAPIKeyAuditLog) (*models.IntegrationGatewayAPIKeyAuditLog, error) {
	if item == nil {
		return nil, gorm.ErrInvalidData
	}
	if err := r.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}
