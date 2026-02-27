package eventfabric

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/replay"
	sharedsvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// ReplayService 抽象回放服务接口。
type ReplayService interface {
	CreateTask(ctx context.Context, input replay.CreateTaskInput) (*replay.Task, error)
	GetTask(ctx context.Context, id string) (*replay.Task, error)
	CancelTask(ctx context.Context, id string, operator string) error
}

// AdminReplayHandlerOptions 构造 handler 所需依赖。
type AdminReplayHandlerOptions struct {
	Service ReplayService
}

// AdminReplayHandler 管理端回放接口。
type AdminReplayHandler struct {
	service ReplayService
}

func NewAdminReplayHandler(opts AdminReplayHandlerOptions) *AdminReplayHandler {
	return &AdminReplayHandler{service: opts.Service}
}

type replayWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type createReplayTaskRequest struct {
	Topic      string        `json:"topic"`
	TraceID    string        `json:"trace_id"`
	Window     *replayWindow `json:"window"`
	Reason     string        `json:"reason"`
	OperatorID string        `json:"operator_id"`
	Shadow     bool          `json:"shadow"`
}

type cancelReplayTaskRequest struct {
	OperatorID string `json:"operator_id"`
}

// CreateTask 创建回放任务。
func (h *AdminReplayHandler) CreateTask(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("replay service unavailable", nil))
		return
	}
	var req createReplayTaskRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid request payload", err))
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	start, end, err := parseReplayWindow(req.Window)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid time range", err))
		return
	}

	normalizedTenant := strings.TrimSpace(tenantUUID)
	normalizedTopic := strings.TrimSpace(req.Topic)
	operatorID := resolveReplayOperator(c.Request.Context(), strings.TrimSpace(req.OperatorID))
	task, err := h.service.CreateTask(c.Request.Context(), replay.CreateTaskInput{
		TenantKey:   normalizedTenant,
		Topic:       normalizedTopic,
		TraceID:     strings.TrimSpace(req.TraceID),
		WindowStart: start,
		WindowEnd:   end,
		Reason:      strings.TrimSpace(req.Reason),
		Operator:    operatorID,
		Shadow:      req.Shadow,
	})
	if err != nil {
		dto.RespondErrorFrom(c, mapReplayCreateTaskError(err, normalizedTenant, normalizedTopic))
		return
	}
	dto.ResponseSuccess(c, taskToDTO(task))
}

// GetTask 查询回放任务状态。
func (h *AdminReplayHandler) GetTask(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("replay service unavailable", nil))
		return
	}
	taskID := c.Param("task_id")
	task, err := h.service.GetTask(c.Request.Context(), strings.TrimSpace(taskID))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("get replay task failed", err))
		return
	}
	if task == nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("replay task not found", nil))
		return
	}
	dto.ResponseSuccess(c, taskToDTO(task))
}

// CancelTask 取消回放任务。
func (h *AdminReplayHandler) CancelTask(c *gin.Context) {
	if h.service == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("replay service unavailable", nil))
		return
	}
	taskID := strings.TrimSpace(c.Param("task_id"))
	var req cancelReplayTaskRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid request payload", err))
		return
	}
	if err := h.service.CancelTask(c.Request.Context(), taskID, strings.TrimSpace(req.OperatorID)); err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("cancel replay task failed", err))
		return
	}
	c.Status(http.StatusNoContent)
}

type replayTaskDTO struct {
	ID            string     `json:"id"`
	TenantUUID    string     `json:"tenant_uuid"`
	Topic         string     `json:"topic"`
	TraceID       string     `json:"trace_id"`
	Status        string     `json:"status"`
	Shadow        bool       `json:"shadow"`
	RequestedBy   string     `json:"requested_by"`
	SubmittedAt   time.Time  `json:"submitted_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	FailureReason string     `json:"failure_reason,omitempty"`
	ResultCount   int        `json:"result_count"`
}

func mapReplayCreateTaskError(err error, tenantUUID string, topic string) error {
	if err == nil {
		return nil
	}
	errMsg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(errMsg, "topic tenant mismatch") {
		wrapped := fmt.Errorf("jwt_tenant=%s, topic=%s: %w", tenantUUID, topic, err)
		return dto.WithCode(dto.NewBadRequest("topic 租户与当前登录租户不一致", wrapped), sharedsvc.ErrorCodeTenantMismatch)
	}
	if errors.Is(err, sharedsvc.ErrUnauthorized) {
		return dto.WithCode(dto.NewForbidden("unauthorized", err), sharedsvc.ErrorCodeUnauthorized)
	}
	if strings.Contains(errMsg, "topic") && strings.Contains(errMsg, "not found") {
		wrapped := fmt.Errorf("jwt_tenant=%s, topic=%s: %w", tenantUUID, topic, err)
		return dto.NewNotFound("topic 未注册或当前环境不可用", wrapped)
	}
	return dto.NewInternal("create replay task failed", err)
}

func resolveReplayOperator(ctx context.Context, requested string) string {
	requested = strings.TrimSpace(requested)
	if requested != "" && strings.Contains(requested, ":") {
		return requested
	}
	claims := reqctx.GetClaims(ctx)
	if claims != nil {
		for _, role := range claims.Roles {
			role = strings.TrimSpace(role)
			if role == "" {
				continue
			}
			if strings.HasPrefix(role, "role:") {
				return role
			}
			return "role:" + role
		}
	}
	if requested != "" {
		return requested
	}
	if reqctx.IsRoot(ctx) {
		return "role:role_admin"
	}
	return strings.TrimSpace(reqctx.GetSubject(ctx))
}

func taskToDTO(task *replay.Task) replayTaskDTO {
	if task == nil {
		return replayTaskDTO{}
	}
	return replayTaskDTO{
		ID:            task.ID,
		TenantUUID:    task.TenantKey,
		Topic:         task.Topic,
		TraceID:       task.TraceID,
		Status:        task.Status,
		Shadow:        task.Shadow,
		RequestedBy:   task.RequestedBy,
		SubmittedAt:   task.SubmittedAt,
		CompletedAt:   task.CompletedAt,
		FailureReason: task.FailureReason,
		ResultCount:   task.ResultCount,
	}
}

func parseReplayWindow(window *replayWindow) (time.Time, time.Time, error) {
	if window == nil {
		return time.Time{}, time.Time{}, nil
	}
	var start, end time.Time
	var err error
	if strings.TrimSpace(window.Start) != "" {
		start, err = time.Parse(time.RFC3339, window.Start)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if strings.TrimSpace(window.End) != "" {
		end, err = time.Parse(time.RFC3339, window.End)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	return start, end, nil
}
