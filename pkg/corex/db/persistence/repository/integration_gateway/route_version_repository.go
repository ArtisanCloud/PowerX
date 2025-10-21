package integration_gateway

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// IntegrationRouteVersionRepository 负责路由快照的存取。
type IntegrationRouteVersionRepository struct {
	*baseRepo.BaseRepository[models.IntegrationRouteVersion]
	db *gorm.DB
}

func NewIntegrationRouteVersionRepository(db *gorm.DB) *IntegrationRouteVersionRepository {
	if db == nil {
		panic("integration route version repository requires non-nil db")
	}
	return &IntegrationRouteVersionRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.IntegrationRouteVersion](db),
		db:             db,
	}
}

func (r *IntegrationRouteVersionRepository) Create(ctx context.Context, version *models.IntegrationRouteVersion) (*models.IntegrationRouteVersion, error) {
	if version == nil {
		return nil, gorm.ErrInvalidData
	}
	if err := r.db.WithContext(ctx).Create(version).Error; err != nil {
		return nil, err
	}
	return version, nil
}

func (r *IntegrationRouteVersionRepository) ListByRoute(ctx context.Context, routeUUID uuid.UUID, limit int) ([]models.IntegrationRouteVersion, error) {
	query := r.db.WithContext(ctx).
		Where("route_uuid = ?", routeUUID).
		Order("version DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var versions []models.IntegrationRouteVersion
	if err := query.Find(&versions).Error; err != nil {
		return nil, err
	}
	return versions, nil
}
