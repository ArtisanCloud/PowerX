package runtimescheduler

import (
	"context"
	"errors"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	baserepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type JobFilter struct {
	TenantUUID string
	OwnerType  string
	OwnerID    string
	Status     string
	Page       int
	PageSize   int
}

type JobRepository struct {
	base *baserepo.BaseRepository[models.SchedulerJob]
	db   *gorm.DB
}

func NewJobRepository(db *gorm.DB) *JobRepository {
	return &JobRepository{base: baserepo.NewBaseRepository[models.SchedulerJob](db), db: db}
}

func (r *JobRepository) Create(ctx context.Context, job *models.SchedulerJob) (*models.SchedulerJob, error) {
	return r.base.Create(ctx, job)
}

func (r *JobRepository) Update(ctx context.Context, job *models.SchedulerJob) (*models.SchedulerJob, error) {
	expected := job.Revision
	res := r.db.WithContext(ctx).Model(&models.SchedulerJob{}).Where("uuid = ? AND tenant_uuid = ? AND revision = ? AND status <> ?", job.UUID, job.TenantUUID, expected, models.JobStatusDeleted).Updates(map[string]any{
		"name": job.Name, "schedule_type": job.ScheduleType, "schedule_expr": job.ScheduleExpr, "timezone": job.Timezone, "topic": job.Topic, "payload_json": job.PayloadJSON, "status": job.Status, "next_run_at": job.NextRunAt, "last_run_at": job.LastRunAt, "misfire_policy": job.MisfirePolicy, "overlap_policy": job.OverlapPolicy, "idempotency_key": job.IdempotencyKey, "actor_type": job.ActorType, "actor_user_id": job.ActorUserID, "actor_user_uuid": job.ActorUserUUID, "actor_member_id": job.ActorMemberID, "actor_member_uuid": job.ActorMemberUUID, "updated_by": job.UpdatedBy, "last_error": job.LastError, "trace_id": job.TraceID, "revision": gorm.Expr("revision + 1"),
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, ErrRevisionConflict
	}
	job.Revision = expected + 1
	return r.FindByUUID(ctx, job.UUID)
}

func (r *JobRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*models.SchedulerJob, error) {
	var row models.SchedulerJob
	err := r.db.WithContext(ctx).Where("uuid = ?", id).Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *JobRepository) List(ctx context.Context, filter JobFilter) ([]*models.SchedulerJob, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.SchedulerJob{}).Where("status <> ?", models.JobStatusDeleted)
	if v := strings.TrimSpace(filter.TenantUUID); v != "" {
		query = query.Where("tenant_uuid = ?", v)
	}
	if v := strings.TrimSpace(filter.OwnerType); v != "" {
		query = query.Where("owner_type = ?", v)
	}
	if v := strings.TrimSpace(filter.OwnerID); v != "" {
		query = query.Where("owner_id = ?", v)
	}
	if v := strings.TrimSpace(filter.Status); v != "" {
		query = query.Where("status = ?", v)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if filter.PageSize > 0 {
		query = query.Limit(filter.PageSize)
	}
	if filter.Page > 0 && filter.PageSize > 0 {
		query = query.Offset((filter.Page - 1) * filter.PageSize)
	}
	var rows []*models.SchedulerJob
	if err := query.Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *JobRepository) ListDue(ctx context.Context, now time.Time, limit int) ([]*models.SchedulerJob, error) {
	query := r.db.WithContext(ctx).
		Where("status = ?", models.JobStatusActive).
		Where("next_run_at IS NOT NULL AND next_run_at <= ?", now.UTC())
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []*models.SchedulerJob
	if err := query.Order("next_run_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *JobRepository) ClaimDue(ctx context.Context, id uuid.UUID, dueAt time.Time) (*models.SchedulerJob, error) {
	var row models.SchedulerJob
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("uuid = ?", id).
			Take(&row).Error; err != nil {
			return err
		}
		if row.Status != models.JobStatusActive || row.NextRunAt == nil || row.NextRunAt.After(dueAt.UTC()) {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *JobRepository) UpdateFields(ctx context.Context, id uuid.UUID, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&models.SchedulerJob{}).
		Where("uuid = ?", id).
		Updates(fields).Error
}

var ErrRevisionConflict = errors.New("scheduler job revision conflict")

func (r *JobRepository) FindHistorical(ctx context.Context, tenant string, id uuid.UUID) (*models.SchedulerJob, error) {
	var row models.SchedulerJob
	err := r.db.WithContext(ctx).Unscoped().Where("tenant_uuid = ? AND uuid = ?", tenant, id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
func (r *JobRepository) SoftDelete(ctx context.Context, row *models.SchedulerJob, operator, trace string, now time.Time) error {
	res := r.db.WithContext(ctx).Model(&models.SchedulerJob{}).Where("uuid = ? AND tenant_uuid = ? AND revision = ? AND status <> ?", row.UUID, row.TenantUUID, row.Revision, models.JobStatusDeleted).Updates(map[string]any{"status": models.JobStatusDeleted, "actor_type": row.ActorType, "actor_user_id": row.ActorUserID, "actor_user_uuid": row.ActorUserUUID, "actor_member_id": row.ActorMemberID, "actor_member_uuid": row.ActorMemberUUID, "deleted_at": now, "next_run_at": nil, "updated_by": operator, "trace_id": trace, "revision": gorm.Expr("revision + 1")})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrRevisionConflict
	}
	row.Status = models.JobStatusDeleted
	row.DeletedAt = gorm.DeletedAt{Time: now, Valid: true}
	row.NextRunAt = nil
	row.Revision++
	row.UpdatedBy = operator
	row.TraceID = trace
	return nil
}
