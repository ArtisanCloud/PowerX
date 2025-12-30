package capability_registry

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// WorkflowTemplateRepository 管理 WorkflowTemplateRef 模型。
type WorkflowTemplateRepository struct {
	*baseRepo.BaseRepository[models.WorkflowTemplateRef]
	db *gorm.DB
}

// NewWorkflowTemplateRepository 创建仓储实例。
func NewWorkflowTemplateRepository(db *gorm.DB) *WorkflowTemplateRepository {
	if db == nil {
		panic("workflow template repository requires DB")
	}
	return &WorkflowTemplateRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.WorkflowTemplateRef](db),
		db:             db,
	}
}

// Upsert 插入或更新模板。
func (r *WorkflowTemplateRepository) Upsert(ctx context.Context, template *models.WorkflowTemplateRef) (*models.WorkflowTemplateRef, error) {
	if template == nil {
		return nil, errors.New("workflow template payload is nil")
	}
	return r.BaseRepository.Upsert(ctx, template, []clause.Column{{Name: "template_id"}})
}

// UpsertBatch 批量插入或更新模板。
func (r *WorkflowTemplateRepository) UpsertBatch(ctx context.Context, templates []*models.WorkflowTemplateRef) ([]*models.WorkflowTemplateRef, error) {
	if len(templates) == 0 {
		return nil, nil
	}
	return r.BaseRepository.UpsertBatch(ctx, templates, []clause.Column{{Name: "template_id"}})
}

// ListByCapabilityID 查询某能力下的所有模板。
func (r *WorkflowTemplateRepository) ListByCapabilityID(ctx context.Context, capabilityID string) ([]models.WorkflowTemplateRef, error) {
	capabilityID = strings.TrimSpace(capabilityID)
	if capabilityID == "" {
		return nil, errors.New("capability_id is required")
	}
	var templates []models.WorkflowTemplateRef
	err := r.db.WithContext(ctx).
		Where("capability_id = ?", capabilityID).
		Order("name ASC").
		Find(&templates).Error
	return templates, err
}

// GetByTemplateID 通过 template_id 查询。
func (r *WorkflowTemplateRepository) GetByTemplateID(ctx context.Context, templateID string) (*models.WorkflowTemplateRef, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, errors.New("template_id is required")
	}
	var template models.WorkflowTemplateRef
	err := r.db.WithContext(ctx).
		Where("template_id = ?", templateID).
		First(&template).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWorkflowTemplateNotFound
	}
	if err != nil {
		return nil, err
	}
	return &template, nil
}

// DeleteByCapabilityID 删除能力关联的模板。
func (r *WorkflowTemplateRepository) DeleteByCapabilityID(ctx context.Context, capabilityID string) error {
	capabilityID = strings.TrimSpace(capabilityID)
	if capabilityID == "" {
		return errors.New("capability_id is required")
	}
	return r.db.WithContext(ctx).
		Where("capability_id = ?", capabilityID).
		Delete(&models.WorkflowTemplateRef{}).Error
}

// ListAll 返回所有模板，供 Workflow Catalog 等组件生成全量快照。
func (r *WorkflowTemplateRepository) ListAll(ctx context.Context) ([]models.WorkflowTemplateRef, error) {
	var templates []models.WorkflowTemplateRef
	err := r.db.WithContext(ctx).
		Order("capability_id ASC, template_id ASC").
		Find(&templates).Error
	return templates, err
}
