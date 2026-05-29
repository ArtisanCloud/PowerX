package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	wsbus "github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ErrCodePluginDrainRequired       = "PLUGIN_DRAIN_REQUIRED"
	ErrCodePluginDraining            = "PLUGIN_DRAINING"
	ErrCodePluginDrainNotFound       = "PLUGIN_DRAIN_JOB_NOT_FOUND"
	ErrCodePluginDrainInvalidRequest = "PLUGIN_DRAIN_INVALID_REQUEST"
)

type PluginDrainJobService struct {
	db           *gorm.DB
	jobs         *reposetting.PluginDrainJobRepository
	tenantConfig *reposetting.PluginInstanceConfigRepository
}

type CreateDrainJobInput struct {
	PluginID string
	Version  string
	Reason   string
	Mode     string
}

type PluginDrainImpact struct {
	PluginID       string `json:"plugin_id"`
	Version        string `json:"version,omitempty"`
	TenantCount    int64  `json:"tenant_count"`
	DrainRequired  bool   `json:"drain_required"`
	RequiredStatus string `json:"required_status,omitempty"`
}

type CancelDrainBlockersInput struct {
	PluginID          string
	Reason            string
	EventTaskIDs      []uint64
	SchedulerJobUUIDs []string
}

type ListDrainBlockersInput struct {
	PluginID string
	Kind     string
	Page     int
	PageSize int
}

type DrainBlockerPage struct {
	PluginID    string                  `json:"plugin_id"`
	Kind        string                  `json:"kind"`
	Items       any                     `json:"items"`
	Pagination  *dto.PaginationResponse `json:"pagination"`
	EventTotal  int64                   `json:"event_task_total,omitempty"`
	JobTotal    int64                   `json:"scheduler_job_total,omitempty"`
	GeneratedAt time.Time               `json:"generated_at"`
}

