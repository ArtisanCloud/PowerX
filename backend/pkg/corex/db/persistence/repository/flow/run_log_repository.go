package flow

// pkg/corex/db/persistence/repository/flow/run_log_repository.go

import (
	"context"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type AgentPlanRunRepository struct {
	*repository.BaseRepository[models.AgentPlanRun]
}

type AgentTaskEventRepository struct {
	*repository.BaseRepository[models.AgentTaskEvent]
}

func NewAgentPlanRunRepository(db *gorm.DB) *AgentPlanRunRepository {
	return &AgentPlanRunRepository{BaseRepository: repository.NewBaseRepository[models.AgentPlanRun](db)}
}
func NewAgentTaskEventRepository(db *gorm.DB) *AgentTaskEventRepository {
	return &AgentTaskEventRepository{BaseRepository: repository.NewBaseRepository[models.AgentTaskEvent](db)}
}

// UpsertStart: plan.start 到达时创建/更新 head 记录
func (r *AgentPlanRunRepository) UpsertStart(ctx context.Context, rec *models.AgentPlanRun) error {
	db := r.BaseRepository.DB.WithContext(ctx)
	// 确保字段
	if rec.Status == "" {
		rec.Status = "running"
	}
	if rec.StartedAt.IsZero() {
		rec.StartedAt = time.Now()
	}
	// plan_id 作为冲突键
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "plan_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"request_id":  rec.RequestID,
			"trace_id":    rec.TraceID,
			"tenant_uuid": rec.TenantUUID,
			"user_id":     rec.UserID,
			"customer_id": rec.CustomerID,
			"status":      rec.Status,
			"started_at":  rec.StartedAt,
			"meta":        rec.Meta,
			"updated_at":  gorm.Expr("NOW()"),
		}),
	}).Create(rec).Error
}

// MarkEnd: plan.end 时更新状态 + 结束时间
func (r *AgentPlanRunRepository) MarkEnd(ctx context.Context, planID, status string, endedAt time.Time, meta any) error {
	db := r.BaseRepository.DB.WithContext(ctx)
	if status == "" {
		status = "completed"
	}
	return db.Model(&models.AgentPlanRun{}).
		Where("plan_id = ?", planID).
		Updates(map[string]any{
			"status":     status,
			"ended_at":   endedAt,
			"meta":       meta,
			"updated_at": gorm.Expr("NOW()"),
		}).Error
}

// Insert: 任务事件行
func (r *AgentTaskEventRepository) Insert(ctx context.Context, e *models.AgentTaskEvent) error {
	return r.BaseRepository.DB.WithContext(ctx).Create(e).Error
}
