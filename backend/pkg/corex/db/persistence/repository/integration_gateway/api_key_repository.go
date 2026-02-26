package integration_gateway

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type IntegrationGatewayAPIKeyRepository struct {
	*baseRepo.BaseRepository[models.IntegrationGatewayAPIKey]
	db *gorm.DB
}

func NewIntegrationGatewayAPIKeyRepository(db *gorm.DB) *IntegrationGatewayAPIKeyRepository {
	if db == nil {
		panic("integration gateway api key repository requires non-nil db")
	}
	return &IntegrationGatewayAPIKeyRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.IntegrationGatewayAPIKey](db),
		db:             db,
	}
}

func (r *IntegrationGatewayAPIKeyRepository) Create(ctx context.Context, item *models.IntegrationGatewayAPIKey) (*models.IntegrationGatewayAPIKey, error) {
	if item == nil {
		return nil, gorm.ErrInvalidData
	}
	if err := r.db.WithContext(ctx).Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *IntegrationGatewayAPIKeyRepository) GetByUUID(ctx context.Context, keyUUID uuid.UUID) (*models.IntegrationGatewayAPIKey, error) {
	var item models.IntegrationGatewayAPIKey
	if err := r.db.WithContext(ctx).Where("uuid = ?", keyUUID).First(&item).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *IntegrationGatewayAPIKeyRepository) FindActiveByHash(ctx context.Context, tenantUUID string, keyHash string) (*models.IntegrationGatewayAPIKey, error) {
	var item models.IntegrationGatewayAPIKey
	query := r.db.WithContext(ctx).
		Where("key_hash = ? AND status = ?", strings.TrimSpace(keyHash), "active")
	if strings.TrimSpace(tenantUUID) != "" {
		query = query.Where("tenant_uuid = ?", strings.TrimSpace(tenantUUID))
	}
	if err := query.First(&item).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *IntegrationGatewayAPIKeyRepository) ListByTenant(ctx context.Context, tenantUUID string, offset, limit int) ([]models.IntegrationGatewayAPIKey, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.IntegrationGatewayAPIKey{}).
		Where("tenant_uuid = ?", strings.TrimSpace(tenantUUID)).
		Order("created_at DESC")

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	var items []models.IntegrationGatewayAPIKey
	if err := query.Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *IntegrationGatewayAPIKeyRepository) UpdateLastUsed(ctx context.Context, keyUUID uuid.UUID, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&models.IntegrationGatewayAPIKey{}).
		Where("uuid = ?", keyUUID).
		Update("last_used_at", at.UTC()).
		Error
}

func (r *IntegrationGatewayAPIKeyRepository) UpdateStatus(ctx context.Context, keyUUID uuid.UUID, status string, actor string) error {
	updates := map[string]interface{}{
		"status":     strings.TrimSpace(status),
		"updated_by": strings.TrimSpace(actor),
	}
	return r.db.WithContext(ctx).
		Model(&models.IntegrationGatewayAPIKey{}).
		Where("uuid = ?", keyUUID).
		Updates(updates).
		Error
}
