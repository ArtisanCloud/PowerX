package workflow

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// WorkflowPackInstallationRepository 管理租户内置 Workflow Pack 安装状态。
type WorkflowPackInstallationRepository struct {
	*repository.BaseRepository[modelworkflow.WorkflowPackInstallation]
	db *gorm.DB
}

// NewWorkflowPackInstallationRepository 创建 Workflow Pack 安装状态仓储。
func NewWorkflowPackInstallationRepository(db *gorm.DB) *WorkflowPackInstallationRepository {
	return &WorkflowPackInstallationRepository{
		BaseRepository: repository.NewBaseRepository[modelworkflow.WorkflowPackInstallation](db),
		db:             db,
	}
}

// GetByTenantKey 查询租户内指定 Workflow Pack 的安装状态。
func (r *WorkflowPackInstallationRepository) GetByTenantKey(ctx context.Context, tenantUUID string, workflowKey string) (*modelworkflow.WorkflowPackInstallation, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	workflowKey = strings.TrimSpace(workflowKey)
	if tenantUUID == "" {
		return nil, errors.New("tenant uuid is required")
	}
	if workflowKey == "" {
		return nil, errors.New("workflow_key is required")
	}
	var installation modelworkflow.WorkflowPackInstallation
	if err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND workflow_key = ?", tenantUUID, workflowKey).
		First(&installation).Error; err != nil {
		return nil, err
	}
	return &installation, nil
}

// UpsertEnabled 写入或更新租户内置 Workflow Pack 的启用状态。
func (r *WorkflowPackInstallationRepository) UpsertEnabled(ctx context.Context, installation *modelworkflow.WorkflowPackInstallation) (*modelworkflow.WorkflowPackInstallation, error) {
	if installation == nil {
		return nil, errors.New("workflow pack installation payload is nil")
	}
	installation.Status = modelworkflow.WorkflowPackInstallationStatusEnabled
	now := time.Now().UTC()
	if installation.InstalledAt == nil {
		installation.InstalledAt = &now
	}
	installation.RemovedAt = nil
	installation.RemovedBy = uuid.Nil
	installation.LastAction = "install"
	if err := installation.BeforeSave(r.db); err != nil {
		return nil, err
	}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "tenant_uuid"},
				{Name: "workflow_key"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"version",
				"checksum",
				"status",
				"definition_uuid",
				"definition_version",
				"source",
				"installed_at",
				"removed_at",
				"removed_by",
				"last_seeded_at",
				"last_action",
				"updated_at",
			}),
		}).
		Create(installation).Error
	if err != nil {
		return nil, err
	}
	return r.GetByTenantKey(ctx, installation.TenantUUID, installation.WorkflowKey)
}

// MarkDeleted 将租户内置 Workflow Pack 标记为删除，不再被自动重新生成。
func (r *WorkflowPackInstallationRepository) MarkDeleted(ctx context.Context, tenantUUID string, workflowKey string, actorUUID uuid.UUID) error {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	workflowKey = strings.TrimSpace(workflowKey)
	if tenantUUID == "" {
		return errors.New("tenant uuid is required")
	}
	if workflowKey == "" {
		return errors.New("workflow_key is required")
	}
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).
		Model(&modelworkflow.WorkflowPackInstallation{}).
		Where("tenant_uuid = ? AND workflow_key = ?", tenantUUID, workflowKey).
		Updates(map[string]interface{}{
			"status":      modelworkflow.WorkflowPackInstallationStatusDeleted,
			"removed_at":  now,
			"removed_by":  actorUUID,
			"last_action": "delete",
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListByTenant 分页列出租户内置 Workflow Pack 安装状态。
func (r *WorkflowPackInstallationRepository) ListByTenant(ctx context.Context, tenantUUID string, keyword string, limit, offset int) ([]modelworkflow.WorkflowPackInstallation, int64, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	if tenantUUID == "" {
		return nil, 0, errors.New("tenant uuid is required")
	}
	q := r.db.WithContext(ctx).
		Model(&modelworkflow.WorkflowPackInstallation{}).
		Where("tenant_uuid = ?", tenantUUID)
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
	var installations []modelworkflow.WorkflowPackInstallation
	err := q.Order("workflow_key ASC").Limit(limit).Offset(offset).Find(&installations).Error
	return installations, total, err
}
