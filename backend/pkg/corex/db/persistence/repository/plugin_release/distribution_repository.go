package plugin_release

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// DistributionRepository aggregates persistence for offline packages and marketplace listings.
type DistributionRepository struct {
	packages *baseRepo.BaseRepository[models.OfflineDistributionPackage]
	listings *baseRepo.BaseRepository[models.MarketplaceListing]
	db       *gorm.DB
}

// NewDistributionRepository returns a new repository instance.
func NewDistributionRepository(db *gorm.DB) *DistributionRepository {
	if db == nil {
		panic("distribution repository requires non-nil db")
	}
	return &DistributionRepository{
		packages: baseRepo.NewBaseRepository[models.OfflineDistributionPackage](db),
		listings: baseRepo.NewBaseRepository[models.MarketplaceListing](db),
		db:       db,
	}
}

// CreatePackage persists a new offline package record.
func (r *DistributionRepository) CreatePackage(ctx context.Context, pkg *models.OfflineDistributionPackage) (*models.OfflineDistributionPackage, error) {
	if pkg == nil {
		return nil, gorm.ErrInvalidData
	}
	return r.packages.Create(ctx, pkg)
}

// UpdatePackageStatus updates package status and optional metadata.
func (r *DistributionRepository) UpdatePackageStatus(ctx context.Context, id uint64, status string, healthCheck string, slaDeadline *time.Time) error {
	if id == 0 {
		return gorm.ErrInvalidData
	}
	data := map[string]interface{}{
		"status": strings.ToLower(status),
	}
	if healthCheck != "" {
		data["health_check_log"] = healthCheck
	}
	if slaDeadline != nil {
		data["sla_deadline"] = *slaDeadline
	}
	return r.db.WithContext(ctx).
		Model(&models.OfflineDistributionPackage{}).
		Where("id = ?", id).
		Updates(data).Error
}

// GetPackageByID fetches package by primary key.
func (r *DistributionRepository) GetPackageByID(ctx context.Context, id uint64) (*models.OfflineDistributionPackage, error) {
	if id == 0 {
		return nil, gorm.ErrInvalidData
	}
	var pkg models.OfflineDistributionPackage
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Take(&pkg).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &pkg, nil
}

// GetListingByID fetches a marketplace listing by primary key.
func (r *DistributionRepository) GetListingByID(ctx context.Context, id uint64) (*models.MarketplaceListing, error) {
	if id == 0 {
		return nil, gorm.ErrInvalidData
	}
	var listing models.MarketplaceListing
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Take(&listing).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &listing, nil
}

// CreateListing creates a new marketplace listing referencing the offline package.
func (r *DistributionRepository) CreateListing(ctx context.Context, listing *models.MarketplaceListing) (*models.MarketplaceListing, error) {
	if listing == nil {
		return nil, gorm.ErrInvalidData
	}
	return r.listings.Create(ctx, listing)
}

// UpdateListingReview updates review status, counts and escalation timestamp.
func (r *DistributionRepository) UpdateListingReview(ctx context.Context, listingID uint64, status string, reviewCount int, escalatedAt *time.Time) error {
	if listingID == 0 {
		return gorm.ErrInvalidData
	}
	update := map[string]interface{}{
		"review_status": strings.ToLower(status),
		"review_count":  reviewCount,
	}
	if escalatedAt != nil {
		update["escalated_at"] = *escalatedAt
	}
	return r.db.WithContext(ctx).
		Model(&models.MarketplaceListing{}).
		Where("id = ?", listingID).
		Updates(update).Error
}

// UpdateListingNotification stores notification ticket references and publish timestamp.
func (r *DistributionRepository) UpdateListingNotification(ctx context.Context, listingID uint64, ticket *uuid.UUID, publishedAt *time.Time) error {
	if listingID == 0 {
		return gorm.ErrInvalidData
	}
	update := map[string]interface{}{}
	if ticket != nil {
		update["notification_ticket"] = *ticket
	}
	if publishedAt != nil {
		update["published_at"] = *publishedAt
	}
	if len(update) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&models.MarketplaceListing{}).
		Where("id = ?", listingID).
		Updates(update).Error
}

// ListListingsByPackage returns listings for a given offline package.
func (r *DistributionRepository) ListListingsByPackage(ctx context.Context, packageID uint64) ([]models.MarketplaceListing, error) {
	var listings []models.MarketplaceListing
	err := r.db.WithContext(ctx).
		Where("offline_package_id = ?", packageID).
		Order("created_at ASC").
		Find(&listings).Error
	if err != nil {
		return nil, err
	}
	return listings, nil
}
