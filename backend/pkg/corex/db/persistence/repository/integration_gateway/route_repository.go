package integration_gateway

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// IntegrationRouteRepository 负责集成网关路由主表的持久化。
type IntegrationRouteRepository struct {
	*baseRepo.BaseRepository[models.IntegrationRoute]
	db *gorm.DB
}

// NewIntegrationRouteRepository 构造仓储实例。
func NewIntegrationRouteRepository(db *gorm.DB) *IntegrationRouteRepository {
	if db == nil {
		panic("integration route repository requires non-nil db")
	}
	return &IntegrationRouteRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.IntegrationRoute](db),
		db:             db,
	}
}

// Create 在租户内创建新的集成入口，确保别名唯一。
func (r *IntegrationRouteRepository) Create(ctx context.Context, route *models.IntegrationRoute) (*models.IntegrationRoute, error) {
	if route == nil {
		return nil, gorm.ErrInvalidData
	}
	err := r.db.WithContext(ctx).Create(route).Error
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, ErrSlugConflict
		}
		return nil, err
	}
	return route, nil
}

// UpdateWithVersion 使用乐观并发控制更新路由基础信息，并可更新当前版本号。
func (r *IntegrationRouteRepository) UpdateWithVersion(
	ctx context.Context,
	routeUUID uuid.UUID,
	expectedVersion uint32,
	mutate func(route *models.IntegrationRoute),
) (*models.IntegrationRoute, error) {
	if routeUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var result models.IntegrationRoute
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("uuid = ?", routeUUID).
			First(&result).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrRouteNotFound
			}
			return err
		}

		if expectedVersion > 0 && result.CurrentVersion != expectedVersion {
			return ErrVersionConflict
		}

		if mutate != nil {
			mutate(&result)
		}
		// 若调用方希望自增版本，可直接在 mutate 中修改 CurrentVersion。
		return tx.Save(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetBySlug 根据租户与别名获取路由。
func (r *IntegrationRouteRepository) GetBySlug(ctx context.Context, tenantUUID, slug string) (*models.IntegrationRoute, error) {
	var route models.IntegrationRoute
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND route_slug = ?", tenantUUID, slug).
		First(&route).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrRouteNotFound
		}
		return nil, err
	}
	return &route, nil
}

// GetByUUID 根据 UUID 获取路由。
func (r *IntegrationRouteRepository) GetByUUID(ctx context.Context, routeUUID uuid.UUID) (*models.IntegrationRoute, error) {
	var route models.IntegrationRoute
	err := r.db.WithContext(ctx).
		Where("uuid = ?", routeUUID).
		First(&route).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrRouteNotFound
		}
		return nil, err
	}
	return &route, nil
}

// ListByTenant 按租户分页返回路由列表。
func (r *IntegrationRouteRepository) ListByTenant(ctx context.Context, tenantUUID string, offset, limit int) ([]models.IntegrationRoute, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.IntegrationRoute{}).
		Where("tenant_uuid = ?", tenantUUID).
		Order("route_slug ASC")

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

	var routes []models.IntegrationRoute
	if err := query.Find(&routes).Error; err != nil {
		return nil, 0, err
	}
	return routes, total, nil
}
