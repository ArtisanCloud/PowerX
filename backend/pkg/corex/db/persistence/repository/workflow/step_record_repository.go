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

// StepRecordRepository 管理工作流步骤记录。
type StepRecordRepository struct {
	*repository.BaseRepository[modelworkflow.WorkflowStepRecord]
	db *gorm.DB
}

// NewStepRecordRepository 创建仓储实例。
func NewStepRecordRepository(db *gorm.DB) *StepRecordRepository {
	return &StepRecordRepository{
		BaseRepository: repository.NewBaseRepository[modelworkflow.WorkflowStepRecord](db),
		db:             db,
	}
}

// AppendRecord 追加新的步骤记录。
func (r *StepRecordRepository) AppendRecord(ctx context.Context, record *modelworkflow.WorkflowStepRecord) (*modelworkflow.WorkflowStepRecord, error) {
	if record == nil {
		return nil, errors.New("step record payload is nil")
	}
	return r.BaseRepository.Create(ctx, record)
}

// GetByID 根据主键查询步骤记录。
func (r *StepRecordRepository) GetByID(ctx context.Context, id uint64) (*modelworkflow.WorkflowStepRecord, error) {
	if id == 0 {
		return nil, errors.New("step record id is required")
	}
	var record modelworkflow.WorkflowStepRecord
	if err := r.db.WithContext(ctx).First(&record, id).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// ListByInstance 列出某实例下的所有记录，按创建时间排序。
func (r *StepRecordRepository) ListByInstance(ctx context.Context, instanceUUID uuid.UUID) ([]modelworkflow.WorkflowStepRecord, error) {
	if instanceUUID == uuid.Nil {
		return nil, errors.New("instance uuid is required")
	}
	var records []modelworkflow.WorkflowStepRecord
	err := r.db.WithContext(ctx).
		Where("instance_uuid = ?", instanceUUID).
		Order("scheduled_at ASC, id ASC").
		Find(&records).Error
	return records, err
}

// FindLatestByStep 获取某步骤最近一次记录。
func (r *StepRecordRepository) FindLatestByStep(ctx context.Context, instanceUUID uuid.UUID, stepID string) (*modelworkflow.WorkflowStepRecord, error) {
	if instanceUUID == uuid.Nil {
		return nil, errors.New("instance uuid is required")
	}
	if strings.TrimSpace(stepID) == "" {
		return nil, errors.New("step_id is required")
	}

	var record modelworkflow.WorkflowStepRecord
	err := r.db.WithContext(ctx).
		Where("instance_uuid = ? AND step_id = ?", instanceUUID, stepID).
		Order("id DESC").
		First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// UpdateState 更新步骤状态与附加字段。
func (r *StepRecordRepository) UpdateState(ctx context.Context, id uint64, nextState string, updates map[string]interface{}) error {
	if id == 0 {
		return errors.New("step record id is required")
	}
	if updates == nil {
		updates = map[string]interface{}{}
	}
	if nextState != "" {
		updates["state"] = nextState
	}
	updates["last_transition_at"] = time.Now().UTC()

	return r.db.WithContext(ctx).
		Model(&modelworkflow.WorkflowStepRecord{}).
		Where("id = ?", id).
		Updates(updates).Error
}