type DrainBlockerEventTask struct {
	ID           uint64     `json:"id"`
	TaskID       string     `json:"task_id"`
	SubscriberID string     `json:"subscriber_id"`
	Topic        string     `json:"topic"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"error_message,omitempty"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
}

type DrainBlockerSchedulerJob struct {
	UUID      string     `json:"uuid"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type CancelDrainBlockersResult struct {
	PluginID              string                      `json:"plugin_id"`
	CancelledSchedulerJob int64                       `json:"cancelled_scheduler_jobs"`
	CancelledEventTask    int64                       `json:"cancelled_event_tasks"`
	DrainJobs             []*dbsetting.PluginDrainJob `json:"drain_jobs,omitempty"`
}

func NewPluginDrainJobService(db *gorm.DB) *PluginDrainJobService {
	return &PluginDrainJobService{
		db:           db,
		jobs:         reposetting.NewPluginDrainJobRepository(db),
		tenantConfig: reposetting.NewPluginInstanceConfigRepository(db),
	}
}

func NewPluginDrainJobServiceFromRepository(repo *reposetting.PluginInstanceConfigRepository) *PluginDrainJobService {
	svc := &PluginDrainJobService{tenantConfig: repo}
	if db := repo.DB(); db != nil {
		svc.db = db
		svc.jobs = reposetting.NewPluginDrainJobRepository(db)
	}
	return svc
}

func (s *PluginDrainJobService) RequireNoActiveTenantInstances(ctx context.Context, pluginID, version string) (*PluginDrainImpact, error) {
	if s == nil || s.tenantConfig == nil {
		return nil, dto.NewErrorWithCode(http.StatusServiceUnavailable, ErrCodePluginDrainInvalidRequest, "插件 drain 服务不可用", nil)
	}
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return nil, dto.NewErrorWithCode(http.StatusBadRequest, ErrCodePluginDrainInvalidRequest, "缺少插件ID", errors.New("plugin_id required"))
	}
	count, err := s.tenantConfig.CountActiveTenantPluginBindings(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	if s.jobs != nil {
		jobs, err := s.ListDrainJobs(ctx, pluginID, 20)
		if err != nil {
			return nil, err
		}
		for _, job := range jobs {
			if job == nil {
				continue
			}
			switch job.Status {
			case dbsetting.PluginDrainJobStatusRequested,
				dbsetting.PluginDrainJobStatusBlockingNewUsage,
				dbsetting.PluginDrainJobStatusDraining:
				return &PluginDrainImpact{
						PluginID:       pluginID,
						Version:        strings.TrimSpace(version),
						TenantCount:    count,
						DrainRequired:  true,
						RequiredStatus: dbsetting.PluginInstanceStatusDrained,
					}, dto.WithDetails(
						dto.NewErrorWithCode(http.StatusConflict, ErrCodePluginDrainRequired, "插件 drain 未完成，必须先完成 drain", errors.New("plugin drain not ready")),
						map[string]interface{}{
							"plugin_id":                        pluginID,
							"version":                          strings.TrimSpace(version),
							"tenant_instance_count":            count,
							"requires_tenant_instance_cleanup": true,
							"requires_drain":                   true,
							"required_drain_job_status":        dbsetting.PluginDrainJobStatusReadyToUninstall,
							"blocking_drain_job_id":            job.JobID,
							"blocking_drain_job_status":        job.Status,
							"blocked_operation":                "uninstall",
							"scope":                            drainScope(version),
						},
					)
			}
		}
	}
	impact := &PluginDrainImpact{
		PluginID:       pluginID,
		Version:        strings.TrimSpace(version),
		TenantCount:    count,
		DrainRequired:  count > 0,
		RequiredStatus: dbsetting.PluginInstanceStatusDrained,
	}
	if count > 0 {
		return impact, dto.WithDetails(
			dto.NewErrorWithCode(http.StatusConflict, ErrCodePluginDrainRequired, "插件仍存在租户实例，必须先完成 drain", errors.New("plugin has tenant instances")),
			map[string]interface{}{
				"plugin_id":                        impact.PluginID,
				"version":                          impact.Version,
				"tenant_instance_count":            impact.TenantCount,
				"requires_tenant_instance_cleanup": true,
				"requires_drain":                   true,
				"required_tenant_instance_status":  impact.RequiredStatus,
				"blocked_operation":                "uninstall",
				"scope":                            drainScope(impact.Version),
			},
		)
	}
	return impact, nil
}

func (s *PluginDrainJobService) CompleteFinalUninstall(ctx context.Context, pluginID string) error {
	if s == nil || s.db == nil {
		return dto.NewErrorWithCode(http.StatusServiceUnavailable, ErrCodePluginDrainInvalidRequest, "插件 drain 服务不可用", nil)
	}
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return dto.NewErrorWithCode(http.StatusBadRequest, ErrCodePluginDrainInvalidRequest, "缺少插件ID", errors.New("plugin_id required"))
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cfgRepo := reposetting.NewPluginInstanceConfigRepository(tx)
		jobRepo := reposetting.NewPluginDrainJobRepository(tx)
		if _, err := cfgRepo.DeleteDrainedTenantPluginBindings(ctx, pluginID); err != nil {
			return err
		}
		if _, err := jobRepo.MarkReadyJobsCompletedByPlugin(ctx, pluginID, time.Now().UTC()); err != nil {
			return err
		}
		return nil
	})
}

