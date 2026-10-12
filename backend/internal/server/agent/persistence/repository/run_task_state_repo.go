package repository

import (
	"context"
	"fmt"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
)

type RunTaskStateRepository struct{ db *gorm.DB }

func NewRunTaskStateRepository(db *gorm.DB) *RunTaskStateRepository {
	return &RunTaskStateRepository{db: db}
}
func (r *RunTaskStateRepository) Upsert(ctx context.Context, row *dbmodel.AgentRunTaskState) error {
	if r == nil || r.db == nil || row == nil || strings.TrimSpace(row.Env) == "" || strings.TrimSpace(row.TenantUUID) == "" || row.RunUUID == uuid.Nil || row.SnapshotUUID == uuid.Nil || row.PlanRevisionUUID == uuid.Nil || strings.TrimSpace(row.TaskID) == "" || strings.TrimSpace(row.Status) == "" {
		return fmt.Errorf("run task state is incomplete")
	}
	return r.db.WithContext(ctx).Where("env = ? AND tenant_uuid = ? AND run_uuid = ? AND plan_revision_uuid = ? AND task_id = ?", row.Env, row.TenantUUID, row.RunUUID, row.PlanRevisionUUID, row.TaskID).Assign(map[string]any{"status": row.Status, "artifact_ref": row.ArtifactRef}).FirstOrCreate(row).Error
}
func (r *RunTaskStateRepository) List(ctx context.Context, env, tenant string, run, revision uuid.UUID) ([]dbmodel.AgentRunTaskState, error) {
	if r == nil || r.db == nil || env == "" || tenant == "" || run == uuid.Nil || revision == uuid.Nil {
		return nil, fmt.Errorf("run task state scope is incomplete")
	}
	var rows []dbmodel.AgentRunTaskState
	err := r.db.WithContext(ctx).Where("env = ? AND tenant_uuid = ? AND run_uuid = ? AND plan_revision_uuid = ?", env, tenant, run, revision).Find(&rows).Error
	return rows, err
}
