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

// PlanRevisionRepository stores immutable runtime decisions only. It neither
// authorizes a revision nor mutates a prior revision.
type PlanRevisionRepository struct {
	*coreRepo.BaseRepository[dbmodel.AgentPlanRevision]
	db *gorm.DB
}

func NewPlanRevisionRepository(db *gorm.DB) *PlanRevisionRepository {
	return &PlanRevisionRepository{BaseRepository: coreRepo.NewBaseRepository[dbmodel.AgentPlanRevision](db), db: db}
}

func (r *PlanRevisionRepository) Create(ctx context.Context, row *dbmodel.AgentPlanRevision) error {
	if r == nil || r.db == nil || row == nil {
		return fmt.Errorf("plan revision repository is not configured")
	}
	if strings.TrimSpace(row.Env) == "" || strings.TrimSpace(row.TenantUUID) == "" || row.RunUUID == uuid.Nil || row.SnapshotUUID == uuid.Nil || strings.TrimSpace(row.ReasonCode) == "" || len(row.Plan) == 0 {
		return fmt.Errorf("plan revision identity is required")
	}
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *PlanRevisionRepository) ListByRun(ctx context.Context, env, tenantUUID string, runUUID uuid.UUID) ([]dbmodel.AgentPlanRevision, error) {
	if r == nil || r.db == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || runUUID == uuid.Nil {
		return nil, fmt.Errorf("plan revision scope is required")
	}
	var rows []dbmodel.AgentPlanRevision
	err := r.db.WithContext(ctx).Where("env = ? AND tenant_uuid = ? AND run_uuid = ?", strings.TrimSpace(env), strings.TrimSpace(tenantUUID), runUUID).Order("created_at ASC").Find(&rows).Error
	return rows, err
}

func (r *PlanRevisionRepository) GetScoped(ctx context.Context, env, tenantUUID string, runUUID, revisionUUID uuid.UUID) (*dbmodel.AgentPlanRevision, error) {
	if r == nil || r.db == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || runUUID == uuid.Nil || revisionUUID == uuid.Nil {
		return nil, fmt.Errorf("plan revision scope is required")
	}
	var row dbmodel.AgentPlanRevision
	if err := r.db.WithContext(ctx).Where("env = ? AND tenant_uuid = ? AND run_uuid = ? AND uuid = ?", env, tenantUUID, runUUID, revisionUUID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