func (s *PluginDrainJobService) CancelRuntimeBlockers(ctx context.Context, input CancelDrainBlockersInput) (*CancelDrainBlockersResult, error) {
	if s == nil || s.db == nil || s.jobs == nil {
		return nil, dto.NewErrorWithCode(http.StatusServiceUnavailable, ErrCodePluginDrainInvalidRequest, "插件 drain 服务不可用", nil)
	}
	pluginID := strings.TrimSpace(input.PluginID)
	if pluginID == "" {
		return nil, dto.NewErrorWithCode(http.StatusBadRequest, ErrCodePluginDrainInvalidRequest, "缺少插件ID", errors.New("plugin_id required"))
	}
	schedulerIDs := normalizeStrings(input.SchedulerJobUUIDs)
	eventTaskIDs := normalizeUint64s(input.EventTaskIDs)
	if len(schedulerIDs) == 0 && len(eventTaskIDs) == 0 {
		return nil, dto.NewErrorWithCode(http.StatusBadRequest, ErrCodePluginDrainInvalidRequest, "未选择要取消的阻断任务", errors.New("selected blocker ids required"))
	}
	now := time.Now().UTC()
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "cancelled by root drain operation"
	}
	result := &CancelDrainBlockersResult{PluginID: pluginID}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scheduler := tx.Table("scheduler_jobs").
			Where("owner_type = ? AND owner_id = ? AND status IN ?", "plugin", pluginID, []string{"active", "paused"}).
			Where("deleted_at IS NULL")
		if len(schedulerIDs) > 0 {
			scheduler = scheduler.Where("uuid IN ?", schedulerIDs)
			scheduler = scheduler.Updates(map[string]any{
				"status":     "completed",
				"last_error": reason,
				"updated_at": now,
			})
			if scheduler.Error != nil {
				return scheduler.Error
			}
			result.CancelledSchedulerJob = scheduler.RowsAffected
		}

		metadataExpr := "metadata::text"
		if tx.Dialector != nil && strings.EqualFold(tx.Dialector.Name(), "sqlite") {
			metadataExpr = "metadata"
		}
		eventTasks := tx.Table("event_task_histories").
			Where("("+metadataExpr+" LIKE ? OR subscriber_id LIKE ? OR topic LIKE ?)", "%"+pluginID+"%", "%"+pluginID+"%", "%"+pluginID+"%").
			Where("status IN ?", []string{"pending", "deferred", "running"}).
			Where("deleted_at IS NULL")
		if len(eventTaskIDs) > 0 {
			eventTasks = eventTasks.Where("id IN ?", eventTaskIDs)
			eventTasks = eventTasks.Updates(map[string]any{
				"status":        "cancelled",
				"error_message": reason,
				"completed_at":  now,
				"last_seen_at":  now,
				"updated_at":    now,
			})
			if eventTasks.Error != nil {
				return eventTasks.Error
			}
			result.CancelledEventTask = eventTasks.RowsAffected
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	jobs, err := s.ListDrainJobs(ctx, pluginID, 20)
	if err != nil {
		return nil, err
	}
	result.DrainJobs = jobs
	return result, nil
}

