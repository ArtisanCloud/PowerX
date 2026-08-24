package workflow

import (
	"context"
	"errors"
	"sort"
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

type DefinitionListFilter struct {
	TenantUUID string
	Status     []string
	Keyword    string
	SourceType string
	Category   string
	Limit      int
	Offset     int
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

// ListVersionsByWorkflow 返回同一工作流的全部定义版本。
func (r *DefinitionRepository) ListVersionsByWorkflow(ctx context.Context, tenantUUID string, definitionUUID uuid.UUID) ([]modelworkflow.WorkflowDefinition, error) {
	source, err := r.GetByUUID(ctx, tenantUUID, definitionUUID, nil)
	if err != nil {
		return nil, err
	}
	tenantUUID = strings.TrimSpace(strings.ToLower(tenantUUID))
	sourceType := strings.TrimSpace(strings.ToLower(source.SourceType))
	packKey := strings.TrimSpace(source.WorkflowPackKey)
	name := strings.TrimSpace(source.Name)

	query := r.db.WithContext(ctx).
		Where("tenant_uuid = ?", tenantUUID).
		Where("LOWER(source_type) = ?", sourceType)
	if packKey != "" {
		query = query.Where("workflow_pack_key = ?", packKey)
	} else {
		if name == "" {
			return nil, errors.New("workflow definition name is required")
		}
		query = query.Where("name = ?", name)
	}

	var defs []modelworkflow.WorkflowDefinition
	if err := query.Order("version DESC, updated_at DESC").Find(&defs).Error; err != nil {
		return nil, err
	}
	return defs, nil
}

// ListByTenant 按条件分页查询定义。
func (r *DefinitionRepository) ListByTenant(ctx context.Context, filter DefinitionListFilter) ([]modelworkflow.WorkflowDefinition, int64, error) {
	tenantUUID := strings.TrimSpace(strings.ToLower(filter.TenantUUID))
	if tenantUUID == "" {
		return nil, 0, errors.New("tenant uuid is required")
	}

	q := r.db.WithContext(ctx).Model(&modelworkflow.WorkflowDefinition{}).Where("tenant_uuid = ?", tenantUUID)
	if len(filter.Status) > 0 {
		q = q.Where("status IN ?", filter.Status)
	}
	if sourceType := strings.TrimSpace(strings.ToLower(filter.SourceType)); sourceType != "" {
		q = q.Where("LOWER(source_type) = ?", sourceType)
	}
	if category := strings.TrimSpace(strings.ToLower(filter.Category)); category != "" {
		if category == "uncategorized" {
			q = q.Where("COALESCE(NULLIF(TRIM(metadata ->> 'category'), ''), 'uncategorized') = ?", category)
		} else {
			q = q.Where("LOWER(metadata ->> 'category') = ?", category)
		}
	}
	if kw := strings.TrimSpace(filter.Keyword); kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		q = q.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", like, like)
	}

	var candidates []modelworkflow.WorkflowDefinition
	if err := q.Order("version DESC, updated_at DESC").Find(&candidates).Error; err != nil {
		return nil, 0, err
	}

	defs := latestWorkflowDefinitions(candidates)
	total := int64(len(defs))

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(defs) {
		return []modelworkflow.WorkflowDefinition{}, total, nil
	}
	end := offset + limit
	if end > len(defs) {
		end = len(defs)
	}
	return defs[offset:end], total, nil
}

func latestWorkflowDefinitions(candidates []modelworkflow.WorkflowDefinition) []modelworkflow.WorkflowDefinition {
	latestByWorkflow := make(map[string]modelworkflow.WorkflowDefinition, len(candidates))
	for _, def := range candidates {
		key := workflowDefinitionListKey(def)
		if key == "" {
			continue
		}
		current, exists := latestByWorkflow[key]
		if !exists || def.Version > current.Version || (def.Version == current.Version && def.UpdatedAt.After(current.UpdatedAt)) {
			latestByWorkflow[key] = def
		}
	}

	defs := make([]modelworkflow.WorkflowDefinition, 0, len(latestByWorkflow))
	for _, def := range latestByWorkflow {
		defs = append(defs, def)
	}
	sort.SliceStable(defs, func(i, j int) bool {
		if defs[i].UpdatedAt.Equal(defs[j].UpdatedAt) {
			return defs[i].Version > defs[j].Version
		}
		return defs[i].UpdatedAt.After(defs[j].UpdatedAt)
	})
	return defs
}

func workflowDefinitionListKey(def modelworkflow.WorkflowDefinition) string {
	sourceType := strings.TrimSpace(strings.ToLower(def.SourceType))
	packKey := strings.TrimSpace(strings.ToLower(def.WorkflowPackKey))
	if packKey != "" {
		return sourceType + "|pack|" + packKey
	}
	name := strings.TrimSpace(strings.ToLower(def.Name))
	if name == "" {
		return ""
	}
	return sourceType + "|name|" + name
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
