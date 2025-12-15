package workflow

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

var (
	// ErrWorkflowDefinitionNotFound 在未命中工作流定义时返回。
	ErrWorkflowDefinitionNotFound = gorm.ErrRecordNotFound
)

// DefinitionRepository 封装工作流定义的持久化访问。
type DefinitionRepository struct {
	*repository.BaseRepository[modelworkflow.WorkflowDefinition]
	db *gorm.DB
}

// NewDefinitionRepository 创建仓储实例。
func NewDefinitionRepository(db *gorm.DB) *DefinitionRepository {
	return &DefinitionRepository{
		BaseRepository: repository.NewBaseRepository[modelworkflow.WorkflowDefinition](db),
		db:             db,
	}
}

// CreateDefinition 按照租户与名称建立新的定义版本。
func (r *DefinitionRepository) CreateDefinition(ctx context.Context, def *modelworkflow.WorkflowDefinition) (*modelworkflow.WorkflowDefinition, error) {
	if def == nil {
		return nil, errors.New("workflow definition payload is nil")
	}
	return r.BaseRepository.Create(ctx, def)
}

// NextVersion 计算指定名称的下一版本号。
func (r *DefinitionRepository) NextVersion(ctx context.Context, tenantUUID string, name string) (int32, error) {
	tenantUUID = strings.TrimSpace(strings.ToLower(tenantUUID))
	if tenantUUID == "" {
		return 0, errors.New("tenant uuid is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("definition name is required")
	}

	var current int32
	err := r.db.WithContext(ctx).
		Model(&modelworkflow.WorkflowDefinition{}).
		Where("tenant_uuid = ? AND name = ?", tenantUUID, name).
		Select("COALESCE(MAX(version), 0)").
		Scan(&current).Error
	if err != nil {
		return 0, err
	}
	return current + 1, nil
}

// GetByUUID 根据 UUID（可选版本）检索定义。
func (r *DefinitionRepository) GetByUUID(ctx context.Context, tenantUUID string, definitionUUID uuid.UUID, version *int32) (*modelworkflow.WorkflowDefinition, error) {
	tenantUUID = strings.TrimSpace(strings.ToLower(tenantUUID))
	if tenantUUID == "" {
		return nil, errors.New("tenant uuid is required")
	}
	if definitionUUID == uuid.Nil {
		return nil, errors.New("definition UUID is required")
	}

	query := r.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, definitionUUID)
	if version != nil && *version > 0 {
		query = query.Where("version = ?", *version)
	}

	var def modelworkflow.WorkflowDefinition
	if err := query.First(&def).Error; err != nil {
		return nil, err
	}
	return &def, nil
}

// GetLatestPublished 获取最新已发布的定义版本。
func (r *DefinitionRepository) GetLatestPublished(ctx context.Context, tenantUUID string, definitionUUID uuid.UUID) (*modelworkflow.WorkflowDefinition, error) {
	query := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND uuid = ?", strings.TrimSpace(strings.ToLower(tenantUUID)), definitionUUID).
		Where("status = ?", "published").
		Order("version DESC")

	var def modelworkflow.WorkflowDefinition
	if err := query.First(&def).Error; err != nil {
		return nil, err
	}
	return &def, nil
}

// ListByTenant 按条件分页查询定义。
func (r *DefinitionRepository) ListByTenant(ctx context.Context, tenantUUID string, status []string, keyword string, limit, offset int) ([]modelworkflow.WorkflowDefinition, int64, error) {
	tenantUUID = strings.TrimSpace(strings.ToLower(tenantUUID))
	if tenantUUID == "" {
		return nil, 0, errors.New("tenant uuid is required")
	}

	q := r.db.WithContext(ctx).Model(&modelworkflow.WorkflowDefinition{}).Where("tenant_uuid = ?", tenantUUID)
	if len(status) > 0 {
		q = q.Where("status IN ?", status)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		q = q.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var defs []modelworkflow.WorkflowDefinition
	err := q.Order("updated_at DESC, version DESC").Limit(limit).Offset(offset).Find(&defs).Error
	return defs, total, err
}

// UpdateStatus 更新状态及相关字段。
func (r *DefinitionRepository) UpdateStatus(ctx context.Context, tenantUUID string, definitionUUID uuid.UUID, version int32, status string, updates map[string]interface{}) error {
	if strings.TrimSpace(tenantUUID) == "" || definitionUUID == uuid.Nil || version <= 0 {
		return errors.New("invalid parameters for updating workflow definition")
	}
	if updates == nil {
		updates = map[string]interface{}{}
	}
	if status != "" {
		updates["status"] = status
	}

	return r.db.WithContext(ctx).
		Model(&modelworkflow.WorkflowDefinition{}).
		Where("tenant_uuid = ? AND uuid = ? AND version = ?", strings.TrimSpace(strings.ToLower(tenantUUID)), definitionUUID, version).
		Updates(updates).
		Error
}
