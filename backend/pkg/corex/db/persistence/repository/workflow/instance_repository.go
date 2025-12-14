package workflow

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// InstanceRepository 负责工作流实例的持久化读写。
type InstanceRepository struct {
	*repository.BaseRepository[modelworkflow.WorkflowInstance]
	db *gorm.DB
}

// InstanceListFilter 用于筛选实例列表。
type InstanceListFilter struct {
	TenantUUID     string
	DefinitionUUID uuid.UUID
	State          string
	From           *time.Time
	To             *time.Time
	AgentUUID      uuid.UUID
	IncludeTags    datatypes.JSON
	PageSize       int
	Page           int
	Keyword        string
}

// NewInstanceRepository 创建实例仓储。
func NewInstanceRepository(db *gorm.DB) *InstanceRepository {
	return &InstanceRepository{
		BaseRepository: repository.NewBaseRepository[modelworkflow.WorkflowInstance](db),
		db:             db,
	}
}

// CreateInstance 持久化新的工作流实例。
func (r *InstanceRepository) CreateInstance(ctx context.Context, instance *modelworkflow.WorkflowInstance) (*modelworkflow.WorkflowInstance, error) {
	if instance == nil {
		return nil, errors.New("workflow instance payload is nil")
	}
	return r.BaseRepository.Create(ctx, instance)
}

// GetByUUID 通过 UUID 查询单个实例。
func (r *InstanceRepository) GetByUUID(ctx context.Context, tenantUUID string, instanceUUID uuid.UUID) (*modelworkflow.WorkflowInstance, error) {
	if strings.TrimSpace(tenantUUID) == "" {
		return nil, errors.New("tenant uuid is required")
	}
	if instanceUUID == uuid.Nil {
		return nil, errors.New("instance uuid is required")
	}

	var instance modelworkflow.WorkflowInstance
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND uuid = ?", strings.ToLower(strings.TrimSpace(tenantUUID)), instanceUUID).
		First(&instance).Error
	if err != nil {
		return nil, err
	}
	return &instance, nil
}

// ListInstances 根据过滤条件分页查询实例。
func (r *InstanceRepository) ListInstances(ctx context.Context, filter InstanceListFilter) ([]modelworkflow.WorkflowInstance, int64, error) {
	tenantUUID := strings.ToLower(strings.TrimSpace(filter.TenantUUID))
	if tenantUUID == "" {
		return nil, 0, errors.New("tenant uuid is required")
	}

	query := r.db.WithContext(ctx).Model(&modelworkflow.WorkflowInstance{}).Where("tenant_uuid = ?", tenantUUID)
	if filter.DefinitionUUID != uuid.Nil {
		query = query.Where("definition_uuid = ?", filter.DefinitionUUID)
	}
	if filter.State != "" {
		query = query.Where("state = ?", filter.State)
	}
	if filter.AgentUUID != uuid.Nil {
		stepTable := coremodel.PowerXSchema + "." + coremodel.TableWorkflowStepRecords
		query = query.Where(
			"uuid IN (?)",
			r.db.Table(stepTable).
				Select("instance_uuid").
				Where("subject_uuid = ?", filter.AgentUUID).
				Group("instance_uuid"),
		)
	}
	if filter.From != nil {
		query = query.Where("created_at >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where("created_at < ?", *filter.To)
	}
	if filter.IncludeTags != nil && len(filter.IncludeTags) > 0 {
		query = query.Where("tags @> ?", filter.IncludeTags)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}

	var instances []modelworkflow.WorkflowInstance
	err := query.
		Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&instances).Error
	return instances, total, err
}

// UpdateState 更新实例状态及关联字段。
func (r *InstanceRepository) UpdateState(ctx context.Context, tenantUUID string, instanceUUID uuid.UUID, nextState string, updates map[string]interface{}) error {
	if strings.TrimSpace(tenantUUID) == "" || instanceUUID == uuid.Nil {
		return errors.New("invalid parameters for update")
	}
	if updates == nil {
		updates = map[string]interface{}{}
	}
	if nextState != "" {
		updates["state"] = nextState
		updates["last_transition_at"] = time.Now().UTC()
	}

	return r.db.WithContext(ctx).
		Model(&modelworkflow.WorkflowInstance{}).
		Where("tenant_uuid = ? AND uuid = ?", strings.ToLower(strings.TrimSpace(tenantUUID)), instanceUUID).
		Updates(updates).Error
}
