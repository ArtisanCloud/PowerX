package notifications

import (
	"encoding/json"
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
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type handler struct {
	svc *notificationssvc.Service
}

type createNotificationRequest struct {
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	Type        string         `json:"type"`
	Category    string         `json:"category"`
	IsImportant bool           `json:"is_important"`
	MemberUUID  string         `json:"member_uuid"`
	Metadata    map[string]any `json:"metadata"`
}

type notificationView struct {
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
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func newHandler(deps *shared.Deps) *handler {
	if deps == nil {
		return &handler{}
	}
	if deps.Notifications != nil {
		return &handler{svc: deps.Notifications}
	}
	if deps.DB != nil {
		return &handler{svc: notificationssvc.NewService(deps.DB)}
	}
	return &handler{}
}

func (h *handler) create(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid required", err)
		return
	}

	var req createNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}

	memberUUID := strings.TrimSpace(req.MemberUUID)
	if memberUUID != "" {
		if _, parseErr := uuid.Parse(memberUUID); parseErr != nil {
			dto.ResponseError(c, http.StatusBadRequest, "notification.member_uuid_invalid", parseErr)
			return
		}
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		dto.ResponseError(c, http.StatusBadRequest, "notification.title_required", nil)
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		dto.ResponseError(c, http.StatusBadRequest, "notification.content_required", nil)
		return
	}

	var metadata datatypes.JSON
	if len(req.Metadata) > 0 {
		raw, err := json.Marshal(req.Metadata)
		if err != nil {
			dto.ResponseError(c, http.StatusBadRequest, "invalid metadata", err)
			return
		}
		metadata = datatypes.JSON(raw)
	}

	item, err := h.svc.Create(c.Request.Context(), notificationssvc.CreateInput{
		TenantUUID:  tenantUUID,
		MemberUUID:  memberUUID,
		Title:       title,
		Content:     content,
		Type:        strings.TrimSpace(req.Type),
		Category:    strings.TrimSpace(req.Category),
		IsImportant: req.IsImportant,
		Metadata:    metadata,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "create notification failed", err)
		return
	}

	payload := toNotificationView(item)
	bus.DefaultHub.PublishWithContext(c.Request.Context(), tenantUUID, eventbus.TopicSystemNotification, payload, reqctx.GetTraceID(c.Request.Context()))
	dto.ResponseSuccess(c, payload)
}

func toNotificationView(item *notificationmodel.Notification) notificationView {
	view := notificationView{
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
	}
	if len(item.Metadata) > 0 {
		var metadata map[string]any
		if err := json.Unmarshal(item.Metadata, &metadata); err == nil {
			view.Metadata = metadata
		}
	}
	return view
}
