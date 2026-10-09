package runtimescheduler

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/runtime_scheduler"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *Service) withJobFence(ctx context.Context, jobID string, fn func(*Service) error) error {
	if s == nil {
		return appErr(503, "SCHEDULER_UNAVAILABLE", "调度服务不可用", nil)
	}
	id, err := uuid.Parse(jobID)
	if err != nil || id == uuid.Nil {
		return appErr(400, "SCHEDULER_INVALID_JOB_ID", "任务UUID无效", err)
	}
	err = s.jobs.WithJobFence(ctx, id, func(db *gorm.DB) error {
		scoped := *s
		scoped.db = db
		scoped.jobs = repo.NewJobRepository(db)
		scoped.runs = repo.NewRunRepository(db)
		return fn(&scoped)
	})
	if err != nil {
		if dto.CodeOf(err) != "" {
			return err
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return appErr(409, "SCHEDULER_JOB_BUSY", "任务正在发布或修改，请重新预览后重试", err)
		}
		return mapDBErr(err)
	}
	return nil
}
func (s *Service) UpdateJob(ctx context.Context, in UpdateJobInput) (out *models.SchedulerJob, err error) {
	err = s.withJobFence(ctx, in.JobID, func(locked *Service) error { out, err = locked.updateJob(ctx, in); return err })
	return
}
func (s *Service) ResumeJob(ctx context.Context, id, operator, trace string) (out *models.SchedulerJob, err error) {
	err = s.withJobFence(ctx, id, func(locked *Service) error { out, err = locked.resumeJob(ctx, id, operator, trace); return err })
	return
}
func (s *Service) TriggerJob(ctx context.Context, id, operator, trace string) (out *TriggerResult, err error) {
	err = s.withJobFence(ctx, id, func(locked *Service) error { out, err = locked.triggerJob(ctx, id, operator, trace); return err })
	return
}
func (s *Service) setStatus(ctx context.Context, id, status, operator, trace string) (out *models.SchedulerJob, err error) {
	err = s.withJobFence(ctx, id, func(locked *Service) error {
		out, err = locked.setStatusLocked(ctx, id, status, operator, trace)
		return err
	})
	return
}
func (s *Service) dispatchDueJob(ctx context.Context, id uuid.UUID, now time.Time) (out bool, err error) {
	err = s.withJobFence(ctx, id.String(), func(locked *Service) error { out, err = locked.dispatchDueJobLocked(ctx, id, now); return err })
	return
}
func (s *Service) findHistoricalJob(ctx context.Context, id string) (*models.SchedulerJob, error) {
	tenant, err := reqctx.RequireTenantUUID(ctx)
	if err != nil {
		return nil, appErr(401, "SCHEDULER_UNAUTHORIZED", "缺少租户身份", err)
	}
	parsed, err := uuid.Parse(id)
	if err != nil || parsed == uuid.Nil {
		return nil, appErr(400, "SCHEDULER_INVALID_JOB_ID", "任务UUID无效", err)
	}
	row, err := s.jobs.FindHistorical(ctx, tenant, parsed)
	if err != nil {
		return nil, mapDBErr(err)
	}
	if row == nil {
		return nil, appErr(404, "SCHEDULER_JOB_NOT_FOUND", "任务不存在", nil)
	}
	return row, nil
}
