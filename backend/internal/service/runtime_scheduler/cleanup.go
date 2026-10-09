package runtimescheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/runtime_scheduler"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

const (
	CleanupReadCapabilityID   = "com.corex.scheduler.jobs.service_read"
	CleanupDeleteCapabilityID = "com.corex.scheduler.jobs.service_delete"
	CleanupReadScope          = "_scope.scheduler.jobs.service_read"
	CleanupDeleteScope        = "_scope.scheduler.jobs.service_delete"
)

type DeleteJobInput struct {
	JobUUID          string `json:"job_uuid"`
	ExpectedRevision uint64 `json:"expected_revision"`
}
type DeleteJobResult struct {
	Schema          string    `json:"schema"`
	JobUUID         string    `json:"job_uuid"`
	Status          string    `json:"status"`
	Deleted         bool      `json:"deleted"`
	AlreadyDeleted  bool      `json:"already_deleted"`
	Revision        uint64    `json:"revision"`
	DeletedAt       time.Time `json:"deleted_at"`
	HistoryRetained bool      `json:"history_retained"`
	TraceID         string    `json:"trace_id"`
}

// authorizeCleanup applies the same tenant/owner boundary to preview, receipt,
// history and deletion. API-key namespace text is never plugin identity.
func (s *Service) authorizeCleanup(ctx context.Context, ownerType, ownerID, action string) error {
	tenant, err := reqctx.RequireTenantUUID(ctx)
	if err != nil {
		return appErr(401, "SCHEDULER_UNAUTHORIZED", "缺少租户身份", err)
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil {
		return appErr(401, "SCHEDULER_UNAUTHORIZED", "缺少调用身份", nil)
	}
	if cap.ServiceCredential(ctx) {
		if ownerType != models.OwnerTypePlugin || ownerID == "" {
			return appErr(403, "SCHEDULER_OWNER_FORBIDDEN", "服务凭据仅可操作已授权插件任务", nil)
		}
		capability, scope := CleanupReadCapabilityID, CleanupReadScope
		if action == "delete" {
			capability, scope = CleanupDeleteCapabilityID, CleanupDeleteScope
		}
		access, err := cap.NewGrantStatusService(s.db).CurrentAccess(ctx)
		if err == nil {
			err = access.Require(capability)
		}
		if err != nil {
			var failure *cap.DirectGrantError
			if errors.As(err, &failure) && failure.Status != 403 {
				return appErr(failure.Status, "SCHEDULER_AUTH_UNAVAILABLE", "调度授权检查不可用", err)
			}
			return appErr(403, "SCHEDULER_CAPABILITY_FORBIDDEN", "调度能力未授权或已撤销", err)
		}
		if strings.EqualFold(claims.Issuer, "powerx-sts") {
			if claims.PluginID != ownerID {
				return appErr(403, "SCHEDULER_PLUGIN_OWNER_MISMATCH", "插件任务owner不匹配", nil)
			}
			return nil
		}
		key, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(s.db).FindActiveByHash(ctx, tenant, reqctx.AuthenticatedAPIKeyHash(ctx))
		if err != nil {
			return mapDBErr(err)
		}
		if key == nil {
			return appErr(401, "SCHEDULER_UNAUTHORIZED", "API Key已失效", nil)
		}
		grants, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(s.db).ListByAPIKeyUUID(ctx, key.UUID)
		if err != nil {
			return mapDBErr(err)
		}
		owned := grants[:0]
		for _, grant := range grants {
			if grant.PluginID == ownerID {
				owned = append(owned, grant)
			}
		}
		if !gwrepo.APIKeyPermissionGranted(owned, scope, action, "api", "scheduler_jobs") {
			return appErr(403, "SCHEDULER_PLUGIN_OWNER_MISMATCH", "API Key未授予目标插件owner权限", nil)
		}
		return nil
	}
	if claims.UserID == 0 && !claims.IsRoot {
		return appErr(401, "SCHEDULER_ADMIN_USER_REQUIRED", "需要后台用户身份", nil)
	}
	if ownerType == models.OwnerTypeCore && !claims.IsRoot {
		return appErr(403, "SCHEDULER_CORE_OWNER_FORBIDDEN", "只有root可操作core任务", nil)
	}
	allowed, err := iamsvc.NewRBACService(s.db).Enforce(ctx, iamsvc.ActorContext{TenantUUID: tenant, IsRoot: reqctx.IsRoot(ctx)}, tenant, reqctx.GetMemberID(ctx), "corex", "scheduler.jobs", action)
	if err != nil || !allowed {
		return appErr(403, "SCHEDULER_PERMISSION_DENIED", "缺少调度任务操作权限", err)
	}
	return nil
}
func (s *Service) DeleteJob(ctx context.Context, input DeleteJobInput) (result *DeleteJobResult, err error) {
	if input.ExpectedRevision == 0 {
		return nil, appErr(400, "SCHEDULER_EXPECTED_REVISION_REQUIRED", "必须提交预览版本expected_revision", nil)
	}
	err = s.withJobFence(ctx, input.JobUUID, func(locked *Service) error {
		row, lookupErr := locked.findHistoricalJob(ctx, input.JobUUID)
		if lookupErr != nil {
			return lookupErr
		}
		if err := locked.authorizeCleanup(ctx, row.OwnerType, row.OwnerID, "delete"); err != nil {
			return err
		}
		deleted := row.DeletedAt.Valid || row.Status == models.JobStatusDeleted
		if !deleted && row.Revision != input.ExpectedRevision {
			return appErr(409, "SCHEDULER_JOB_VERSION_CONFLICT", "任务已变化，请重新预览", nil)
		}
		if !deleted {
			actor := actorFromContext(ctx, reqctx.GetSubject(ctx))
			row.ActorType = actor.Type
			row.ActorUserID = actor.UserID
			row.ActorUserUUID = actor.UserUUID
			row.ActorMemberID = actor.MemberID
			row.ActorMemberUUID = actor.MemberUUID
			if err := locked.jobs.SoftDelete(ctx, row, reqctx.GetSubject(ctx), reqctx.GetTraceID(ctx), locked.clock().UTC()); err != nil {
				return mapDBErr(err)
			}
		}
		result = &DeleteJobResult{Schema: "powerx.scheduler.job-deletion/v1", JobUUID: row.UUID.String(), Status: models.JobStatusDeleted, Deleted: true, AlreadyDeleted: deleted, Revision: row.Revision, DeletedAt: row.DeletedAt.Time, HistoryRetained: true, TraceID: reqctx.GetTraceID(ctx)}
		return nil
	})
	return
}

type CleanupJob struct {
	JobUUID      string          `json:"job_uuid"`
	TenantUUID   string          `json:"tenant_uuid"`
	OwnerType    string          `json:"owner_type"`
	OwnerID      string          `json:"owner_id"`
	Name         string          `json:"name"`
	ScheduleType string          `json:"schedule_type"`
	ScheduleExpr string          `json:"schedule_expr"`
	Timezone     string          `json:"timezone"`
	Payload      json.RawMessage `json:"payload"`
	Status       string          `json:"status"`
	Revision     uint64          `json:"revision"`
	NextRunAt    *time.Time      `json:"next_run_at,omitempty"`
	LastRunAt    *time.Time      `json:"last_run_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	DeletedAt    *time.Time      `json:"deleted_at,omitempty"`
}

func cleanupJob(row *models.SchedulerJob) *CleanupJob {
	out := &CleanupJob{JobUUID: row.UUID.String(), TenantUUID: row.TenantUUID, OwnerType: row.OwnerType, OwnerID: row.OwnerID, Name: row.Name, ScheduleType: row.ScheduleType, ScheduleExpr: row.ScheduleExpr, Timezone: row.Timezone, Payload: append(json.RawMessage(nil), row.PayloadJSON...), Status: row.Status, Revision: row.Revision, NextRunAt: row.NextRunAt, LastRunAt: row.LastRunAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.DeletedAt.Valid {
		out.DeletedAt = &row.DeletedAt.Time
	}
	return out
}
func (s *Service) GetCleanupJob(ctx context.Context, id string, includeDeleted bool) (*CleanupJob, error) {
	row, err := s.findHistoricalJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeCleanup(ctx, row.OwnerType, row.OwnerID, "read"); err != nil {
		return nil, err
	}
	if !includeDeleted && (row.DeletedAt.Valid || row.Status == models.JobStatusDeleted) {
		return nil, appErr(404, "SCHEDULER_JOB_NOT_FOUND", "任务已删除", nil)
	}
	return cleanupJob(row), nil
}
func (s *Service) ListCleanupJobs(ctx context.Context, in ListJobsInput) ([]*CleanupJob, int64, error) {
	if (in.Status != "" && in.Status != models.JobStatusActive && in.Status != models.JobStatusPaused && in.Status != models.JobStatusCompleted) || in.Page < 0 || in.PageSize < 0 || in.PageSize > 500 {
		return nil, 0, InvalidCleanupArgumentError()
	}
	if in.OwnerType == "" {
		in.OwnerType = models.OwnerTypePlugin
	}
	if err := s.authorizeCleanup(ctx, in.OwnerType, in.OwnerID, "read"); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.ListJobs(ctx, in)
	if err != nil {
		return nil, 0, err
	}
	items := make([]*CleanupJob, 0, len(rows))
	for _, row := range rows {
		items = append(items, cleanupJob(row))
	}
	return items, total, nil
}
func (s *Service) ListCleanupRuns(ctx context.Context, in ListRunsInput) ([]*models.SchedulerJobRun, int64, error) {
	row, err := s.findHistoricalJob(ctx, in.JobID)
	if err != nil {
		return nil, 0, err
	}
	if err := s.authorizeCleanup(ctx, row.OwnerType, row.OwnerID, "read"); err != nil {
		return nil, 0, err
	}
	page, size := normalizePage(in.Page, in.PageSize)
	return s.runs.List(ctx, repo.RunFilter{JobUUID: row.UUID, Page: page, PageSize: size})
}

func InvalidCleanupArgumentError() error {
	return appErr(400, "SCHEDULER_INVALID_ARGUMENT", "删除需要有效UUID及expected_revision，且只接受正式字段", nil)
}
func AdminUserRequiredError() error {
	return appErr(403, "SCHEDULER_ADMIN_USER_REQUIRED", "服务凭据请使用tenant或typed服务入口", nil)
}
func ServiceActorRequiredError() error {
	return appErr(403, "SCHEDULER_SERVICE_ACTOR_REQUIRED", "需要API Key或STS服务凭据", nil)
}

func (s *Service) AdminGetJob(ctx context.Context, id string) (*models.SchedulerJob, error) {
	if cap.ServiceCredential(ctx) {
		return nil, AdminUserRequiredError()
	}
	row, err := s.findHistoricalJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeCleanup(ctx, row.OwnerType, row.OwnerID, "read"); err != nil {
		return nil, err
	}
	if row.DeletedAt.Valid || row.Status == models.JobStatusDeleted {
		return nil, appErr(404, "SCHEDULER_JOB_NOT_FOUND", "任务已删除", nil)
	}
	return row, nil
}
func (s *Service) AdminListJobs(ctx context.Context, in ListJobsInput) ([]*models.SchedulerJob, int64, error) {
	if cap.ServiceCredential(ctx) {
		return nil, 0, AdminUserRequiredError()
	}
	if err := s.authorizeCleanup(ctx, in.OwnerType, in.OwnerID, "read"); err != nil {
		return nil, 0, err
	}
	if !reqctx.IsRoot(ctx) {
		in.OwnerType = models.OwnerTypePlugin
	}
	return s.ListJobs(ctx, in)
}
func (s *Service) AdminListRuns(ctx context.Context, in ListRunsInput) ([]*models.SchedulerJobRun, int64, error) {
	if cap.ServiceCredential(ctx) {
		return nil, 0, AdminUserRequiredError()
	}
	return s.ListCleanupRuns(ctx, in)
}
