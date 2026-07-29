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

// LeaseQueuedSteps leases queued step records for a runner worker.
func (r *StepRecordRepository) LeaseQueuedSteps(ctx context.Context, limit int, leaseOwner string, leaseUntil time.Time) ([]modelworkflow.WorkflowStepRecord, error) {
	return r.leaseQueuedSteps(ctx, uuid.Nil, limit, leaseOwner, leaseUntil)
}

// LeaseQueuedStepsByInstance leases queued step records for a single workflow instance.
func (r *StepRecordRepository) LeaseQueuedStepsByInstance(ctx context.Context, instanceUUID uuid.UUID, limit int, leaseOwner string, leaseUntil time.Time) ([]modelworkflow.WorkflowStepRecord, error) {
	if instanceUUID == uuid.Nil {
		return nil, errors.New("instance uuid is required")
	}
	return r.leaseQueuedSteps(ctx, instanceUUID, limit, leaseOwner, leaseUntil)
}

func (r *StepRecordRepository) leaseQueuedSteps(ctx context.Context, instanceUUID uuid.UUID, limit int, leaseOwner string, leaseUntil time.Time) ([]modelworkflow.WorkflowStepRecord, error) {
	leaseOwner = strings.TrimSpace(leaseOwner)
	if leaseOwner == "" {
		return nil, errors.New("lease owner is required")
	}
	if leaseUntil.IsZero() {
		return nil, errors.New("lease_until is required")
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	var leased []modelworkflow.WorkflowStepRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var records []modelworkflow.WorkflowStepRecord
		now := time.Now().UTC()
		query := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("state = ?", "queued").
			Where("scheduled_at <= ?", now).
			Where("(lease_until IS NULL OR lease_until < ?)", now)
		if instanceUUID != uuid.Nil {
			query = query.Where("instance_uuid = ?", instanceUUID)
		}
		if err := query.Order("scheduled_at ASC, id ASC").Limit(limit).Find(&records).Error; err != nil {
			return err
		}
		if len(records) == 0 {
			leased = nil
			return nil
		}
		ids := make([]uint64, 0, len(records))
		for _, record := range records {
			ids = append(ids, record.ID)
		}
		updates := map[string]interface{}{
			"state":              "in_progress",
			"lease_owner":        leaseOwner,
			"lease_until":        leaseUntil.UTC(),
			"started_at":         now,
			"last_transition_at": now,
		}
		if err := tx.Model(&modelworkflow.WorkflowStepRecord{}).
			Where("id IN ?", ids).
			Where("state = ?", "queued").
			Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", ids).Order("scheduled_at ASC, id ASC").Find(&leased).Error; err != nil {
			return err
		}
		return nil
	})
	return leased, err
}

// UpdateStateForAttempt updates a step only when the attempt still matches.
func (r *StepRecordRepository) UpdateStateForAttempt(ctx context.Context, id uint64, attempt int32, nextState string, updates map[string]interface{}) (bool, error) {
	if id == 0 {
		return false, errors.New("step record id is required")
	}
	if attempt < 0 {
		return false, errors.New("attempt must be non-negative")
	}
	if updates == nil {
		updates = map[string]interface{}{}
	}
	if nextState != "" {
		updates["state"] = nextState
	}
	now := time.Now().UTC()
	updates["last_transition_at"] = now
	switch nextState {
	case "completed", "failed", "compensated":
		updates["completed_at"] = now
		updates["lease_owner"] = ""
		updates["lease_until"] = nil
	}

	result := r.db.WithContext(ctx).
		Model(&modelworkflow.WorkflowStepRecord{}).
		Where("id = ? AND attempt = ?", id, attempt).
		Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
