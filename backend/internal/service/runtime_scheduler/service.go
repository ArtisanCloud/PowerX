package runtimescheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/runtime_scheduler"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const CapabilityID = "com.corex.scheduler.jobs"

type Service struct {
	db      *gorm.DB
	jobs    *repo.JobRepository
	runs    *repo.RunRepository
	plugins *reposetting.PluginInstanceConfigRepository
	eventBu event_bus.EventBus
	clock   func() time.Time
}

type Options struct {
	DB       *gorm.DB
	EventBus event_bus.EventBus
	Clock    func() time.Time
}

func NewService(opts Options) *Service {
	if opts.DB == nil {
		return nil
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Service{
		db:      opts.DB,
		jobs:    repo.NewJobRepository(opts.DB),
		runs:    repo.NewRunRepository(opts.DB),
		plugins: reposetting.NewPluginInstanceConfigRepository(opts.DB),
		eventBu: opts.EventBus,
		clock:   clock,
	}
}

type JobSpec struct {
	TenantUUID     string         `json:"tenant_uuid"`
	OwnerType      string         `json:"owner_type"`
	OwnerID        string         `json:"owner_id"`
	Name           string         `json:"name"`
	ScheduleType   string         `json:"schedule_type"`
	ScheduleExpr   string         `json:"schedule_expr"`
	Timezone       string         `json:"timezone"`
	Payload        map[string]any `json:"payload"`
	MisfirePolicy  string         `json:"misfire_policy"`
	OverlapPolicy  string         `json:"overlap_policy"`
	IdempotencyKey string         `json:"idempotency_key"`
}

type UpdateJobInput struct {
	JobID          string
	Name           *string
	ScheduleType   *string
	ScheduleExpr   *string
	Timezone       *string
	Payload        map[string]any
	MisfirePolicy  *string
	OverlapPolicy  *string
	IdempotencyKey *string
	Operator       string
	TraceID        string
}

type ListJobsInput struct {
	TenantUUID string
	OwnerType  string
	OwnerID    string
	Status     string
	Page       int
	PageSize   int
}

type ListRunsInput struct {
	JobID    string
	Page     int
	PageSize int
}

type TriggerResult struct {
	Job *models.SchedulerJob    `json:"job"`
	Run *models.SchedulerJobRun `json:"run"`
}

type actorContext struct {
	Type       string `json:"type,omitempty"`
	UserID     uint64 `json:"user_id,omitempty"`
	UserUUID   string `json:"user_uuid,omitempty"`
	MemberID   uint64 `json:"member_id,omitempty"`
	MemberUUID string `json:"member_uuid,omitempty"`
	Subject    string `json:"subject,omitempty"`
}

type DispatchDueInput struct {
	Now   time.Time
	Limit int
}

type DispatchDueResult struct {
	DueCount        int
	DispatchedCount int
}

func (s *Service) CreateJob(ctx context.Context, spec JobSpec, operator, traceID string) (*models.SchedulerJob, error) {
	if s == nil {
		return nil, appErr(http.StatusServiceUnavailable, "SCHEDULER_UNAVAILABLE", "调度服务不可用", nil)
	}
	normalized, err := s.normalizeSpec(ctx, spec, true)
	if err != nil {
		return nil, err
	}
	if err := s.ensureOwnerAcceptsNewUsage(ctx, normalized.OwnerType, normalized.OwnerID); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(normalized.Payload)
	if err != nil {
		return nil, appErr(http.StatusBadRequest, "SCHEDULER_INVALID_PAYLOAD", "调度 payload 无法序列化", err)
	}
	next, err := computeNextRun(normalized.ScheduleType, normalized.ScheduleExpr, normalized.Timezone, s.clock())
	if err != nil {
		return nil, err
	}
	actor := actorFromContext(ctx, strings.TrimSpace(operator))
	row := &models.SchedulerJob{
		TenantUUID:      normalized.TenantUUID,
		OwnerType:       normalized.OwnerType,
		OwnerID:         normalized.OwnerID,
		Name:            normalized.Name,
		ScheduleType:    normalized.ScheduleType,
		ScheduleExpr:    normalized.ScheduleExpr,
		Timezone:        normalized.Timezone,
		Topic:           models.TopicSchedulerTriggeredV1,
		PayloadJSON:     datatypes.JSON(payload),
		Status:          models.JobStatusActive,
		NextRunAt:       next,
		MisfirePolicy:   normalized.MisfirePolicy,
		OverlapPolicy:   normalized.OverlapPolicy,
		IdempotencyKey:  normalized.IdempotencyKey,
		ActorType:       actor.Type,
		ActorUserID:     actor.UserID,
		ActorUserUUID:   actor.UserUUID,
		ActorMemberID:   actor.MemberID,
		ActorMemberUUID: actor.MemberUUID,
		CreatedBy:       strings.TrimSpace(operator),
		UpdatedBy:       strings.TrimSpace(operator),
		TraceID:         strings.TrimSpace(traceID),
	}
	created, err := s.jobs.Create(ctx, row)
	if err != nil {
		return nil, mapDBErr(err)
	}
	return created, nil
}

func (s *Service) ensureOwnerAcceptsNewUsage(ctx context.Context, ownerType, ownerID string) error {
	if !strings.EqualFold(strings.TrimSpace(ownerType), models.OwnerTypePlugin) {
		return nil
	}
	if s == nil || s.plugins == nil {
		return appErr(http.StatusServiceUnavailable, "SCHEDULER_PLUGIN_USAGE_GUARD_UNAVAILABLE", "插件使用状态检查不可用", nil)
	}
	count, err := s.plugins.CountTenantPluginBindings(ctx, reposetting.ListTenantPluginOptions{
		PluginIDs: []string{strings.TrimSpace(ownerID)},
		Key:       reposetting.KeyClientCredentials,
		Statuses: []string{
			dbsetting.PluginInstanceStatusDrainingRequested,
			dbsetting.PluginInstanceStatusDisabledByPlatform,
		},
	})
	if err != nil {
		return err
	}
	if count > 0 {
		return appErr(http.StatusConflict, "SCHEDULER_PLUGIN_DRAINING", "插件正在 drain 或已被平台禁用，禁止新增调度任务", nil)
	}
	return nil
}

func (s *Service) UpdateJob(ctx context.Context, input UpdateJobInput) (*models.SchedulerJob, error) {
	job, err := s.getMutableJob(ctx, input.JobID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeJob(ctx, job); err != nil {
		return nil, err
	}
	if input.Name != nil {
		job.Name = strings.TrimSpace(*input.Name)
	}
	if input.ScheduleType != nil {
		job.ScheduleType = strings.TrimSpace(*input.ScheduleType)
	}
	if input.ScheduleExpr != nil {
		job.ScheduleExpr = strings.TrimSpace(*input.ScheduleExpr)
	}
	if input.Timezone != nil {
		job.Timezone = strings.TrimSpace(*input.Timezone)
	}
	if input.Payload != nil {
		payload, err := json.Marshal(input.Payload)
		if err != nil {
			return nil, appErr(http.StatusBadRequest, "SCHEDULER_INVALID_PAYLOAD", "调度 payload 无法序列化", err)
		}
		job.PayloadJSON = datatypes.JSON(payload)
	}
	if input.MisfirePolicy != nil {
		job.MisfirePolicy = strings.TrimSpace(*input.MisfirePolicy)
	}
	if input.OverlapPolicy != nil {
		job.OverlapPolicy = strings.TrimSpace(*input.OverlapPolicy)
	}
	if input.IdempotencyKey != nil {
		job.IdempotencyKey = strings.TrimSpace(*input.IdempotencyKey)
	}
	spec := JobSpec{
		TenantUUID:     job.TenantUUID,
		OwnerType:      job.OwnerType,
		OwnerID:        job.OwnerID,
		Name:           job.Name,
		ScheduleType:   job.ScheduleType,
		ScheduleExpr:   job.ScheduleExpr,
		Timezone:       job.Timezone,
		MisfirePolicy:  job.MisfirePolicy,
		OverlapPolicy:  job.OverlapPolicy,
		IdempotencyKey: job.IdempotencyKey,
	}
	if _, err := s.normalizeSpec(ctx, spec, false); err != nil {
		return nil, err
	}
	next, err := computeNextRun(job.ScheduleType, job.ScheduleExpr, job.Timezone, s.clock())
	if err != nil {
		return nil, err
	}
	job.NextRunAt = next
	job.Topic = models.TopicSchedulerTriggeredV1
	actor := actorFromContext(ctx, strings.TrimSpace(input.Operator))
	job.ActorType = actor.Type
	job.ActorUserID = actor.UserID
	job.ActorUserUUID = actor.UserUUID
	job.ActorMemberID = actor.MemberID
	job.ActorMemberUUID = actor.MemberUUID
	job.UpdatedBy = strings.TrimSpace(input.Operator)
	job.TraceID = strings.TrimSpace(input.TraceID)
	updated, err := s.jobs.Update(ctx, job)
	if err != nil {
		return nil, mapDBErr(err)
	}
	return updated, nil
}

func (s *Service) GetJob(ctx context.Context, jobID string) (*models.SchedulerJob, error) {
	job, err := s.findJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeJob(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Service) ListJobs(ctx context.Context, input ListJobsInput) ([]*models.SchedulerJob, int64, error) {
	tenantUUID, err := resolveTenant(ctx, input.TenantUUID)
	if err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(input.Page, input.PageSize)
	return s.jobs.List(ctx, repo.JobFilter{
		TenantUUID: tenantUUID,
		OwnerType:  strings.TrimSpace(input.OwnerType),
		OwnerID:    strings.TrimSpace(input.OwnerID),
		Status:     strings.TrimSpace(input.Status),
		Page:       page,
		PageSize:   pageSize,
	})
}

func (s *Service) PauseJob(ctx context.Context, jobID, operator, traceID string) (*models.SchedulerJob, error) {
	return s.setStatus(ctx, jobID, models.JobStatusPaused, operator, traceID)
}

func (s *Service) ResumeJob(ctx context.Context, jobID, operator, traceID string) (*models.SchedulerJob, error) {
	job, err := s.getMutableJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeJob(ctx, job); err != nil {
		return nil, err
	}
	next, err := computeNextRun(job.ScheduleType, job.ScheduleExpr, job.Timezone, s.clock())
	if err != nil {
		return nil, err
	}
	job.Status = models.JobStatusActive
	job.NextRunAt = next
	actor := actorFromContext(ctx, strings.TrimSpace(operator))
	job.ActorType = actor.Type
	job.ActorUserID = actor.UserID
	job.ActorUserUUID = actor.UserUUID
	job.ActorMemberID = actor.MemberID
	job.ActorMemberUUID = actor.MemberUUID
	job.UpdatedBy = strings.TrimSpace(operator)
	job.TraceID = strings.TrimSpace(traceID)
	return s.jobs.Update(ctx, job)
}

func (s *Service) TriggerJob(ctx context.Context, jobID, operator, traceID string) (*TriggerResult, error) {
	job, err := s.getMutableJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeJob(ctx, job); err != nil {
		return nil, err
	}
	if job.Status != models.JobStatusActive {
		return nil, appErr(http.StatusConflict, "SCHEDULER_JOB_NOT_ACTIVE", "调度任务不是 active 状态，不能触发", nil)
	}
	now := s.clock().UTC()
	if strings.TrimSpace(traceID) == "" {
		traceID = reqctx.GetTraceID(ctx)
	}
	actor := actorFromContext(ctx, strings.TrimSpace(operator))
	eventID := fmt.Sprintf("scheduler.%s.%d", job.UUID.String(), now.UnixMilli())
	run := &models.SchedulerJobRun{
		JobUUID:         job.UUID,
		TenantUUID:      job.TenantUUID,
		OwnerType:       job.OwnerType,
		OwnerID:         job.OwnerID,
		TriggerSource:   models.TriggerSourceManual,
		ScheduledAt:     job.NextRunAt,
		FiredAt:         &now,
		Status:          models.RunStatusTriggered,
		EventID:         eventID,
		TraceID:         strings.TrimSpace(traceID),
		ActorType:       actor.Type,
		ActorUserID:     actor.UserID,
		ActorUserUUID:   actor.UserUUID,
		ActorMemberID:   actor.MemberID,
		ActorMemberUUID: actor.MemberUUID,
	}
	createdRun, err := s.runs.Create(ctx, run)
	if err != nil {
		return nil, mapDBErr(err)
	}
	payload := s.buildTriggerPayload(job, createdRun, models.TriggerSourceManual, now)
	if s.eventBu != nil {
		s.eventBu.Publish(models.TopicSchedulerTriggeredV1, payload, ctx)
	}
	next, nextErr := computeNextRun(job.ScheduleType, job.ScheduleExpr, job.Timezone, now)
	if nextErr != nil {
		job.LastError = nextErr.Error()
	} else {
		job.LastError = ""
		job.NextRunAt = next
	}
	job.LastRunAt = &now
	job.UpdatedBy = strings.TrimSpace(operator)
	job.ActorType = actor.Type
	job.ActorUserID = actor.UserID
	job.ActorUserUUID = actor.UserUUID
	job.ActorMemberID = actor.MemberID
	job.ActorMemberUUID = actor.MemberUUID
	job.TraceID = strings.TrimSpace(traceID)
	if updated, updateErr := s.jobs.Update(ctx, job); updateErr == nil {
		job = updated
	}
	return &TriggerResult{Job: job, Run: createdRun}, nil
}

func (s *Service) DispatchDue(ctx context.Context, input DispatchDueInput) (*DispatchDueResult, error) {
	if s == nil {
		return nil, appErr(http.StatusServiceUnavailable, "SCHEDULER_UNAVAILABLE", "调度服务不可用", nil)
	}
	now := input.Now
	if now.IsZero() {
		now = s.clock()
	}
	now = now.UTC()
	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	dueJobs, err := s.jobs.ListDue(ctx, now, limit)
	if err != nil {
		return nil, mapDBErr(err)
	}
	result := &DispatchDueResult{DueCount: len(dueJobs)}
	for _, due := range dueJobs {
		if due == nil {
			continue
		}
		ok, err := s.dispatchDueJob(ctx, due.UUID, now)
		if err != nil {
			return result, err
		}
		if ok {
			result.DispatchedCount++
		}
	}
	return result, nil
}

func (s *Service) ListRuns(ctx context.Context, input ListRunsInput) ([]*models.SchedulerJobRun, int64, error) {
	job, err := s.GetJob(ctx, input.JobID)
	if err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(input.Page, input.PageSize)
	return s.runs.List(ctx, repo.RunFilter{JobUUID: job.UUID, Page: page, PageSize: pageSize})
}

func (s *Service) dispatchDueJob(ctx context.Context, jobID uuid.UUID, now time.Time) (bool, error) {
	var job models.SchedulerJob
	var createdRun *models.SchedulerJobRun
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("uuid = ?", jobID).
			Take(&job).Error; err != nil {
			return err
		}
		if job.Status != models.JobStatusActive || job.NextRunAt == nil || job.NextRunAt.After(now.UTC()) {
			return gorm.ErrRecordNotFound
		}
		traceID := strings.TrimSpace(job.TraceID)
		if traceID == "" {
			traceID = fmt.Sprintf("scheduler.%s.%d", job.UUID.String(), now.UnixMilli())
		}
		source := triggerSourceForSchedule(job.ScheduleType)
		eventID := fmt.Sprintf("scheduler.%s.%d", job.UUID.String(), now.UnixMilli())
		actor := systemSchedulerActor()
		run := &models.SchedulerJobRun{
			JobUUID:         job.UUID,
			TenantUUID:      job.TenantUUID,
			OwnerType:       job.OwnerType,
			OwnerID:         job.OwnerID,
			TriggerSource:   source,
			ScheduledAt:     job.NextRunAt,
			FiredAt:         &now,
			Status:          models.RunStatusTriggered,
			EventID:         eventID,
			TraceID:         traceID,
			ActorType:       actor.Type,
			ActorUserID:     actor.UserID,
			ActorUserUUID:   actor.UserUUID,
			ActorMemberID:   actor.MemberID,
			ActorMemberUUID: actor.MemberUUID,
		}
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		createdRun = run
		next, nextErr := nextAfterDispatch(&job, now)
		updates := map[string]interface{}{
			"last_run_at": now,
			"updated_by":  "runtime_scheduler",
			"trace_id":    traceID,
		}
		if nextErr != nil {
			updates["last_error"] = nextErr.Error()
		} else {
			updates["last_error"] = ""
			if next == nil {
				updates["next_run_at"] = nil
				if job.ScheduleType == models.ScheduleTypeOnce {
					updates["status"] = models.JobStatusCompleted
				}
			} else {
				updates["next_run_at"] = *next
			}
		}
		return tx.Model(&models.SchedulerJob{}).Where("uuid = ?", job.UUID).Updates(updates).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, mapDBErr(err)
	}
	if createdRun == nil {
		return false, nil
	}
	payload := s.buildTriggerPayload(&job, createdRun, createdRun.TriggerSource, now)
	if s.eventBu != nil {
		dispatchCtx := reqctx.WithTenantUUID(ctx, job.TenantUUID)
		dispatchCtx = reqctx.WithTraceID(dispatchCtx, createdRun.TraceID)
		s.eventBu.Publish(models.TopicSchedulerTriggeredV1, payload, dispatchCtx)
	}
	return true, nil
}

func (s *Service) setStatus(ctx context.Context, jobID, status, operator, traceID string) (*models.SchedulerJob, error) {
	job, err := s.getMutableJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeJob(ctx, job); err != nil {
		return nil, err
	}
	job.Status = status
	actor := actorFromContext(ctx, strings.TrimSpace(operator))
	job.ActorType = actor.Type
	job.ActorUserID = actor.UserID
	job.ActorUserUUID = actor.UserUUID
	job.ActorMemberID = actor.MemberID
	job.ActorMemberUUID = actor.MemberUUID
	job.UpdatedBy = strings.TrimSpace(operator)
	job.TraceID = strings.TrimSpace(traceID)
	updated, err := s.jobs.Update(ctx, job)
	if err != nil {
		return nil, mapDBErr(err)
	}
	return updated, nil
}

func (s *Service) findJob(ctx context.Context, jobID string) (*models.SchedulerJob, error) {
	id, err := uuid.Parse(strings.TrimSpace(jobID))
	if err != nil {
		return nil, appErr(http.StatusBadRequest, "SCHEDULER_INVALID_JOB_ID", "无效的调度任务 ID", err)
	}
	job, err := s.jobs.FindByUUID(ctx, id)
	if err != nil {
		return nil, mapDBErr(err)
	}
	if job == nil {
		return nil, appErr(http.StatusNotFound, "SCHEDULER_JOB_NOT_FOUND", "调度任务不存在", nil)
	}
	return job, nil
}

func (s *Service) getMutableJob(ctx context.Context, jobID string) (*models.SchedulerJob, error) {
	job, err := s.findJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.Status == models.JobStatusDeleted {
		return nil, appErr(http.StatusGone, "SCHEDULER_JOB_DELETED", "调度任务已删除", nil)
	}
	if job.Status == models.JobStatusCompleted {
		return nil, appErr(http.StatusConflict, "SCHEDULER_JOB_COMPLETED", "调度任务已完成，不能修改", nil)
	}
	return job, nil
}

func (s *Service) authorizeJob(ctx context.Context, job *models.SchedulerJob) error {
	if job == nil {
		return appErr(http.StatusNotFound, "SCHEDULER_JOB_NOT_FOUND", "调度任务不存在", nil)
	}
	tenantUUID, err := resolveTenant(ctx, job.TenantUUID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(tenantUUID, job.TenantUUID) {
		return appErr(http.StatusForbidden, "SCHEDULER_TENANT_MISMATCH", "调度任务租户不匹配", nil)
	}
	return authorizeOwner(ctx, job.OwnerType, job.OwnerID)
}

func (s *Service) normalizeSpec(ctx context.Context, spec JobSpec, requirePayload bool) (JobSpec, error) {
	tenantUUID, err := resolveTenant(ctx, spec.TenantUUID)
	if err != nil {
		return JobSpec{}, err
	}
	spec.TenantUUID = tenantUUID
	spec.OwnerType = strings.TrimSpace(spec.OwnerType)
	spec.OwnerID = strings.TrimSpace(spec.OwnerID)
	spec.Name = strings.TrimSpace(spec.Name)
	spec.ScheduleType = strings.TrimSpace(spec.ScheduleType)
	spec.ScheduleExpr = strings.TrimSpace(spec.ScheduleExpr)
	spec.Timezone = strings.TrimSpace(spec.Timezone)
	if spec.Timezone == "" {
		spec.Timezone = "UTC"
	}
	spec.MisfirePolicy = normalizeMisfire(spec.MisfirePolicy)
	spec.OverlapPolicy = normalizeOverlap(spec.OverlapPolicy)
	spec.IdempotencyKey = strings.TrimSpace(spec.IdempotencyKey)
	if spec.Name == "" {
		return JobSpec{}, appErr(http.StatusBadRequest, "SCHEDULER_NAME_REQUIRED", "调度任务名称不能为空", nil)
	}
	if spec.OwnerType == "" || spec.OwnerID == "" {
		return JobSpec{}, appErr(http.StatusBadRequest, "SCHEDULER_OWNER_REQUIRED", "调度任务 owner_type/owner_id 不能为空", nil)
	}
	if err := authorizeOwner(ctx, spec.OwnerType, spec.OwnerID); err != nil {
		return JobSpec{}, err
	}
	if requirePayload && spec.Payload == nil {
		return JobSpec{}, appErr(http.StatusBadRequest, "SCHEDULER_PAYLOAD_REQUIRED", "调度 payload 不能为空", nil)
	}
	if spec.Payload == nil {
		spec.Payload = map[string]any{}
	}
	if _, err := computeNextRun(spec.ScheduleType, spec.ScheduleExpr, spec.Timezone, s.clock()); err != nil {
		return JobSpec{}, err
	}
	return spec, nil
}

func (s *Service) buildTriggerPayload(job *models.SchedulerJob, run *models.SchedulerJobRun, triggerSource string, firedAt time.Time) map[string]any {
	payload := map[string]any{}
	_ = json.Unmarshal(job.PayloadJSON, &payload)
	runActor := actorFromRun(run)
	jobActor := actorFromJob(job)
	return map[string]any{
		"job_id":          job.UUID.String(),
		"job_name":        job.Name,
		"owner_type":      job.OwnerType,
		"owner_id":        job.OwnerID,
		"tenant_uuid":     job.TenantUUID,
		"trigger_source":  triggerSource,
		"scheduled_at":    formatTimePtr(run.ScheduledAt),
		"fired_at":        firedAt.Format(time.RFC3339Nano),
		"trace_id":        run.TraceID,
		"event_id":        run.EventID,
		"idempotency_key": job.IdempotencyKey,
		"business_action": payload["business_action"],
		"actor":           actorToMap(runActor),
		"job_actor":       actorToMap(jobActor),
		"payload":         payload,
	}
}

func actorFromContext(ctx context.Context, subject string) actorContext {
	actorType := "user"
	if strings.EqualFold(strings.TrimSpace(reqctx.GetScope(ctx)), "api_key") {
		actorType = "api_key"
	}
	if strings.TrimSpace(subject) == "" {
		subject = reqctx.GetSubject(ctx)
	}
	if strings.TrimSpace(subject) == "" {
		subject = reqctx.GetUserUUID(ctx)
	}
	return actorContext{
		Type:       actorType,
		UserID:     reqctx.GetUserID(ctx),
		UserUUID:   strings.TrimSpace(reqctx.GetUserUUID(ctx)),
		MemberID:   reqctx.GetMemberID(ctx),
		MemberUUID: strings.TrimSpace(reqctx.GetMemberUUID(ctx)),
		Subject:    strings.TrimSpace(subject),
	}
}

func systemSchedulerActor() actorContext {
	return actorContext{Type: "system", Subject: "runtime_scheduler"}
}

func actorFromJob(job *models.SchedulerJob) actorContext {
	if job == nil {
		return actorContext{}
	}
	subject := strings.TrimSpace(job.ActorMemberUUID)
	if subject == "" {
		subject = strings.TrimSpace(job.ActorUserUUID)
	}
	if subject == "" {
		subject = strings.TrimSpace(job.UpdatedBy)
	}
	return actorContext{
		Type:       strings.TrimSpace(job.ActorType),
		UserID:     job.ActorUserID,
		UserUUID:   strings.TrimSpace(job.ActorUserUUID),
		MemberID:   job.ActorMemberID,
		MemberUUID: strings.TrimSpace(job.ActorMemberUUID),
		Subject:    subject,
	}
}

func actorFromRun(run *models.SchedulerJobRun) actorContext {
	if run == nil {
		return actorContext{}
	}
	subject := strings.TrimSpace(run.ActorMemberUUID)
	if subject == "" {
		subject = strings.TrimSpace(run.ActorUserUUID)
	}
	if subject == "" && strings.TrimSpace(run.ActorType) == "system" {
		subject = "runtime_scheduler"
	}
	return actorContext{
		Type:       strings.TrimSpace(run.ActorType),
		UserID:     run.ActorUserID,
		UserUUID:   strings.TrimSpace(run.ActorUserUUID),
		MemberID:   run.ActorMemberID,
		MemberUUID: strings.TrimSpace(run.ActorMemberUUID),
		Subject:    subject,
	}
}

func actorToMap(actor actorContext) map[string]any {
	out := map[string]any{}
	if actor.Type != "" {
		out["type"] = actor.Type
	}
	if actor.Subject != "" {
		out["subject"] = actor.Subject
	}
	if actor.UserID > 0 {
		out["user_id"] = actor.UserID
	}
	if actor.UserUUID != "" {
		out["user_uuid"] = actor.UserUUID
	}
	if actor.MemberID > 0 {
		out["member_id"] = actor.MemberID
	}
	if actor.MemberUUID != "" {
		out["member_uuid"] = actor.MemberUUID
	}
	return out
}

func computeNextRun(scheduleType, expr, timezone string, now time.Time) (*time.Time, error) {
	scheduleType = strings.TrimSpace(scheduleType)
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, appErr(http.StatusBadRequest, "SCHEDULER_EXPR_REQUIRED", "调度表达式不能为空", nil)
	}
	loc, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil {
		return nil, appErr(http.StatusBadRequest, "SCHEDULER_INVALID_TIMEZONE", "无效的调度时区", err)
	}
	switch scheduleType {
	case models.ScheduleTypeOnce:
		ts, err := time.Parse(time.RFC3339, expr)
		if err != nil {
			return nil, appErr(http.StatusBadRequest, "SCHEDULER_INVALID_ONCE_EXPR", "once 调度表达式必须是 RFC3339 时间", err)
		}
		next := ts.UTC()
		return &next, nil
	case models.ScheduleTypeInterval:
		d, err := time.ParseDuration(expr)
		if err != nil || d <= 0 {
			if err == nil {
				err = errors.New("duration must be positive")
			}
			return nil, appErr(http.StatusBadRequest, "SCHEDULER_INVALID_INTERVAL_EXPR", "interval 调度表达式必须是正数 Go duration", err)
		}
		next := now.UTC().Add(d)
		return &next, nil
	case models.ScheduleTypeCron:
		next, err := nextCron(expr, now.In(loc))
		if err != nil {
			return nil, appErr(http.StatusBadRequest, "SCHEDULER_INVALID_CRON_EXPR", "cron 调度表达式无效", err)
		}
		nextUTC := next.UTC()
		return &nextUTC, nil
	default:
		return nil, appErr(http.StatusBadRequest, "SCHEDULER_INVALID_TYPE", "无效的调度类型", nil)
	}
}

func nextAfterDispatch(job *models.SchedulerJob, now time.Time) (*time.Time, error) {
	if job == nil {
		return nil, nil
	}
	if job.ScheduleType == models.ScheduleTypeOnce {
		return nil, nil
	}
	return computeNextRun(job.ScheduleType, job.ScheduleExpr, job.Timezone, now)
}

func triggerSourceForSchedule(scheduleType string) string {
	switch strings.TrimSpace(scheduleType) {
	case models.ScheduleTypeOnce:
		return models.TriggerSourceOnce
	case models.ScheduleTypeInterval:
		return models.TriggerSourceInterval
	case models.ScheduleTypeCron:
		return models.TriggerSourceCron
	default:
		return models.TriggerSourceManual
	}
}

func authorizeOwner(ctx context.Context, ownerType, ownerID string) error {
	ownerType = strings.TrimSpace(ownerType)
	ownerID = strings.TrimSpace(ownerID)
	switch ownerType {
	case models.OwnerTypeCore:
		if !reqctx.IsRoot(ctx) {
			return appErr(http.StatusForbidden, "SCHEDULER_CORE_OWNER_FORBIDDEN", "只有 root 可以管理 core 调度任务", nil)
		}
	case models.OwnerTypePlugin:
		claims := reqctx.GetClaims(ctx)
		if claims == nil {
			return appErr(http.StatusUnauthorized, "SCHEDULER_CLAIMS_REQUIRED", "缺少鉴权上下文", nil)
		}
		if claims.IsRoot {
			return nil
		}
		if isAPIKeySchedulerNamespaceCaller(ctx, claims) {
			return nil
		}
		expected := pluginIDFromClaims(claims)
		if expected == "" || !strings.EqualFold(expected, ownerID) {
			return appErr(http.StatusForbidden, "SCHEDULER_PLUGIN_OWNER_MISMATCH", "插件只能管理自己的调度任务", nil)
		}
	default:
		return appErr(http.StatusBadRequest, "SCHEDULER_INVALID_OWNER_TYPE", "无效的 owner_type", nil)
	}
	return nil
}

func isAPIKeySchedulerNamespaceCaller(ctx context.Context, claims *reqctx.CoreXClaims) bool {
	if strings.EqualFold(strings.TrimSpace(reqctx.GetScope(ctx)), "api_key") {
		return true
	}
	if claims == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(claims.Scope), "api_key") {
		return true
	}
	for _, platform := range claims.Platforms {
		if strings.EqualFold(strings.TrimSpace(platform), "api_key") {
			return true
		}
	}
	return false
}

func pluginIDFromClaims(claims *reqctx.CoreXClaims) string {
	if claims == nil {
		return ""
	}
	for _, aud := range claims.Audience {
		aud = strings.TrimSpace(aud)
		if strings.HasPrefix(aud, "plugin:") {
			return strings.TrimPrefix(aud, "plugin:")
		}
	}
	return ""
}

func resolveTenant(ctx context.Context, requested string) (string, error) {
	ctxTenant := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if ctxTenant == "" {
		return "", appErr(http.StatusBadRequest, "SCHEDULER_TENANT_REQUIRED", "缺少租户上下文", nil)
	}
	requested = strings.TrimSpace(requested)
	if requested != "" && !strings.EqualFold(requested, ctxTenant) {
		return "", appErr(http.StatusForbidden, "SCHEDULER_TENANT_MISMATCH", "请求租户与鉴权上下文不一致", nil)
	}
	return ctxTenant, nil
}

func normalizeMisfire(v string) string {
	switch strings.TrimSpace(v) {
	case models.MisfirePolicyRunCatchup:
		return models.MisfirePolicyRunCatchup
	default:
		return models.MisfirePolicySkip
	}
}

func normalizeOverlap(v string) string {
	switch strings.TrimSpace(v) {
	case models.OverlapPolicyQueue:
		return models.OverlapPolicyQueue
	case models.OverlapPolicyParallel:
		return models.OverlapPolicyParallel
	default:
		return models.OverlapPolicySkip
	}
}

func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	return page, pageSize
}

func appErr(httpCode int, code, message string, err error) error {
	return dto.NewErrorWithCode(httpCode, code, message, err)
}

func mapDBErr(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique") {
		return appErr(http.StatusConflict, "SCHEDULER_JOB_CONFLICT", "同一租户和 owner 下已存在同名调度任务", err)
	}
	return appErr(http.StatusInternalServerError, "SCHEDULER_DB_FAILED", "调度数据操作失败", err)
}

func formatTimePtr(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.UTC().Format(time.RFC3339Nano)
}
