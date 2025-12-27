package capability_registry

import (
	"encoding/json"
	"net/http"
	"time"

	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	capability_registrydto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry/dto"
	"github.com/gin-gonic/gin"
)

// RouterHandler 处理 Router 相关 HTTP 请求。
type RouterHandler struct {
	service *router.Service
}

// NewRouterHandler 构造 RouterHandler。
func NewRouterHandler(service *router.Service) *RouterHandler {
	if service == nil {
		panic("router handler requires service")
	}
	return &RouterHandler{service: service}
}

type invokeRequest struct {
	CapabilityID string            `json:"capability_id" binding:"required"`
	TenantUUID   string            `json:"tenant_uuid" binding:"required"`
	Payload      json.RawMessage   `json:"payload"`
	StickyKey    string            `json:"sticky_key"`
	TimeoutMs    int               `json:"timeout_ms"`
	Headers      map[string]string `json:"headers"`
}

type invokeResponse struct {
	AdapterID    string `json:"adapter_id"`
	Endpoint     string `json:"endpoint"`
	Transport    string `json:"transport"`
	FallbackUsed bool   `json:"fallback_used"`
	Payload      any    `json:"payload,omitempty"`
	LatencyMs    int64  `json:"latency_ms"`
}

type reportHealthRequest struct {
	CapabilityID string `json:"capability_id" binding:"required"`
	TenantUUID   string `json:"tenant_uuid" binding:"required"`
	AdapterID    string `json:"adapter_id" binding:"required"`
	Status       string `json:"status" binding:"required"`
	Reason       string `json:"reason"`
	Failures     uint32 `json:"failures"`
}

// Invoke 处理路由调用。
func (h *RouterHandler) Invoke(ctx *gin.Context) {
	var req invokeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		capability_registrydto.RespondError(ctx, capability_registrydto.ErrInvalidRequest, err)
		return
	}
	result, err := h.service.Invoke(ctx.Request.Context(), router.InvokeRequest{
		CapabilityID: req.CapabilityID,
		TenantUUID:   trimTenantUUID(req.TenantUUID),
		Payload:      req.Payload,
		Timeout:      timeDuration(req.TimeoutMs),
		StickyKey:    req.StickyKey,
	})
	if err != nil {
		capability_registrydto.RespondError(ctx, capability_registrydto.ErrInvokeFailed, err)
		return
	}
	var payload any
	if len(result.Payload) > 0 {
		var generic any
		if json.Unmarshal(result.Payload, &generic) == nil {
			payload = generic
		} else {
			payload = string(result.Payload)
		}
	}
	ctx.JSON(http.StatusOK, invokeResponse{
		AdapterID:    result.AdapterID,
		Endpoint:     result.Endpoint,
		Transport:    result.Transport,
		FallbackUsed: result.FallbackUsed,
		Payload:      payload,
		LatencyMs:    result.Latency.Milliseconds(),
	})
}

// ReportHealth 处理健康上报。
func (h *RouterHandler) ReportHealth(ctx *gin.Context) {
	var req reportHealthRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		capability_registrydto.RespondError(ctx, capability_registrydto.ErrInvalidRequest, err)
		return
	}
	if err := h.service.ReportHealth(ctx.Request.Context(), router.ReportHealthInput{
		CapabilityID: req.CapabilityID,
		TenantUUID:   trimTenantUUID(req.TenantUUID),
		AdapterID:    req.AdapterID,
		Status:       req.Status,
		Reason:       req.Reason,
		Failures:     req.Failures,
	}); err != nil {
		capability_registrydto.RespondError(ctx, capability_registrydto.ErrInternal, err)
		return
	}
	ctx.Status(http.StatusAccepted)
}

func timeDuration(timeoutMs int) time.Duration {
	if timeoutMs <= 0 {
		return 0
	}
	return time.Duration(timeoutMs) * time.Millisecond
}
