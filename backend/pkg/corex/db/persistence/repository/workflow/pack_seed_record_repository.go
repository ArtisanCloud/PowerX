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

// WorkflowPackSeedRecordRepository 管理 Workflow Pack seed 版本记录。
type WorkflowPackSeedRecordRepository struct {
	*repository.BaseRepository[modelworkflow.WorkflowPackSeedRecord]
	db *gorm.DB
}

// NewWorkflowPackSeedRecordRepository 创建 Workflow Pack seed 仓储。
func NewWorkflowPackSeedRecordRepository(db *gorm.DB) *WorkflowPackSeedRecordRepository {
	return &WorkflowPackSeedRecordRepository{
		BaseRepository: repository.NewBaseRepository[modelworkflow.WorkflowPackSeedRecord](db),
		db:             db,
	}
}

// CreateRecord 创建 seed 记录。
func (r *WorkflowPackSeedRecordRepository) CreateRecord(ctx context.Context, record *modelworkflow.WorkflowPackSeedRecord) (*modelworkflow.WorkflowPackSeedRecord, error) {
	if record == nil {
		return nil, errors.New("workflow pack seed record payload is nil")
	}
	return r.BaseRepository.Create(ctx, record)
}

// GetLatestByKey 查询指定租户/平台下某 workflow_key 的最新 seed 记录。
func (r *WorkflowPackSeedRecordRepository) GetLatestByKey(ctx context.Context, tenantUUID string, workflowKey string) (*modelworkflow.WorkflowPackSeedRecord, error) {
	tenantUUID = strings.TrimSpace(strings.ToLower(tenantUUID))
	if tenantUUID == "" {
		return nil, errors.New("tenant uuid is required")
	}
	workflowKey = strings.TrimSpace(workflowKey)
	if workflowKey == "" {
		return nil, errors.New("workflow_key is required")
	}
	q := r.db.WithContext(ctx).Where("tenant_uuid = ? AND workflow_key = ?", tenantUUID, workflowKey)
	var record modelworkflow.WorkflowPackSeedRecord
	if err := q.Order("version DESC, id DESC").First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// ListByTenant 列出指定租户或平台 seed 记录。
func (r *WorkflowPackSeedRecordRepository) ListByTenant(ctx context.Context, tenantUUID string, keyword string, limit, offset int) ([]modelworkflow.WorkflowPackSeedRecord, int64, error) {
	tenantUUID = strings.TrimSpace(strings.ToLower(tenantUUID))
	if tenantUUID == "" {
		return nil, 0, errors.New("tenant uuid is required")
	}
	q := r.db.WithContext(ctx).Model(&modelworkflow.WorkflowPackSeedRecord{}).Where("tenant_uuid = ?", tenantUUID)
	if kw := strings.TrimSpace(keyword); kw != "" {
		q = q.Where("LOWER(workflow_key) LIKE ?", "%"+strings.ToLower(kw)+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	var records []modelworkflow.WorkflowPackSeedRecord
	err := q.Order("workflow_key ASC, version DESC").Limit(limit).Offset(offset).Find(&records).Error
	return records, total, err
}

// ListByDefinition 查询指定定义关联的 seed 记录。
func (r *WorkflowPackSeedRecordRepository) ListByDefinition(ctx context.Context, definitionUUID uuid.UUID) ([]modelworkflow.WorkflowPackSeedRecord, error) {
	if definitionUUID == uuid.Nil {
		return nil, errors.New("definition_uuid is required")
	}
	var records []modelworkflow.WorkflowPackSeedRecord
	err := r.db.WithContext(ctx).
		Where("definition_uuid = ?", definitionUUID).
		Order("version DESC, id DESC").
		Find(&records).Error
	return records, err
}
