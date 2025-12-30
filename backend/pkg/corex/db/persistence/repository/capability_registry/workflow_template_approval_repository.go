package capability_registry

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// WorkflowTemplateApprovalRepository 管理模板升级审批记录。
type WorkflowTemplateApprovalRepository struct {
	*baseRepo.BaseRepository[models.WorkflowTemplateApproval]
	db *gorm.DB
}

// NewWorkflowTemplateApprovalRepository 创建仓储实例。
func NewWorkflowTemplateApprovalRepository(db *gorm.DB) *WorkflowTemplateApprovalRepository {
	if db == nil {
		panic("workflow template approval repository requires DB")
	}
	return &WorkflowTemplateApprovalRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.WorkflowTemplateApproval](db),
		db:             db,
	}
}

// Upsert 保存或更新审批记录。
func (r *WorkflowTemplateApprovalRepository) Upsert(ctx context.Context, approval *models.WorkflowTemplateApproval) (*models.WorkflowTemplateApproval, error) {
	if approval == nil {
		return nil, errors.New("workflow template approval payload is nil")
	}
	return r.BaseRepository.Upsert(ctx, approval, []clause.Column{{Name: "template_id"}})
}

// GetByTemplateID 返回指定模板的审批记录。
func (r *WorkflowTemplateApprovalRepository) GetByTemplateID(ctx context.Context, templateID string) (*models.WorkflowTemplateApproval, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, errors.New("template_id is required")
	}
	var record models.WorkflowTemplateApproval
	err := r.db.WithContext(ctx).
		Where("template_id = ?", templateID).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWorkflowTemplateApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// DeleteStale 在模板被删除或重新同步时，可清理旧审批记录。
func (r *WorkflowTemplateApprovalRepository) DeleteStale(ctx context.Context, templateIDs []string, before time.Time) error {
	if len(templateIDs) == 0 {
		return nil
	}
	trimmed := make([]string, 0, len(templateIDs))
	for _, id := range templateIDs {
		if v := strings.TrimSpace(id); v != "" {
			trimmed = append(trimmed, v)
		}
	}
	if len(trimmed) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("template_id IN ? AND approved_at < ?", trimmed, before).
		Delete(&models.WorkflowTemplateApproval{}).Error
}

// ListAll 返回所有审批记录，供批量映射使用。
func (r *WorkflowTemplateApprovalRepository) ListAll(ctx context.Context) ([]models.WorkflowTemplateApproval, error) {
	var approvals []models.WorkflowTemplateApproval
	err := r.db.WithContext(ctx).
		Order("approved_at DESC").
		Find(&approvals).Error
	return approvals, err
}
