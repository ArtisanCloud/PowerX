package capability_registry

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	sandbox "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/sandbox"
)

// SandboxHandler 处理 Router Sandbox HTTP 请求。
type SandboxHandler struct {
	service *sandbox.Service
}

// NewSandboxHandler 构造 SandboxHandler。
func NewSandboxHandler(service *sandbox.Service) *SandboxHandler {
	if service == nil {
		panic("sandbox handler requires service")
	}
	return &SandboxHandler{service: service}
}

type sandboxInvokeRequest struct {
	CapabilityID string                      `json:"capability_id" binding:"required"`
	TenantUUID   string                      `json:"tenant_uuid" binding:"required"`
	Payload      json.RawMessage             `json:"payload"`
	StickyKey    string                      `json:"sticky_key"`
	TimeoutMs    int                         `json:"timeout_ms"`
	Override     *routerOverrideRegistration `json:"registration_override"`
}

type routerOverrideRegistration struct {
	CapabilityID  string                   `json:"capability_id"`
	TenantUUID    string                   `json:"tenant_uuid"`
	Status        string                   `json:"status"`
	Adapters      []router.AdapterEndpoint `json:"adapters"`
	RoutingPolicy router.RoutingPolicy     `json:"routing_policy"`
	FallbackPlan  *router.FallbackPlan     `json:"fallback_plan"`
}

type sandboxInvokeResponse struct {
	AdapterID    string `json:"adapter_id"`
	Endpoint     string `json:"endpoint"`
	Transport    string `json:"transport"`
	FallbackUsed bool   `json:"fallback_used"`
	Payload      any    `json:"payload,omitempty"`
	LatencyMs    int64  `json:"latency_ms"`
}

// Invoke 模拟路由调用，不改变实际路由状态。
func (h *SandboxHandler) Invoke(ctx *gin.Context) {
	var req sandboxInvokeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": "sandbox.invalid_request", "message": err.Error()})
		return
	}
	var override *router.Registration
	if req.Override != nil {
		overrideTenant := trimTenantUUID(req.Override.TenantUUID)
		override = &router.Registration{
			CapabilityID:  req.Override.CapabilityID,
			TenantUUID:    overrideTenant,
			Status:        req.Override.Status,
			Adapters:      req.Override.Adapters,
			RoutingPolicy: req.Override.RoutingPolicy,
			FallbackPlan:  req.Override.FallbackPlan,
		}
	}
	tenantUUID := trimTenantUUID(req.TenantUUID)
	result, err := h.service.SimulateInvoke(ctx.Request.Context(), req.CapabilityID, tenantUUID, router.InvokeRequest{
		TenantUUID: tenantUUID,
		Payload:    req.Payload,
		Timeout:    routerTimeDuration(req.TimeoutMs),
		StickyKey:  req.StickyKey,
	}, override)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": "sandbox.invoke_failed", "message": err.Error()})
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
	ctx.JSON(http.StatusOK, sandboxInvokeResponse{
		AdapterID:    result.AdapterID,
		Endpoint:     result.Endpoint,
		Transport:    result.Transport,
		FallbackUsed: result.FallbackUsed,
		Payload:      payload,
		LatencyMs:    result.Latency.Milliseconds(),
	})
}

func routerTimeDuration(timeoutMs int) time.Duration {
	if timeoutMs <= 0 {
		return 0
	}
	return time.Duration(timeoutMs) * time.Millisecond
}
