package workflow

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// CompensationRepository 管理补偿步骤记录。
type CompensationRepository struct {
	*repository.BaseRepository[modelworkflow.WorkflowStepCompensation]
	db *gorm.DB
}

// NewCompensationRepository 创建仓储实例。
func NewCompensationRepository(db *gorm.DB) *CompensationRepository {
	return &CompensationRepository{
		BaseRepository: repository.NewBaseRepository[modelworkflow.WorkflowStepCompensation](db),
		db:             db,
	}
}

// CreateCompensation 创建补偿记录。
func (r *CompensationRepository) CreateCompensation(ctx context.Context, comp *modelworkflow.WorkflowStepCompensation) (*modelworkflow.WorkflowStepCompensation, error) {
	if comp == nil {
		return nil, errors.New("compensation payload is nil")
	}
	return r.BaseRepository.Create(ctx, comp)
}

// ListByInstance 列出某实例下的补偿记录。
func (r *CompensationRepository) ListByInstance(ctx context.Context, instanceUUID uuid.UUID) ([]modelworkflow.WorkflowStepCompensation, error) {
	if instanceUUID == uuid.Nil {
		return nil, errors.New("instance uuid is required")
	}
	compTable := (&modelworkflow.WorkflowStepCompensation{}).TableName()
	stepTable := (&modelworkflow.WorkflowStepRecord{}).TableName()

	var comps []modelworkflow.WorkflowStepCompensation
	err := r.db.WithContext(ctx).
		Table(compTable).
		Joins("JOIN "+stepTable+" sr ON sr.id = "+compTable+".step_record_id").
		Where("sr.instance_uuid = ?", instanceUUID).
		Order(compTable + ".id DESC").
		Select(compTable + ".*").
		Find(&comps).Error
	return comps, err
}

// UpdateState 更新补偿状态。
func (r *CompensationRepository) UpdateState(ctx context.Context, id uint64, state string, updates map[string]interface{}) error {
	if id == 0 {
		return errors.New("compensation id is required")
	}
	if updates == nil {
		updates = map[string]interface{}{}
	}
	if state != "" {
		updates["state"] = state
	}
	return r.db.WithContext(ctx).
		Model(&modelworkflow.WorkflowStepCompensation{}).
		Where("id = ?", id).
		Updates(updates).Error
}
