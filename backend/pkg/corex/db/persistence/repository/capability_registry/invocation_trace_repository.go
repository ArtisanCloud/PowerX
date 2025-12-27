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

// InvocationTraceRepository 管理能力调用追踪记录。
type InvocationTraceRepository struct {
	*baseRepo.BaseRepository[models.InvocationTrace]
	db *gorm.DB
}

// InvocationTraceFilter 用于筛选追踪记录。
type InvocationTraceFilter struct {
	TenantUUID   string
	CapabilityID string
	PluginID     string
	Status       []string
	Limit        int
	Offset       int
	OrderBy      string
}

// NewInvocationTraceRepository 创建仓储实例。
func NewInvocationTraceRepository(db *gorm.DB) *InvocationTraceRepository {
	if db == nil {
		panic("invocation trace repository requires DB")
	}
	return &InvocationTraceRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.InvocationTrace](db),
		db:             db,
	}
}

// Create 插入新的追踪记录。
func (r *InvocationTraceRepository) Create(ctx context.Context, trace *models.InvocationTrace) (*models.InvocationTrace, error) {
	if trace == nil {
		return nil, errors.New("invocation trace payload is nil")
	}
	return r.BaseRepository.Create(ctx, trace)
}

// GetByTraceID 根据 trace_id 查询。
func (r *InvocationTraceRepository) GetByTraceID(ctx context.Context, traceID string) (*models.InvocationTrace, error) {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return nil, errors.New("trace_id is required")
	}
	var record models.InvocationTrace
	err := r.db.WithContext(ctx).
		Where("trace_id = ?", traceID).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvocationTraceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// List 根据过滤条件列出追踪记录。
func (r *InvocationTraceRepository) List(ctx context.Context, filter InvocationTraceFilter) ([]models.InvocationTrace, error) {
	query := r.db.WithContext(ctx).Model(&models.InvocationTrace{})

	if filter.TenantUUID != "" {
		query = query.Where("tenant_uuid = ?", filter.TenantUUID)
	}
	if filter.CapabilityID != "" {
		query = query.Where("capability_id = ?", filter.CapabilityID)
	}
	if filter.PluginID != "" {
		query = query.Where("plugin_id = ?", filter.PluginID)
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

	var records []models.InvocationTrace
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// MarkEventPublished 更新追踪记录的事件状态。
func (r *InvocationTraceRepository) MarkEventPublished(ctx context.Context, traceID string, publicationID uuid.UUID) error {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return errors.New("trace_id is required")
	}
	payload := map[string]interface{}{
		"event_published": true,
	}
	if publicationID != uuid.Nil {
		payload["event_publication_id"] = publicationID
	}
	result := r.db.WithContext(ctx).
		Model(&models.InvocationTrace{}).
		Where("trace_id = ?", traceID).
		Updates(payload)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrInvocationTraceNotFound
	}
	return nil
}
