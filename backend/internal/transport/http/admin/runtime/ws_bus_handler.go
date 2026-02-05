package runtime

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
)

type wsBusPublishRequest struct {
	Topic   string `json:"topic"`
	Payload any    `json:"payload"`
	TraceID string `json:"trace_id"`
}

type wsBusHandler struct{}

func newWSBusHandler() *wsBusHandler {
	return &wsBusHandler{}
}

func (h *wsBusHandler) publish(c *gin.Context) {
	var req wsBusPublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid required", err)
		return
	}
	memberID := reqctx.GetMemberID(c.Request.Context())
	isRoot := reqctx.IsRoot(c.Request.Context())
	if !isRoot && memberID == 0 {
		dto.ResponseError(c, http.StatusForbidden, "member required", nil)
		return
	}

	reqTopic := strings.TrimSpace(req.Topic)
	if reqTopic == "" {
		dto.ResponseError(c, http.StatusBadRequest, "topic required", nil)
		return
	}
	if !bus.IsPublishTopicAllowed(reqTopic) {
		dto.ResponseError(c, http.StatusForbidden, "topic not allowed", nil)
		return
	}

	traceID := strings.TrimSpace(req.TraceID)
	if traceID == "" {
		traceID = reqctx.GetTraceID(c.Request.Context())
	}

	bus.DefaultHub.Publish(strings.TrimSpace(tenantUUID), reqTopic, req.Payload, traceID)
	logger.InfoF(c.Request.Context(), "[ws-bus] publish topic=%s tenant=%s trace_id=%s", reqTopic, strings.TrimSpace(tenantUUID), traceID)

	dto.ResponseSuccessWithStatusAndPayload(c, http.StatusOK, map[string]interface{}{
		"topic":       reqTopic,
		"tenant_uuid": strings.TrimSpace(tenantUUID),
	})
}