func (s *PluginDrainJobService) ListRuntimeBlockers(ctx context.Context, input ListDrainBlockersInput) (*DrainBlockerPage, error) {
	if s == nil || s.db == nil {
		return nil, dto.NewErrorWithCode(http.StatusServiceUnavailable, ErrCodePluginDrainInvalidRequest, "插件 drain 服务不可用", nil)
	}
	pluginID := strings.TrimSpace(input.PluginID)
	if pluginID == "" {
		return nil, dto.NewErrorWithCode(http.StatusBadRequest, ErrCodePluginDrainInvalidRequest, "缺少插件ID", errors.New("plugin_id required"))
	}
	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		kind = "event_task"
	}
	if kind != "event_task" && kind != "scheduler_job" {
		return nil, dto.NewErrorWithCode(http.StatusBadRequest, ErrCodePluginDrainInvalidRequest, "kind 只允许 event_task 或 scheduler_job", errors.New("invalid blocker kind"))
	}
	page := input.Page
	if page <= 0 {
		page = 1
	}
	pageSize := input.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	resp := &DrainBlockerPage{
		PluginID:    pluginID,
		Kind:        kind,
		GeneratedAt: time.Now().UTC(),
	}
	pagination := &dto.PaginationResponse{Page: page, PageSize: pageSize}
	switch kind {
	case "scheduler_job":
		query := s.schedulerBlockerQuery(ctx, pluginID)
		if err := query.Count(&pagination.Total).Error; err != nil {
			return nil, err
		}
		var rows []DrainBlockerSchedulerJob
		if err := s.schedulerBlockerQuery(ctx, pluginID).
			Select("uuid, name, status, next_run_at, updated_at").
			Order("updated_at DESC, id DESC").
			Limit(pageSize).
			Offset(offset).
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		resp.Items = rows
		resp.JobTotal = pagination.Total
	default:
		query := s.eventTaskBlockerQuery(ctx, pluginID)
		if err := query.Count(&pagination.Total).Error; err != nil {
			return nil, err
		}
		var rows []DrainBlockerEventTask
		if err := s.eventTaskBlockerQuery(ctx, pluginID).
			Select("id, task_id, subscriber_id, topic, status, error_message, last_seen_at, updated_at").
			Order("updated_at DESC, id DESC").
			Limit(pageSize).
			Offset(offset).
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		resp.Items = rows
		resp.EventTotal = pagination.Total
	}
	pagination.CalculatePages()
	resp.Pagination = pagination
	if pagination.Total == 0 {
		if err := s.refreshActiveDrainJobsForPlugin(ctx, pluginID); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (s *PluginDrainJobService) refreshActiveDrainJobsForPlugin(ctx context.Context, pluginID string) error {
	if s == nil || s.jobs == nil {
		return nil
	}
	jobs, err := s.jobs.ListByPlugin(ctx, pluginID, 20)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job == nil {
			continue
		}
		switch strings.TrimSpace(job.Status) {
		case dbsetting.PluginDrainJobStatusRequested,
			dbsetting.PluginDrainJobStatusBlockingNewUsage,
			dbsetting.PluginDrainJobStatusDraining:
			if _, err := s.RefreshDrainJobProgress(ctx, job.JobID); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func normalizeUint64s(values []uint64) []uint64 {
	out := make([]uint64, 0, len(values))
	seen := make(map[uint64]struct{}, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (s *PluginDrainJobService) CreateDrainJob(ctx context.Context, input CreateDrainJobInput) (*dbsetting.PluginDrainJob, error) {
	if s == nil || s.db == nil {
		return nil, dto.NewErrorWithCode(http.StatusServiceUnavailable, ErrCodePluginDrainInvalidRequest, "插件 drain 服务不可用", nil)
	}
	pluginID := strings.TrimSpace(input.PluginID)
	if pluginID == "" {
		return nil, dto.NewErrorWithCode(http.StatusBadRequest, ErrCodePluginDrainInvalidRequest, "缺少插件ID", errors.New("plugin_id required"))
	}
	now := time.Now().UTC()
	job := &dbsetting.PluginDrainJob{
		JobID:               uuid.NewString(),
		PluginID:            pluginID,
		Version:             strings.TrimSpace(input.Version),
		Scope:               drainScope(input.Version),
		Status:              dbsetting.PluginDrainJobStatusDraining,
		Reason:              strings.TrimSpace(input.Reason),
		RequestedByRootUser: reqctx.GetUserID(ctx),
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		jobRepo := reposetting.NewPluginDrainJobRepository(tx)
		cfgRepo := reposetting.NewPluginInstanceConfigRepository(tx)
		if err := jobRepo.Create(ctx, job); err != nil {
			return err
		}
		var affected int64
		var err error
		switch strings.TrimSpace(input.Mode) {
		case "emergency_disable":
			job.Status = dbsetting.PluginDrainJobStatusBlockingNewUsage
			affected, err = cfgRepo.MarkPluginInstancesDisabledByPlatform(ctx, pluginID, job.JobID, now)
		default:
			affected, err = cfgRepo.MarkPluginInstancesDraining(ctx, pluginID, job.JobID, now)
		}
		if err != nil {
			return err
		}
		job.AffectedTenantCount = affected
		job.DrainedTenantCount = 0
		if affected == 0 {
			job.Status = dbsetting.PluginDrainJobStatusReadyToUninstall
		}
		return jobRepo.Update(ctx, job)
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.RefreshDrainJobProgress(ctx, job.JobID); err != nil {
		return nil, err
	}
	job, err = s.getDrainJob(ctx, job.JobID)
	if err != nil {
		return nil, err
	}
	publishPluginDrainStatus(ctx, job, "插件进入准备卸载流程", "插件 "+pluginID+" 已进入 drain，新增使用入口已被阻断。")
	return job, nil
}

func (s *PluginDrainJobService) GetDrainJob(ctx context.Context, jobID string) (*dbsetting.PluginDrainJob, error) {
	if s == nil || s.jobs == nil {
		return nil, dto.NewErrorWithCode(http.StatusServiceUnavailable, ErrCodePluginDrainInvalidRequest, "插件 drain 服务不可用", nil)
	}
	return s.getDrainJob(ctx, jobID)
}

func (s *PluginDrainJobService) getDrainJob(ctx context.Context, jobID string) (*dbsetting.PluginDrainJob, error) {
	job, err := s.jobs.GetByJobID(ctx, jobID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, dto.NewErrorWithCode(http.StatusNotFound, ErrCodePluginDrainNotFound, "插件 drain job 不存在", err)
		}
		return nil, err
	}
	return job, nil
}

func (s *PluginDrainJobService) ListDrainJobs(ctx context.Context, pluginID string, limit int) ([]*dbsetting.PluginDrainJob, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return nil, dto.NewErrorWithCode(http.StatusBadRequest, ErrCodePluginDrainInvalidRequest, "缺少插件ID", errors.New("plugin_id required"))
	}
	items, err := s.jobs.ListByPlugin(ctx, pluginID, limit)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *PluginDrainJobService) RefreshDrainJobProgress(ctx context.Context, jobID string) (*dbsetting.PluginDrainJob, error) {
	if s == nil || s.jobs == nil || s.tenantConfig == nil {
		return nil, dto.NewErrorWithCode(http.StatusServiceUnavailable, ErrCodePluginDrainInvalidRequest, "插件 drain 服务不可用", nil)
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, dto.NewErrorWithCode(http.StatusBadRequest, ErrCodePluginDrainInvalidRequest, "缺少 drain job id", errors.New("job_id required"))
	}
	job, err := s.jobs.GetByJobID(ctx, jobID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, dto.NewErrorWithCode(http.StatusNotFound, ErrCodePluginDrainNotFound, "插件 drain job 不存在", err)
		}
		return nil, err
	}
	blockers, err := s.autoMarkDrainedWhenNoBlockers(ctx, job)
	if err != nil {
		return nil, err
	}
	active, err := s.tenantConfig.CountActiveTenantPluginBindings(ctx, job.PluginID)
	if err != nil {
		return nil, err
	}
	drained, err := s.tenantConfig.CountDrainedTenantPluginBindings(ctx, job.PluginID)
	if err != nil {
		return nil, err
	}
	job.DrainedTenantCount = drained
	previousStatus := strings.TrimSpace(job.Status)
	if blockers > 0 {
		if job.Status == dbsetting.PluginDrainJobStatusReadyToUninstall || job.Status == dbsetting.PluginDrainJobStatusCompleted {
			job.Status = dbsetting.PluginDrainJobStatusDraining
			job.CompletedAt = nil
		}
	} else if active == 0 {
		job.Status = dbsetting.PluginDrainJobStatusReadyToUninstall
		if job.CompletedAt == nil {
			now := time.Now().UTC()
			job.CompletedAt = &now
		}
	} else if job.Status == dbsetting.PluginDrainJobStatusReadyToUninstall || job.Status == dbsetting.PluginDrainJobStatusCompleted {
		job.Status = dbsetting.PluginDrainJobStatusDraining
		job.CompletedAt = nil
	}
	if err := s.jobs.Update(ctx, job); err != nil {
		return nil, err
	}
	if previousStatus != dbsetting.PluginDrainJobStatusReadyToUninstall && job.Status == dbsetting.PluginDrainJobStatusReadyToUninstall {
		publishPluginDrainStatus(ctx, job, "插件可最终卸载", "插件 "+job.PluginID+" 的租户实例已全部 drained，可以执行最终卸载。")
	}
	return job, nil
}

func (s *PluginDrainJobService) autoMarkDrainedWhenNoBlockers(ctx context.Context, job *dbsetting.PluginDrainJob) (int64, error) {
	if s == nil || s.db == nil || s.tenantConfig == nil || job == nil {
		return 0, nil
	}
	if job.Status != dbsetting.PluginDrainJobStatusDraining &&
		job.Status != dbsetting.PluginDrainJobStatusBlockingNewUsage &&
		job.Status != dbsetting.PluginDrainJobStatusRequested {
		return 0, nil
	}
	blockers, err := s.countRuntimeBlockers(ctx, job.PluginID)
	if err != nil {
		return 0, err
	}
	if blockers > 0 {
		if details, detailErr := s.runtimeBlockerDetails(ctx, job.PluginID); detailErr == nil {
			job.LastBlockerJSON = details
		}
		return blockers, nil
	}
	job.LastBlockerJSON = nil
	drained, err := s.tenantConfig.MarkPluginDrainInstancesDrained(ctx, job.PluginID, job.JobID, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	job.DrainedTenantCount += drained
	return 0, nil
}

func (s *PluginDrainJobService) countRuntimeBlockers(ctx context.Context, pluginID string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return 0, nil
	}
	var total int64
	var schedulerCount int64
	if err := s.schedulerBlockerQuery(ctx, pluginID).Count(&schedulerCount).Error; err != nil {
		return 0, err
	}
	total += schedulerCount
	var taskHistoryCount int64
	if err := s.eventTaskBlockerQuery(ctx, pluginID).Count(&taskHistoryCount).Error; err != nil {
		return 0, err
	}
	total += taskHistoryCount
	return total, nil
}

func (s *PluginDrainJobService) runtimeBlockerDetails(ctx context.Context, pluginID string) ([]byte, error) {
	pluginID = strings.TrimSpace(pluginID)
	if s == nil || s.db == nil || pluginID == "" {
		return nil, nil
	}
	details := map[string]any{
		"plugin_id": pluginID,
	}
	var schedulerCount int64
	if err := s.schedulerBlockerQuery(ctx, pluginID).Count(&schedulerCount).Error; err != nil {
		return nil, err
	}
	details["scheduler_job_count"] = schedulerCount
	var schedulerRows []struct {
		UUID      string     `json:"uuid"`
		Name      string     `json:"name"`
		Status    string     `json:"status"`
		NextRunAt *time.Time `json:"next_run_at,omitempty"`
	}
	if err := s.schedulerBlockerQuery(ctx, pluginID).
		Select("uuid, name, status, next_run_at").
		Order("updated_at DESC").
		Limit(5).
		Scan(&schedulerRows).Error; err != nil {
		return nil, err
	}
	details["scheduler_jobs"] = schedulerRows
	var taskCount int64
	if err := s.eventTaskBlockerQuery(ctx, pluginID).Count(&taskCount).Error; err != nil {
		return nil, err
	}
	details["event_task_count"] = taskCount
	var taskRows []struct {
		ID           uint64     `json:"id"`
		TaskID       string     `json:"task_id"`
		SubscriberID string     `json:"subscriber_id"`
		Topic        string     `json:"topic"`
		Status       string     `json:"status"`
		ErrorMessage string     `json:"error_message,omitempty"`
		LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
	}
	if err := s.eventTaskBlockerQuery(ctx, pluginID).
		Select("id, task_id, subscriber_id, topic, status, error_message, last_seen_at").
		Order("updated_at DESC").
		Limit(5).
		Scan(&taskRows).Error; err != nil {
		return nil, err
	}
	details["event_tasks"] = taskRows
	if schedulerCount == 0 && taskCount == 0 {
		return nil, nil
	}
	return json.Marshal(details)
}

func (s *PluginDrainJobService) schedulerBlockerQuery(ctx context.Context, pluginID string) *gorm.DB {
	return s.db.WithContext(ctx).
		Table("scheduler_jobs").
		Where("owner_type = ? AND owner_id = ? AND status IN ?", "plugin", pluginID, []string{"active", "paused"}).
		Where("deleted_at IS NULL")
}

func (s *PluginDrainJobService) eventTaskBlockerQuery(ctx context.Context, pluginID string) *gorm.DB {
	metadataExpr := "metadata::text"
	if s.db.Dialector != nil && strings.EqualFold(s.db.Dialector.Name(), "sqlite") {
		metadataExpr = "metadata"
	}
	return s.db.WithContext(ctx).
		Table("event_task_histories").
		Where("("+metadataExpr+" LIKE ? OR subscriber_id LIKE ? OR topic LIKE ?)", "%"+pluginID+"%", "%"+pluginID+"%", "%"+pluginID+"%").
		Where("status IN ?", []string{"pending", "deferred", "running"}).
		Where("deleted_at IS NULL")
}

func (s *PluginDrainJobService) EnsurePluginAcceptsNewUsage(ctx context.Context, pluginID string) error {
	if s == nil || s.tenantConfig == nil {
		return dto.NewErrorWithCode(http.StatusServiceUnavailable, ErrCodePluginDrainInvalidRequest, "插件 drain 服务不可用", nil)
	}
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return dto.NewErrorWithCode(http.StatusBadRequest, ErrCodePluginDrainInvalidRequest, "缺少插件ID", errors.New("plugin_id required"))
	}
	if s.jobs != nil {
		jobCount, err := s.jobs.CountBlockingByPlugin(ctx, pluginID)
		if err != nil {
			return err
		}
		if jobCount > 0 {
			return dto.WithDetails(
				dto.NewErrorWithCode(http.StatusConflict, ErrCodePluginDraining, "插件正在 drain 或等待最终卸载，禁止新增使用", errors.New("plugin is draining")),
				map[string]interface{}{"plugin_id": pluginID, "blocked_operation": "new_usage", "blocking_drain_job_count": jobCount},
			)
		}
	}
	count, err := s.tenantConfig.CountTenantPluginBindings(ctx, reposetting.ListTenantPluginOptions{
		PluginIDs: []string{pluginID},
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
		return dto.WithDetails(
			dto.NewErrorWithCode(http.StatusConflict, ErrCodePluginDraining, "插件正在 drain 或已被平台禁用，禁止新增使用", errors.New("plugin is draining")),
			map[string]interface{}{"plugin_id": pluginID, "blocked_operation": "new_usage", "draining_instance_count": count},
		)
	}
	return nil
}

func drainScope(version string) string {
	if strings.TrimSpace(version) != "" {
		return dbsetting.PluginDrainJobScopePluginVersion
	}
	return dbsetting.PluginDrainJobScopePlugin
}

func publishPluginDrainStatus(ctx context.Context, job *dbsetting.PluginDrainJob, title, content string) {
	if job == nil {
		return
	}
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if tenantUUID == "" {
		return
	}
	payload := map[string]any{
		"type":                  "warning",
		"title":                 title,
		"content":               content,
		"kind":                  "plugin.drain.status",
		"plugin_id":             strings.TrimSpace(job.PluginID),
		"version":               strings.TrimSpace(job.Version),
		"job_id":                strings.TrimSpace(job.JobID),
		"status":                strings.TrimSpace(job.Status),
		"affected_tenant_count": job.AffectedTenantCount,
		"drained_tenant_count":  job.DrainedTenantCount,
		"createdAt":             time.Now().UTC().Format(time.RFC3339),
		"isRead":                false,
	}
	wsbus.DefaultHub.PublishWithContext(ctx, tenantUUID, eventbus.TopicSystemNotification, payload, reqctx.GetTraceID(ctx))
}
