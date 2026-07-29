package workflow

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// HumanReviewTaskListFilter 描述审核任务分页查询条件。
type HumanReviewTaskListFilter struct {
	TenantUUID           string
	Status               string
	WorkflowInstanceUUID uuid.UUID
	ReviewType           string
	Page                 int
	PageSize             int
}

// HumanReviewTaskRepository 管理 Workflow human.review 审核任务。
type HumanReviewTaskRepository struct {
	*repository.BaseRepository[modelworkflow.HumanReviewTask]
	db *gorm.DB
}

// NewHumanReviewTaskRepository 创建审核任务仓储。
func NewHumanReviewTaskRepository(db *gorm.DB) *HumanReviewTaskRepository {
	return &HumanReviewTaskRepository{
		BaseRepository: repository.NewBaseRepository[modelworkflow.HumanReviewTask](db),
		db:             db,
	}
}

// CreateTask 创建审核任务。
func (r *HumanReviewTaskRepository) CreateTask(ctx context.Context, task *modelworkflow.HumanReviewTask) (*modelworkflow.HumanReviewTask, error) {
	if task == nil {
		return nil, errors.New("human review task payload is nil")
	}
	return r.BaseRepository.Create(ctx, task)
}

// GetByUUID 按租户与 UUID 查询审核任务。
func (r *HumanReviewTaskRepository) GetByUUID(ctx context.Context, tenantUUID string, taskUUID uuid.UUID) (*modelworkflow.HumanReviewTask, error) {
	tenantUUID = strings.TrimSpace(strings.ToLower(tenantUUID))
	if tenantUUID == "" {
		return nil, errors.New("tenant uuid is required")
	}
	if taskUUID == uuid.Nil {
		return nil, errors.New("review task uuid is required")
	}
	var task modelworkflow.HumanReviewTask
	if err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND uuid = ?", tenantUUID, taskUUID).
		First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// ListTasks 按条件分页查询审核任务。
func (r *HumanReviewTaskRepository) ListTasks(ctx context.Context, filter HumanReviewTaskListFilter) ([]modelworkflow.HumanReviewTask, int64, error) {
	tenantUUID := strings.TrimSpace(strings.ToLower(filter.TenantUUID))
	if tenantUUID == "" {
		return nil, 0, errors.New("tenant uuid is required")
	}
	q := r.db.WithContext(ctx).Model(&modelworkflow.HumanReviewTask{}).Where("tenant_uuid = ?", tenantUUID)
	if status := strings.TrimSpace(strings.ToLower(filter.Status)); status != "" {
		q = q.Where("status = ?", status)
	}
	if filter.WorkflowInstanceUUID != uuid.Nil {
		q = q.Where("workflow_instance_uuid = ?", filter.WorkflowInstanceUUID)
	}
	if reviewType := strings.TrimSpace(filter.ReviewType); reviewType != "" {
		q = q.Where("review_type = ?", reviewType)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	var tasks []modelworkflow.HumanReviewTask
	err := q.Order("created_at DESC, id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&tasks).Error
	return tasks, total, err
}

// UpdateDecision 更新审核任务状态与审核结果。
func (r *HumanReviewTaskRepository) UpdateDecision(ctx context.Context, tenantUUID string, taskUUID uuid.UUID, status string, updates map[string]interface{}) error {
	tenantUUID = strings.TrimSpace(strings.ToLower(tenantUUID))
	if tenantUUID == "" || taskUUID == uuid.Nil {
		return errors.New("tenant uuid and review task uuid are required")
	}
	if updates == nil {
		updates = map[string]interface{}{}
	}
	if status != "" {
		updates["status"] = strings.TrimSpace(strings.ToLower(status))
	}
	if _, ok := updates["completed_at"]; !ok {
		switch strings.TrimSpace(strings.ToLower(status)) {
		case "approved", "rejected", "changes_requested", "canceled", "expired":
			updates["completed_at"] = time.Now().UTC()
		}
	}
	return r.db.WithContext(ctx).
		Model(&modelworkflow.HumanReviewTask{}).
		Where("tenant_uuid = ? AND uuid = ?", tenantUUID, taskUUID).
		Updates(updates).Error
}
