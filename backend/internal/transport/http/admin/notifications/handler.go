package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	notificationssvc "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	"github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	notificationmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	deps *shared.Deps
	svc  *notificationssvc.Service
}

func NewHandler(deps *shared.Deps) *Handler {
	var svc *notificationssvc.Service
	if deps != nil {
		svc = deps.Notifications
		if svc == nil && deps.DB != nil {
			svc = notificationssvc.NewService(deps.DB)
		}
	}
	return &Handler{deps: deps, svc: svc}
}

type TestNotificationRequest struct {
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	Type        string         `json:"type"`
	Category    string         `json:"category"`
	IsImportant bool           `json:"isImportant"`
	Metadata    map[string]any `json:"metadata"`
}

type ListNotificationRequest struct {
	Category    string `form:"category"`
	Type        string `form:"type"`
	IsRead      *bool  `form:"is_read"`
	IsImportant *bool  `form:"is_important"`
	dto.PaginationRequest
}

type NotificationView struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	Type        string         `json:"type"`
	Category    string         `json:"category"`
	IsRead      bool           `json:"isRead"`
	IsImportant bool           `json:"isImportant"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	UserID      string         `json:"userId,omitempty"`
	RelatedID   string         `json:"relatedId,omitempty"`
	RelatedType string         `json:"relatedType,omitempty"`
	Actions     []any          `json:"actions,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func toNotificationView(item *notificationmodel.Notification) NotificationView {
	view := NotificationView{
		ID:          item.UUID.String(),
		Title:       item.Title,
		Content:     item.Content,
		Type:        item.Type,
		Category:    item.Category,
		IsRead:      item.IsRead,
		IsImportant: item.IsImportant,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
		UserID:      strings.TrimSpace(item.MemberUUID),
		RelatedID:   item.RelatedID,
		RelatedType: item.RelatedType,
	}
	if len(item.Actions) > 0 {
		var actions []any
		_ = json.Unmarshal(item.Actions, &actions)
		view.Actions = actions
	}
	if len(item.Metadata) > 0 {
		var metadata map[string]any
		_ = json.Unmarshal(item.Metadata, &metadata)
		view.Metadata = metadata
	}
	return view
}

func (h *Handler) List(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid required", err)
		return
	}
	memberUUID := strings.TrimSpace(reqctx.GetSubject(c.Request.Context()))

	var req ListNotificationRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	req.SetDefaultPagination()

	items, total, err := h.svc.List(c.Request.Context(), notificationssvc.ListInput{
		TenantUUID:  tenantUUID,
		MemberUUID:  memberUUID,
		Category:    strings.TrimSpace(req.Category),
		Type:        strings.TrimSpace(req.Type),
		IsRead:      req.IsRead,
		IsImportant: req.IsImportant,
		Page:        req.Page,
		PageSize:    req.PageSize,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取通知列表失败", err)
		return
	}
	views := make([]NotificationView, 0, len(items))
	for i := range items {
		views = append(views, toNotificationView(&items[i]))
	}
	dto.ResponseList(c, views, &dto.PaginationResponse{Total: total, Page: req.Page, PageSize: req.PageSize})
}

func (h *Handler) Get(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid required", err)
		return
	}
	memberUUID := strings.TrimSpace(reqctx.GetSubject(c.Request.Context()))
	id := strings.TrimSpace(c.Param("uuid"))
	item, err := h.svc.Get(c.Request.Context(), tenantUUID, memberUUID, id)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取通知失败", err)
		return
	}
	if item == nil {
		dto.ResponseError(c, http.StatusNotFound, "通知不存在", errors.New("not found"))
		return
	}
	dto.ResponseSuccess(c, toNotificationView(item))
}

func (h *Handler) MarkRead(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid required", err)
		return
	}
	memberUUID := strings.TrimSpace(reqctx.GetSubject(c.Request.Context()))
	id := strings.TrimSpace(c.Param("uuid"))
	if err := h.svc.MarkRead(c.Request.Context(), tenantUUID, memberUUID, id); err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "更新已读状态失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *Handler) Delete(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid required", err)
		return
	}
	memberUUID := strings.TrimSpace(reqctx.GetSubject(c.Request.Context()))
	id := strings.TrimSpace(c.Param("uuid"))
	if err := h.svc.Delete(c.Request.Context(), tenantUUID, memberUUID, id); err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "删除通知失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *Handler) PushTestNotification(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid required", err)
		return
	}
	memberUUID := strings.TrimSpace(reqctx.GetSubject(c.Request.Context()))

	var req TestNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	item, err := h.createNotification(c.Request.Context(), tenantUUID, memberUUID, req)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "写入通知失败", err)
		return
	}
	payload := toNotificationView(item)
	bus.DefaultHub.Publish(tenantUUID, eventbus.TopicSystemNotification, payload, reqctx.GetTraceID(c.Request.Context()))
	dto.ResponseSuccess(c, payload)
}

func (h *Handler) PushTestNotificationQueue(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	if h.deps == nil || h.deps.EventFabric == nil || h.deps.EventFabric.TaskDriver == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "event_fabric task driver unavailable", errors.New("task_driver_unavailable"))
		return
	}

	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid required", err)
		return
	}
	memberUUID := strings.TrimSpace(reqctx.GetSubject(c.Request.Context()))

	var req TestNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}

	item, err := h.createNotification(c.Request.Context(), tenantUUID, memberUUID, req)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "写入通知失败", err)
		return
	}
	payload := toNotificationView(item)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "序列化通知失败", err)
		return
	}

	taskID := fmt.Sprintf("notification.dispatch.%s.%d", strings.TrimSpace(payload.ID), time.Now().UTC().UnixMilli())
	if strings.TrimSpace(payload.ID) == "" {
		taskID = fmt.Sprintf("notification.dispatch.%s.%d", uuid.NewString(), time.Now().UTC().UnixMilli())
	}

	enqueueErr := h.deps.EventFabric.TaskDriver.Enqueue(c.Request.Context(), event_bus.TaskMessage{
		ID:           taskID,
		TenantKey:    "global",
		SubscriberID: eventbus.SubscriberSystemNotificationDispatch,
		Topic:        eventbus.TopicSystemNotification,
		Payload:      payloadBytes,
		TraceID:      reqctx.GetTraceID(c.Request.Context()),
		VisibleAt:    time.Now().UTC(),
		Metadata: map[string]string{
			"kind":        "queue_notification_debug",
			"tenant_uuid": tenantUUID,
			"member_uuid": memberUUID,
		},
	})
	if enqueueErr != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "入队失败", enqueueErr)
		return
	}

	dto.ResponseSuccess(c, gin.H{
		"task_id":       taskID,
		"subscriber_id": eventbus.SubscriberSystemNotificationDispatch,
		"topic":         eventbus.TopicSystemNotification,
		"payload":       payload,
	})
}

func (h *Handler) createNotification(ctx context.Context, tenantUUID, memberUUID string, req TestNotificationRequest) (*notificationmodel.Notification, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "系统通知"
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		content = "这是一条测试通知"
	}
	kind := strings.TrimSpace(req.Type)
	if kind == "" {
		kind = "info"
	}
	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = "system"
	}

	metaJSON, _ := json.Marshal(req.Metadata)
	return h.svc.Create(ctx, notificationssvc.CreateInput{
		TenantUUID:  tenantUUID,
		MemberUUID:  memberUUID,
		Title:       title,
		Content:     content,
		Type:        kind,
		Category:    category,
		IsImportant: req.IsImportant,
		Metadata:    metaJSON,
	})
}
