package scheduler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	runtimescheduler "github.com/ArtisanCloud/PowerX/internal/service/runtime_scheduler"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc *runtimescheduler.Service
}

func NewHandler(deps *shared.Deps) *Handler {
	if deps == nil {
		return nil
	}
	if deps.RuntimeScheduler != nil && deps.RuntimeScheduler.Service != nil {
		return &Handler{svc: deps.RuntimeScheduler.Service}
	}
	if deps.DB == nil {
		return nil
	}
	return &Handler{svc: runtimescheduler.NewService(runtimescheduler.Options{DB: deps.DB, EventBus: deps.EventBus})}
}

type createJobRequest struct {
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

type updateJobRequest struct {
	Name           *string        `json:"name"`
	ScheduleType   *string        `json:"schedule_type"`
	ScheduleExpr   *string        `json:"schedule_expr"`
	Timezone       *string        `json:"timezone"`
	Payload        map[string]any `json:"payload"`
	MisfirePolicy  *string        `json:"misfire_policy"`
	OverlapPolicy  *string        `json:"overlap_policy"`
	IdempotencyKey *string        `json:"idempotency_key"`
}

func (h *Handler) CreateJob(c *gin.Context) {
	var req createJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	job, err := h.svc.CreateJob(c.Request.Context(), runtimescheduler.JobSpec{
		TenantUUID:     req.TenantUUID,
		OwnerType:      req.OwnerType,
		OwnerID:        req.OwnerID,
		Name:           req.Name,
		ScheduleType:   req.ScheduleType,
		ScheduleExpr:   req.ScheduleExpr,
		Timezone:       req.Timezone,
		Payload:        req.Payload,
		MisfirePolicy:  req.MisfirePolicy,
		OverlapPolicy:  req.OverlapPolicy,
		IdempotencyKey: req.IdempotencyKey,
	}, resolveOperator(c), reqctx.GetTraceID(c.Request.Context()))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"job": job})
}

func (h *Handler) UpdateJob(c *gin.Context) {
	var req updateJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	job, err := h.svc.UpdateJob(c.Request.Context(), runtimescheduler.UpdateJobInput{
		JobID:          c.Param("job_id"),
		Name:           req.Name,
		ScheduleType:   req.ScheduleType,
		ScheduleExpr:   req.ScheduleExpr,
		Timezone:       req.Timezone,
		Payload:        req.Payload,
		MisfirePolicy:  req.MisfirePolicy,
		OverlapPolicy:  req.OverlapPolicy,
		IdempotencyKey: req.IdempotencyKey,
		Operator:       resolveOperator(c),
		TraceID:        reqctx.GetTraceID(c.Request.Context()),
	})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"job": job})
}

func (h *Handler) GetJob(c *gin.Context) {
	job, err := h.svc.GetJob(c.Request.Context(), c.Param("job_id"))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"job": job})
}

func (h *Handler) ListJobs(c *gin.Context) {
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("page_size", "50"), 50)
	items, total, err := h.svc.ListJobs(c.Request.Context(), runtimescheduler.ListJobsInput{
		TenantUUID: strings.TrimSpace(c.Query("tenant_uuid")),
		OwnerType:  strings.TrimSpace(c.Query("owner_type")),
		OwnerID:    strings.TrimSpace(c.Query("owner_id")),
		Status:     strings.TrimSpace(c.Query("status")),
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "pagination": gin.H{"total": total, "page": page, "page_size": pageSize}})
}

func (h *Handler) PauseJob(c *gin.Context) {
	job, err := h.svc.PauseJob(c.Request.Context(), c.Param("job_id"), resolveOperator(c), reqctx.GetTraceID(c.Request.Context()))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"job": job})
}

func (h *Handler) ResumeJob(c *gin.Context) {
	job, err := h.svc.ResumeJob(c.Request.Context(), c.Param("job_id"), resolveOperator(c), reqctx.GetTraceID(c.Request.Context()))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"job": job})
}

func (h *Handler) TriggerJob(c *gin.Context) {
	result, err := h.svc.TriggerJob(c.Request.Context(), c.Param("job_id"), resolveOperator(c), reqctx.GetTraceID(c.Request.Context()))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func (h *Handler) ListRuns(c *gin.Context) {
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("page_size", "50"), 50)
	items, total, err := h.svc.ListRuns(c.Request.Context(), runtimescheduler.ListRunsInput{
		JobID:    c.Param("job_id"),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "pagination": gin.H{"total": total, "page": page, "page_size": pageSize}})
}

func resolveOperator(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if subject := strings.TrimSpace(reqctx.GetSubject(c.Request.Context())); subject != "" {
		return subject
	}
	if claims := reqctx.GetClaims(c.Request.Context()); claims != nil {
		if claims.MemberUUID != "" {
			return claims.MemberUUID
		}
		if claims.UserUUID != "" {
			return claims.UserUUID
		}
	}
	return ""
}

func parseInt(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
