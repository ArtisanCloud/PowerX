package capability_registry

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// CapabilityEventPublicationRepository 管理能力事件投递记录。
type CapabilityEventPublicationRepository struct {
	*baseRepo.BaseRepository[models.CapabilityEventPublication]
	db *gorm.DB
}

// CapabilityEventPublicationFilter 提供按状态过滤能力。
type CapabilityEventPublicationFilter struct {
	TenantUUID string
	Topic      string
	Status     []string
	Limit      int
	Offset     int
	OrderBy    string
}

// NewCapabilityEventPublicationRepository 创建仓储实例。
func NewCapabilityEventPublicationRepository(db *gorm.DB) *CapabilityEventPublicationRepository {
	if db == nil {
		panic("capability event publication repository requires DB")
	}
	return &CapabilityEventPublicationRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.CapabilityEventPublication](db),
		db:             db,
	}
}

// Create 插入新的事件记录。
func (r *CapabilityEventPublicationRepository) Create(ctx context.Context, event *models.CapabilityEventPublication) (*models.CapabilityEventPublication, error) {
	if event == nil {
		return nil, errors.New("capability event publication payload is nil")
	}
	return r.BaseRepository.Create(ctx, event)
}

// GetByUUID 根据 UUID 查询。
func (r *CapabilityEventPublicationRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*models.CapabilityEventPublication, error) {
	if id == uuid.Nil {
		return nil, errors.New("event publication uuid is required")
	}
	var record models.CapabilityEventPublication
	err := r.db.WithContext(ctx).
		Where("uuid = ?", id).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrEventPublicationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// List 根据过滤条件列出事件记录。
func (r *CapabilityEventPublicationRepository) List(ctx context.Context, filter CapabilityEventPublicationFilter) ([]models.CapabilityEventPublication, error) {
	query := r.db.WithContext(ctx).Model(&models.CapabilityEventPublication{})

	if filter.TenantUUID != "" {
		query = query.Where("tenant_uuid = ?", filter.TenantUUID)
	}
	if filter.Topic != "" {
		query = query.Where("topic = ?", filter.Topic)
	}
	if len(filter.Status) > 0 {
		query = query.Where("status IN ?", filter.Status)
	}

	orderBy := strings.TrimSpace(filter.OrderBy)
	if orderBy == "" {
		orderBy = "created_at DESC"
	}
	query = query.Order(orderBy)

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var records []models.CapabilityEventPublication
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// UpdateFields 部分更新事件记录。
func (r *CapabilityEventPublicationRepository) UpdateFields(ctx context.Context, id uuid.UUID, fields map[string]interface{}) error {
	if id == uuid.Nil {
		return errors.New("event publication uuid is required")
	}
	if len(fields) == 0 {
		return nil
	}
	result := r.db.WithContext(ctx).
		Model(&models.CapabilityEventPublication{}).
		Where("uuid = ?", id).
		Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrEventPublicationNotFound
	}
	return nil
}
