package repository

import (
	"context"
	"fmt"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	coreRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CapabilityApprovalRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentCapabilityApproval]
	db *gorm.DB
}

func NewCapabilityApprovalRepository(db *gorm.DB) *CapabilityApprovalRepository {
	return &CapabilityApprovalRepository{BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentCapabilityApproval](db), db: db}
}

func (r *CapabilityApprovalRepository) CreateOrGetPending(ctx context.Context, row *dbmodel.AgentCapabilityApproval) (*dbmodel.AgentCapabilityApproval, error) {
	if r == nil || r.db == nil || row == nil || strings.TrimSpace(row.Env) == "" || strings.TrimSpace(row.TenantUUID) == "" || row.RunUUID == uuid.Nil || row.SnapshotUUID == uuid.Nil || row.PlanRevisionUUID == uuid.Nil || row.AgentUUID == uuid.Nil || row.SessionUUID == uuid.Nil || row.MessageUUID == uuid.Nil || row.CapabilityUUID == uuid.Nil || strings.TrimSpace(row.RequestedByUserUUID) == "" {
		return nil, fmt.Errorf("capability approval request is incomplete")
	}
	var out dbmodel.AgentCapabilityApproval
	err := r.db.WithContext(ctx).Where("env = ? AND tenant_uuid = ? AND run_uuid = ? AND snapshot_uuid = ? AND plan_revision_uuid = ? AND agent_uuid = ? AND session_uuid = ? AND message_uuid = ? AND capability_uuid = ? AND requested_by_user_uuid = ? AND status = ?", row.Env, row.TenantUUID, row.RunUUID, row.SnapshotUUID, row.PlanRevisionUUID, row.AgentUUID, row.SessionUUID, row.MessageUUID, row.CapabilityUUID, row.RequestedByUserUUID, dbmodel.AgentCapabilityApprovalPending).First(&out).Error
	if err == nil {
		return &out, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (r *CapabilityApprovalRepository) FindScoped(ctx context.Context, env, tenantUUID, subjectUUID string, agentUUID, sessionUUID, approvalUUID uuid.UUID) (*dbmodel.AgentCapabilityApproval, error) {
	if r == nil || r.db == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || strings.TrimSpace(subjectUUID) == "" || agentUUID == uuid.Nil || sessionUUID == uuid.Nil || approvalUUID == uuid.Nil {
		return nil, fmt.Errorf("capability approval scope is incomplete")
	}
	var out dbmodel.AgentCapabilityApproval
	err := r.db.WithContext(ctx).Where("uuid = ? AND env = ? AND tenant_uuid = ? AND agent_uuid = ? AND session_uuid = ? AND requested_by_user_uuid = ?", approvalUUID, env, tenantUUID, agentUUID, sessionUUID, subjectUUID).First(&out).Error
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *CapabilityApprovalRepository) FindForDecision(ctx context.Context, env, tenantUUID string, approvalUUID uuid.UUID) (*dbmodel.AgentCapabilityApproval, error) {
	if r == nil || r.db == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || approvalUUID == uuid.Nil {
		return nil, fmt.Errorf("capability approval decision scope is incomplete")
	}
	var out dbmodel.AgentCapabilityApproval
	if err := r.db.WithContext(ctx).Where("uuid = ? AND env = ? AND tenant_uuid = ?", approvalUUID, env, tenantUUID).First(&out).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *CapabilityApprovalRepository) ListForTenant(ctx context.Context, env, tenantUUID, status string, limit int) ([]dbmodel.AgentCapabilityApproval, error) {
	if r == nil || r.db == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || limit < 1 || limit > 100 {
		return nil, fmt.Errorf("capability approval list input is invalid")
	}
	query := r.db.WithContext(ctx).Where("env = ? AND tenant_uuid = ?", env, tenantUUID)
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var rows []dbmodel.AgentCapabilityApproval
	if err := query.Order("created_at ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *CapabilityApprovalRepository) Decide(ctx context.Context, row *dbmodel.AgentCapabilityApproval, approverUUID string, approved bool) error {
	if r == nil || r.db == nil || row == nil || row.UUID == uuid.Nil || strings.TrimSpace(approverUUID) == "" {
		return fmt.Errorf("capability approval decision is incomplete")
	}
	status := dbmodel.AgentCapabilityApprovalRejected
	column := "rejected_at"
	if approved {
		status = dbmodel.AgentCapabilityApprovalApproved
		column = "approved_at"
	}
	result := r.db.WithContext(ctx).Model(&dbmodel.AgentCapabilityApproval{}).Where("uuid = ? AND status = ?", row.UUID, dbmodel.AgentCapabilityApprovalPending).Updates(map[string]any{"status": status, "approved_by_user_uuid": approverUUID, column: gorm.Expr("CURRENT_TIMESTAMP")})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("capability approval is not pending")
	}
	return nil
}
