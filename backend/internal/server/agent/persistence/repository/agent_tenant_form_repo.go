package repository

import (
	"context"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AgentTenantFormRepository 负责租户表单存储。
type AgentTenantFormRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentTenantForm]
}

// NewAgentTenantFormRepository 构造 Repo。
func NewAgentTenantFormRepository(db *gorm.DB) *AgentTenantFormRepository {
	return &AgentTenantFormRepository{
		BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentTenantForm](db),
	}
}

// ListByTenant 返回租户表单列表，可选状态过滤。
func (r *AgentTenantFormRepository) ListByTenant(ctx context.Context, tenantID string, statuses []string) ([]dbmodel.AgentTenantForm, error) {
	query := r.DB.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	var forms []dbmodel.AgentTenantForm
	if err := query.Order("created_at DESC").Find(&forms).Error; err != nil {
		return nil, err
	}
	return forms, nil
}

// GetByUUID 返回表单。
func (r *AgentTenantFormRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*dbmodel.AgentTenantForm, error) {
	var form dbmodel.AgentTenantForm
	if err := r.DB.WithContext(ctx).Where("uuid = ?", id).First(&form).Error; err != nil {
		return nil, err
	}
	return &form, nil
}

// Create 插入表单。
func (r *AgentTenantFormRepository) Create(ctx context.Context, form *dbmodel.AgentTenantForm) (*dbmodel.AgentTenantForm, error) {
	if err := r.DB.WithContext(ctx).Create(form).Error; err != nil {
		return nil, err
	}
	return form, nil
}

// Save 保存表单。
func (r *AgentTenantFormRepository) Save(ctx context.Context, form *dbmodel.AgentTenantForm) (*dbmodel.AgentTenantForm, error) {
	if err := r.DB.WithContext(ctx).Save(form).Error; err != nil {
		return nil, err
	}
	return form, nil
}
