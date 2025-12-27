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

// CapabilitySyncJobRepository 管理 CapabilitySyncJob 模型。
type CapabilitySyncJobRepository struct {
	*baseRepo.BaseRepository[models.CapabilitySyncJob]
	db *gorm.DB
}

// CapabilitySyncJobFilter 支持按插件/状态查询任务。
type CapabilitySyncJobFilter struct {
	PluginID     string
	CapabilityID string
	Status       []string
	Limit        int
	Offset       int
	OrderBy      string
}

// NewCapabilitySyncJobRepository 创建仓储实例。
func NewCapabilitySyncJobRepository(db *gorm.DB) *CapabilitySyncJobRepository {
	if db == nil {
		panic("capability sync job repository requires DB")
	}
	return &CapabilitySyncJobRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.CapabilitySyncJob](db),
		db:             db,
	}
}

// Create 插入新的任务记录。
func (r *CapabilitySyncJobRepository) Create(ctx context.Context, job *models.CapabilitySyncJob) (*models.CapabilitySyncJob, error) {
	if job == nil {
		return nil, errors.New("capability sync job payload is nil")
	}
	return r.BaseRepository.Create(ctx, job)
}

// GetByUUID 根据 UUID 查询任务。
func (r *CapabilitySyncJobRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*models.CapabilitySyncJob, error) {
	if id == uuid.Nil {
		return nil, errors.New("job uuid is required")
	}
	var job models.CapabilitySyncJob
	err := r.db.WithContext(ctx).
		Where("uuid = ?", id).
		First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSyncJobNotFound
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// List 根据过滤条件列出任务。
func (r *CapabilitySyncJobRepository) List(ctx context.Context, filter CapabilitySyncJobFilter) ([]models.CapabilitySyncJob, error) {
	query := r.db.WithContext(ctx).Model(&models.CapabilitySyncJob{})

	if filter.PluginID != "" {
		query = query.Where("plugin_id = ?", filter.PluginID)
	}
	if filter.CapabilityID != "" {
		query = query.Where("capability_id = ?", filter.CapabilityID)
	}
	if len(filter.Status) > 0 {
		query = query.Where("status IN ?", filter.Status)
	}

	orderBy := strings.TrimSpace(filter.OrderBy)
	if orderBy == "" {
		orderBy = "started_at DESC"
	}
	query = query.Order(orderBy)

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var jobs []models.CapabilitySyncJob
	if err := query.Find(&jobs).Error; err != nil {
		return nil, err
	}
	return jobs, nil
}

// UpdateFields 更新任务部分字段，常用于写入状态、hash 与错误摘要。
func (r *CapabilitySyncJobRepository) UpdateFields(ctx context.Context, id uuid.UUID, fields map[string]interface{}) error {
	if id == uuid.Nil {
		return errors.New("job uuid is required")
	}
	if len(fields) == 0 {
		return nil
	}
	result := r.db.WithContext(ctx).
		Model(&models.CapabilitySyncJob{}).
		Where("uuid = ?", id).
		Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSyncJobNotFound
	}
	return nil
}
