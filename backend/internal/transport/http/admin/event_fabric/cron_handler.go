package eventfabric

import (
	"strings"
	"time"

	workers "github.com/ArtisanCloud/PowerX/internal/app/shared/workers"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

const (
	cronJobRetryDispatch              = "event_fabric.retry_dispatch"
	cronJobAuthorizationChallengeTask = "event_fabric.authorization_challenge_timeout"
	cronJobStateRunning               = "running"
	cronJobStatePaused                = "paused"
	cronJobStateUnavailable           = "unavailable"
)

type AdminCronHandlerOptions struct {
	RetryWorker         *workers.EventFabricRetryWorker
	AuthorizationWorker *workers.EventFabricAuthorizationTimeoutTaskWorker
}

type AdminCronHandler struct {
	retryWorker         *workers.EventFabricRetryWorker
	authorizationWorker *workers.EventFabricAuthorizationTimeoutTaskWorker
}

type cronJobDTO struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Status         string `json:"status"`
	Kind           string `json:"kind"`
	IntervalSec    int64  `json:"interval_sec,omitempty"`
	BatchSize      int    `json:"batch_size,omitempty"`
	SubscriberID   string `json:"subscriber_id,omitempty"`
	TenantKey      string `json:"tenant_key,omitempty"`
	NextRunAt      string `json:"next_run_at,omitempty"`
	SupportsPause  bool   `json:"supports_pause"`
	SupportsRunNow bool   `json:"supports_run_now"`
}

func NewAdminCronHandler(opts AdminCronHandlerOptions) *AdminCronHandler {
	return &AdminCronHandler{
		retryWorker:         opts.RetryWorker,
		authorizationWorker: opts.AuthorizationWorker,
	}
}

func (h *AdminCronHandler) ListJobs(c *gin.Context) {
	if _, err := reqctx.RequireTenantUUIDFromGin(c); err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	now := time.Now().UTC()
	items := make([]cronJobDTO, 0, 2)

	items = append(items, h.retryDispatchJob(now))
	items = append(items, h.authorizationTimeoutJob(now))

	dto.ResponseSuccess(c, gin.H{
		"items": items,
		"now":   now.Format(time.RFC3339),
	})
}

func (h *AdminCronHandler) PauseJob(c *gin.Context) {
	if _, err := reqctx.RequireTenantUUIDFromGin(c); err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	jobID := strings.TrimSpace(c.Param("job_id"))
	switch jobID {
	case cronJobRetryDispatch:
		if h.retryWorker == nil {
			dto.RespondErrorFrom(c, dto.NewNotFound("cron job not found", nil))
			return
		}
		h.retryWorker.Pause()
		dto.ResponseSuccess(c, h.retryDispatchJob(time.Now().UTC()))
		return
	case cronJobAuthorizationChallengeTask:
		if h.authorizationWorker == nil {
			dto.RespondErrorFrom(c, dto.NewNotFound("cron job not found", nil))
			return
		}
		h.authorizationWorker.Pause()
		dto.ResponseSuccess(c, h.authorizationTimeoutJob(time.Now().UTC()))
		return
	default:
		dto.RespondErrorFrom(c, dto.NewNotFound("cron job not found", nil))
		return
	}
}

func (h *AdminCronHandler) ResumeJob(c *gin.Context) {
	if _, err := reqctx.RequireTenantUUIDFromGin(c); err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	jobID := strings.TrimSpace(c.Param("job_id"))
	switch jobID {
	case cronJobRetryDispatch:
		if h.retryWorker == nil {
			dto.RespondErrorFrom(c, dto.NewNotFound("cron job not found", nil))
			return
		}
		h.retryWorker.Resume()
		dto.ResponseSuccess(c, h.retryDispatchJob(time.Now().UTC()))
		return
	case cronJobAuthorizationChallengeTask:
		if h.authorizationWorker == nil {
			dto.RespondErrorFrom(c, dto.NewNotFound("cron job not found", nil))
			return
		}
		h.authorizationWorker.Resume()
		dto.ResponseSuccess(c, h.authorizationTimeoutJob(time.Now().UTC()))
		return
	default:
		dto.RespondErrorFrom(c, dto.NewNotFound("cron job not found", nil))
		return
	}
}

