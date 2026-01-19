package notifications

import (
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	deps *shared.Deps
}

func NewHandler(deps *shared.Deps) *Handler {
	return &Handler{deps: deps}
}

type TestNotificationRequest struct {
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	Type        string         `json:"type"`
	Category    string         `json:"category"`
	IsImportant bool           `json:"isImportant"`
	Metadata    map[string]any `json:"metadata"`
}

func (h *Handler) PushTestNotification(c *gin.Context) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid required", err)
		return
	}

	var req TestNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}

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

	payload := map[string]any{
		"title":       title,
		"content":     content,
		"type":        kind,
		"category":    category,
		"isRead":      false,
		"isImportant": req.IsImportant,
		"createdAt":   time.Now().UTC().Format(time.RFC3339),
		"updatedAt":   time.Now().UTC().Format(time.RFC3339),
		"metadata":    req.Metadata,
	}

	bus.DefaultHub.Publish(tenantUUID, bus.TopicSystemNotification, payload, reqctx.GetTraceID(c.Request.Context()))
	dto.ResponseSuccess(c, gin.H{"ok": true})
}
