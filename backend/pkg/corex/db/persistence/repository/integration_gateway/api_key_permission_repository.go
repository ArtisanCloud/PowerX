package integration_gateway

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

type IntegrationGatewayAPIKeyPermissionRepository struct {
	*baseRepo.BaseRepository[models.IntegrationGatewayAPIKeyPermission]
	db *gorm.DB
}

func NewIntegrationGatewayAPIKeyPermissionRepository(db *gorm.DB) *IntegrationGatewayAPIKeyPermissionRepository {
	if db == nil {
		panic("integration gateway api key permission repository requires non-nil db")
	}
	return &IntegrationGatewayAPIKeyPermissionRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.IntegrationGatewayAPIKeyPermission](db),
		db:             db,
	}
}

func (r *IntegrationGatewayAPIKeyPermissionRepository) ListByAPIKeyUUID(ctx context.Context, keyUUID uuid.UUID) ([]models.IntegrationGatewayAPIKeyPermission, error) {
	var items []models.IntegrationGatewayAPIKeyPermission
	if err := r.db.WithContext(ctx).
		Where("api_key_uuid = ?", keyUUID).
		Order("created_at ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *IntegrationGatewayAPIKeyPermissionRepository) ReplaceAll(ctx context.Context, keyUUID uuid.UUID, items []models.IntegrationGatewayAPIKeyPermission) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("api_key_uuid = ?", keyUUID).Delete(&models.IntegrationGatewayAPIKeyPermission{}).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Create(&items).Error
	})
}

func (r *IntegrationGatewayAPIKeyPermissionRepository) HasPermission(
	ctx context.Context,
	keyUUID uuid.UUID,
	scope string,
	action string,
	resourceType string,
	resource string,
) (bool, error) {
	var items []models.IntegrationGatewayAPIKeyPermission
	err := r.db.WithContext(ctx).
		Where("api_key_uuid = ? AND effect = ?", keyUUID, "allow").
		Where("scope = ? AND action = ? AND resource_type = ?", strings.TrimSpace(scope), strings.TrimSpace(action), strings.TrimSpace(resourceType)).
		Find(&items).Error
	if err != nil {
		return false, err
	}
	resource = strings.TrimSpace(resource)
	for i := range items {
		if resourcePatternMatch(items[i].ResourcePattern, resource) {
			return true, nil
		}
	}
	return false, nil
}

func resourcePatternMatch(pattern string, resource string) bool {
	pattern = strings.TrimSpace(pattern)
	resource = strings.TrimSpace(resource)
	if pattern == "" || pattern == "*" {
		return true
	}
	if strings.EqualFold(pattern, resource) {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(resource, prefix)
	}
	return false
}