func (h *AdminCronHandler) RunNow(c *gin.Context) {
	if _, err := reqctx.RequireTenantUUIDFromGin(c); err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	jobID := strings.TrimSpace(c.Param("job_id"))
	switch jobID {
	case cronJobRetryDispatch:
		if h.retryWorker == nil {
			dto.RespondErrorFrom(c, dto.NewNotFound("cron job not found", nil))
			return
		}
		h.retryWorker.TriggerNow(c.Request.Context())
		dto.ResponseSuccess(c, h.retryDispatchJob(time.Now().UTC()))
		return
	case cronJobAuthorizationChallengeTask:
		if h.authorizationWorker == nil {
			dto.RespondErrorFrom(c, dto.NewNotFound("cron job not found", nil))
			return
		}
		h.authorizationWorker.TriggerNow(c.Request.Context())
		dto.ResponseSuccess(c, h.authorizationTimeoutJob(time.Now().UTC()))
		return
	default:
		dto.RespondErrorFrom(c, dto.NewNotFound("cron job not found", nil))
		return
	}
}

func (h *AdminCronHandler) retryDispatchJob(now time.Time) cronJobDTO {
	if h.retryWorker == nil {
		return cronJobDTO{
			ID:             cronJobRetryDispatch,
			Name:           "Event Fabric Retry Dispatch",
			Description:    "扫描到期重试任务并发布到统一事件总线",
			Status:         cronJobStateUnavailable,
			Kind:           "interval",
			SupportsPause:  false,
			SupportsRunNow: false,
		}
	}
	status := cronJobStateRunning
	if h.retryWorker.IsPaused() {
		status = cronJobStatePaused
	}
	interval := h.retryWorker.Interval()
	nextRunAt := ""
	if status == cronJobStateRunning && interval > 0 {
		nextRunAt = now.Add(interval).Format(time.RFC3339)
	}
	return cronJobDTO{
		ID:             cronJobRetryDispatch,
		Name:           "Event Fabric Retry Dispatch",
		Description:    "扫描到期重试任务并发布到统一事件总线",
		Status:         status,
		Kind:           "interval",
		IntervalSec:    int64(interval / time.Second),
		BatchSize:      h.retryWorker.BatchSize(),
		NextRunAt:      nextRunAt,
		SupportsPause:  true,
		SupportsRunNow: true,
	}
}

func (h *AdminCronHandler) authorizationTimeoutJob(now time.Time) cronJobDTO {
	if h.authorizationWorker == nil {
		return cronJobDTO{
			ID:             cronJobAuthorizationChallengeTask,
			Name:           "Authorization Challenge Timeout",
			Description:    "消费授权超时任务并执行过期处理",
			Status:         cronJobStateUnavailable,
			Kind:           "queue",
			SupportsPause:  false,
			SupportsRunNow: false,
		}
	}
	status := cronJobStateRunning
	if h.authorizationWorker.IsPaused() {
		status = cronJobStatePaused
	}
	nextRunAt := ""
	if status == cronJobStateRunning {
		nextRunAt = now.Add(h.authorizationWorker.WaitTimeout()).Format(time.RFC3339)
	}
	return cronJobDTO{
		ID:             cronJobAuthorizationChallengeTask,
		Name:           "Authorization Challenge Timeout",
		Description:    "消费授权超时任务并执行过期处理",
		Status:         status,
		Kind:           "queue",
		BatchSize:      h.authorizationWorker.BatchSize(),
		SubscriberID:   h.authorizationWorker.SubscriberID(),
		TenantKey:      h.authorizationWorker.TenantKey(),
		NextRunAt:      nextRunAt,
		SupportsPause:  true,
		SupportsRunNow: true,
	}
}
