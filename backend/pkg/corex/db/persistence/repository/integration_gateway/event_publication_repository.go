package integration_gateway

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// IntegrationEventPublicationRepository 负责事件发布记录的持久化。
type IntegrationEventPublicationRepository struct {
	*baseRepo.BaseRepository[models.IntegrationEventPublication]
	db *gorm.DB
}

func NewIntegrationEventPublicationRepository(db *gorm.DB) *IntegrationEventPublicationRepository {
	if db == nil {
		panic("integration event publication repository requires non-nil db")
	}
	return &IntegrationEventPublicationRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.IntegrationEventPublication](db),
		db:             db,
	}
}

func (r *IntegrationEventPublicationRepository) Create(ctx context.Context, publication *models.IntegrationEventPublication) (*models.IntegrationEventPublication, error) {
	if publication == nil {
		return nil, gorm.ErrInvalidData
	}
	if err := r.db.WithContext(ctx).Create(publication).Error; err != nil {
		return nil, err
	}
	return publication, nil
}

func (r *IntegrationEventPublicationRepository) MarkPublished(ctx context.Context, uuid uuid.UUID, publishedAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&models.IntegrationEventPublication{}).
		Where("uuid = ?", uuid).
		Updates(map[string]interface{}{
			"status":       "sent",
			"publish_time": publishedAt,
			"last_error":   "",
			"updated_at":   time.Now(),
		}).Error
}

func (r *IntegrationEventPublicationRepository) MarkFailed(ctx context.Context, uuid uuid.UUID, attempts int, errMsg string) error {
	return r.db.WithContext(ctx).
		Model(&models.IntegrationEventPublication{}).
		Where("uuid = ?", uuid).
		Updates(map[string]interface{}{
			"status":     "failed",
			"attempts":   attempts,
			"last_error": errMsg,
			"updated_at": time.Now(),
		}).Error
}

func (r *IntegrationEventPublicationRepository) ListPending(ctx context.Context, limit int) ([]models.IntegrationEventPublication, error) {
	query := r.db.WithContext(ctx).
		Where("status IN ?", []string{"pending", "retrying"}).
		Order("created_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var pubs []models.IntegrationEventPublication
	if err := query.Find(&pubs).Error; err != nil {
		return nil, err
	}
	return pubs, nil
}
