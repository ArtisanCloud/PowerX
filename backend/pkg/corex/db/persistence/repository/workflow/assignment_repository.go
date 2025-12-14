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

// AgentAssignmentRepository 管理步骤派发记录。
type AgentAssignmentRepository struct {
	*repository.BaseRepository[modelworkflow.AgentAssignment]
	db *gorm.DB
}

// NewAgentAssignmentRepository 创建仓储实例。
func NewAgentAssignmentRepository(db *gorm.DB) *AgentAssignmentRepository {
	return &AgentAssignmentRepository{
		BaseRepository: repository.NewBaseRepository[modelworkflow.AgentAssignment](db),
		db:             db,
	}
}

// CreateAssignment 记录新的派发。
func (r *AgentAssignmentRepository) CreateAssignment(ctx context.Context, assignment *modelworkflow.AgentAssignment) (*modelworkflow.AgentAssignment, error) {
	if assignment == nil {
		return nil, errors.New("assignment payload is nil")
	}
	return r.BaseRepository.Create(ctx, assignment)
}

// GetLatestByStep 获取某步骤最新的派发记录。
func (r *AgentAssignmentRepository) GetLatestByStep(ctx context.Context, stepRecordID uint64) (*modelworkflow.AgentAssignment, error) {
	if stepRecordID == 0 {
		return nil, errors.New("stepRecordID is required")
	}

	var assignment modelworkflow.AgentAssignment
	err := r.db.WithContext(ctx).
		Where("step_record_id = ?", stepRecordID).
		Order("id DESC").
		First(&assignment).Error
	if err != nil {
		return nil, err
	}
	return &assignment, nil
}

// FindOpenAssignments 查找仍在进行中的派发。
func (r *AgentAssignmentRepository) FindOpenAssignments(ctx context.Context, agentUUID uuid.UUID, statuses []string, limit int) ([]modelworkflow.AgentAssignment, error) {
	if agentUUID == uuid.Nil {
		return nil, errors.New("agent uuid is required")
	}
	if len(statuses) == 0 {
		statuses = []string{"dispatched", "acknowledged"}
	}
	if limit <= 0 {
		limit = 50
	}

	var assignments []modelworkflow.AgentAssignment
	err := r.db.WithContext(ctx).
		Where("agent_uuid = ? AND status IN ?", agentUUID, statuses).
		Order("dispatched_at ASC").
		Limit(limit).
		Find(&assignments).Error
	return assignments, err
}

// UpdateStatus 更新派发状态及时间戳。
func (r *AgentAssignmentRepository) UpdateStatus(ctx context.Context, id uint64, status string, updates map[string]interface{}) error {
	if id == 0 {
		return errors.New("assignment id is required")
	}
	if status == "" {
		return errors.New("status is required")
	}
	if updates == nil {
		updates = map[string]interface{}{}
	}
	updates["status"] = status

	now := time.Now().UTC()
	switch status {
	case "acknowledged":
		updates["acknowledged_at"] = now
	case "timeout":
		updates["acknowledged_at"] = gorm.Expr("COALESCE(acknowledged_at, ?)", now)
		updates["completed_at"] = now
	case "completed", "reassigned":
		updates["completed_at"] = now
	}

	return r.db.WithContext(ctx).
		Model(&modelworkflow.AgentAssignment{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// FindTimedOutAssignments 查找在 Ack 截止前未响应的派发。
func (r *AgentAssignmentRepository) FindTimedOutAssignments(ctx context.Context, tenantUUID string, before time.Time, limit int) ([]modelworkflow.AgentAssignment, error) {
	tenantUUID = strings.TrimSpace(strings.ToLower(tenantUUID))
	if tenantUUID == "" {
		return nil, errors.New("tenant uuid is required")
	}
	if limit <= 0 {
		limit = 50
	}

	assignmentTable := (&modelworkflow.AgentAssignment{}).TableName()
	stepTable := (&modelworkflow.WorkflowStepRecord{}).TableName()
	instanceTable := (&modelworkflow.WorkflowInstance{}).TableName()

	var assignments []modelworkflow.AgentAssignment
	err := r.db.WithContext(ctx).
		Table(assignmentTable).
		Joins("JOIN "+stepTable+" sr ON sr.id = "+assignmentTable+".step_record_id").
		Joins("JOIN "+instanceTable+" inst ON inst.uuid = sr.instance_uuid").
		Where(assignmentTable+".status IN ?", []string{"dispatched", "acknowledged"}).
		Where(assignmentTable+".ack_deadline IS NOT NULL").
		Where(assignmentTable+".ack_deadline <= ?", before).
		Where("inst.tenant_uuid = ?", tenantUUID).
		Order(assignmentTable + ".ack_deadline ASC").
		Limit(limit).
		Select(assignmentTable + ".*").
		Scan(&assignments).Error
	return assignments, err
}
