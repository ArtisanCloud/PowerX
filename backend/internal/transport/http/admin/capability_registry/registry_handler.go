package capability_registry

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
)

// AdminHandlerOptions 注入 HTTP Handler 依赖。
type AdminHandlerOptions struct {
	Service *registry.Service
}

// AdminHandler 处理能力注册 REST 请求。
type AdminHandler struct {
	svc *registry.Service
}

// NewAdminHandler 创建管理端 Handler。
func NewAdminHandler(opts AdminHandlerOptions) *AdminHandler {
	if opts.Service == nil {
		panic("capability registry handler requires service")
	}
	return &AdminHandler{svc: opts.Service}
}

func (h *AdminHandler) CreateCapability(c *gin.Context) {
	var req registrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("registry.invalid_request", err.Error()))
		return
	}
	actor := c.GetHeader("X-Actor-ID")
	input := registry.CreateRegistrationInput{
		Registration: req.toPayload(),
		Actor:        actor,
	}
	res, err := h.svc.CreateRegistration(c.Request.Context(), input)
	if err != nil {
		h.handleError(c, err)
		return
	}
	setETag(c, res.Version)
	c.JSON(http.StatusCreated, registrationSummary(res))
}

func (h *AdminHandler) GetCapability(c *gin.Context) {
	capabilityID := c.Param("capabilityId")
	tenantID := c.Param("tenantId")
	versionParam := c.Query("version")

	var opts registry.GetRegistrationOptions
	if versionParam != "" {
		opts.VersionSelector = versionParam
		if v, err := strconv.ParseUint(versionParam, 10, 64); err == nil {
			opts.Version = v
		}
	}
	res, err := h.svc.GetRegistration(c.Request.Context(), capabilityID, tenantID, opts)
	if err != nil {
		h.handleError(c, err)
		return
	}
	setETag(c, res.Version)
	c.JSON(http.StatusOK, registrationDetail(res))
}

func (h *AdminHandler) UpdateCapability(c *gin.Context) {
	var req registrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("registry.invalid_request", err.Error()))
		return
	}
	capabilityID := c.Param("capabilityId")
	tenantID := c.Param("tenantId")

	if req.CapabilityID == "" {
		req.CapabilityID = capabilityID
	}
	if req.TenantID == "" {
		req.TenantID = tenantID
	}

	ifMatch := c.GetHeader("If-Match")
	if req.Version == nil && ifMatch != "" {
		if v, err := parseETag(ifMatch); err == nil {
			req.Version = &v
		}
	}
	if req.Version == nil {
		c.JSON(http.StatusPreconditionFailed, errorResponse("registry.version_required", "version header missing"))
		return
	}

	actor := c.GetHeader("X-Actor-ID")
	payload := req.toPayload()
	payload.Version = *req.Version
	input := registry.UpdateRegistrationInput{
		Registration: payload,
		Actor:        actor,
	}
	res, err := h.svc.UpdateRegistration(c.Request.Context(), input)
	if err != nil {
		h.handleError(c, err)
		return
	}
	setETag(c, res.Version)
	c.JSON(http.StatusOK, registrationSummary(res))
}

func (h *AdminHandler) DisableCapability(c *gin.Context) {
	capabilityID := c.Param("capabilityId")
	tenantID := c.Param("tenantId")
	var req disableRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, errorResponse("registry.invalid_request", err.Error()))
		return
	}
	ifMatch := c.GetHeader("If-Match")
	var version uint64
	if req.Version != nil {
		version = *req.Version
	} else if ifMatch != "" {
		if v, err := parseETag(ifMatch); err == nil {
			version = v
		}
	}
	actor := c.GetHeader("X-Actor-ID")
	res, err := h.svc.DisableRegistration(c.Request.Context(), registry.DisableRegistrationInput{
		CapabilityID: capabilityID,
		TenantID:     tenantID,
		Reason:       req.Reason,
		Actor:        actor,
		Version:      version,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	setETag(c, res.Version)
	c.JSON(http.StatusAccepted, registrationSummary(res))
}

func (h *AdminHandler) handleError(c *gin.Context, err error) {
	switch {
	case errorsIs(err, registry.ErrRegistrationNotFound):
		c.JSON(http.StatusNotFound, errorResponse("registry.not_found", err.Error()))
	case errorsIs(err, registry.ErrVersionConflict):
		c.JSON(http.StatusPreconditionFailed, errorResponse("registry.version_conflict", err.Error()))
	case errorsIs(err, registry.ErrInvalidPayload):
		c.JSON(http.StatusUnprocessableEntity, errorResponse("registry.invalid_payload", err.Error()))
	default:
		c.JSON(http.StatusInternalServerError, errorResponse("registry.internal_error", err.Error()))
	}
}

func registrationSummary(reg registry.Registration) gin.H {
	return gin.H{
		"capability_id": reg.CapabilityID,
		"tenant_id":     reg.TenantID,
		"version":       reg.Version,
		"status":        reg.Status,
	}
}

func registrationDetail(reg registry.Registration) gin.H {
	adapters := make([]gin.H, 0, len(reg.Adapters))
	for _, adapter := range reg.Adapters {
		ad := gin.H{
			"adapter_id":     adapter.AdapterID,
			"transport_type": adapter.TransportType,
			"endpoint":       adapter.Endpoint,
			"service_ref":    adapter.ServiceRef,
			"weight":         adapter.Weight,
			"timeout_ms":     adapter.TimeoutMS,
			"labels":         adapter.Labels,
		}
		if adapter.MaxConcurrency != nil {
			ad["max_concurrency"] = *adapter.MaxConcurrency
		}
		if adapter.Visibility.Environments.Allow != nil || adapter.Visibility.Environments.Deny != nil ||
			adapter.Visibility.Tenants.Allow != nil || adapter.Visibility.Tenants.Deny != nil {
			ad["visibility"] = adapter.Visibility
		}
		adapters = append(adapters, ad)
	}

	routing := gin.H{
		"strategy":          reg.RoutingPolicy.Strategy,
		"tenant_strategies": reg.RoutingPolicy.TenantStrategies,
		"cooldown_seconds":  reg.RoutingPolicy.CooldownSeconds,
		"fallback_sequence": reg.RoutingPolicy.FallbackSequence,
		"sticky_keys":       reg.RoutingPolicy.StickyKeys,
	}
	if reg.RoutingPolicy.RateLimit != nil {
		routing["rate_limit"] = reg.RoutingPolicy.RateLimit
	}

	response := gin.H{
		"capability_id":        reg.CapabilityID,
		"tenant_id":            reg.TenantID,
		"contract_ref":         reg.ContractRef,
		"status":               reg.Status,
		"version":              reg.Version,
		"environment_policies": reg.EnvironmentPolicies,
		"adapters":             adapters,
		"routing_policy":       routing,
		"tool_grant_ids":       reg.ToolGrantIDs,
		"updated_by":           reg.UpdatedBy,
		"disable_reason":       reg.DisableReason,
	}
	if reg.FallbackPlan != nil {
		response["fallback_plan"] = gin.H{
			"fallback_targets":     reg.FallbackPlan.FallbackTargets,
			"static_response":      reg.FallbackPlan.StaticResponse,
			"trigger_conditions":   reg.FallbackPlan.TriggerConditions,
			"notification_channel": reg.FallbackPlan.NotificationChannel,
		}
	}
	if !reg.UpdatedAt.IsZero() {
		response["updated_at"] = reg.UpdatedAt.Format(time.RFC3339)
	}
	if reg.PublishedAt != nil {
		response["published_at"] = reg.PublishedAt.Format(time.RFC3339)
	}
	if !reg.CreatedAt.IsZero() {
		response["created_at"] = reg.CreatedAt.Format(time.RFC3339)
	}
	return response
}

func errorResponse(code, message string) gin.H {
	return gin.H{
		"code":    code,
		"message": message,
	}
}

func setETag(c *gin.Context, version uint64) {
	c.Header("ETag", "W/\""+strconv.FormatUint(version, 10)+"\"")
}

func parseETag(value string) (uint64, error) {
	s := strings.TrimSpace(value)
	s = strings.TrimPrefix(s, "W/")
	s = strings.Trim(s, "\"")
	return strconv.ParseUint(s, 10, 64)
}

func errorsIs(err error, target error) bool {
	return err != nil && target != nil && errors.Is(err, target)
}

type registrationRequest struct {
	CapabilityID        string                              `json:"capability_id"`
	TenantID            string                              `json:"tenant_id"`
	ContractRef         string                              `json:"contract_ref"`
	Status              string                              `json:"status"`
	EnvironmentPolicies map[string]environmentPolicyRequest `json:"environment_policies"`
	Adapters            []adapterRequest                    `json:"adapters"`
	RoutingPolicy       routingPolicyRequest                `json:"routing_policy"`
	FallbackPlan        *fallbackPlanRequest                `json:"fallback_plan"`
	ToolGrantIDs        []string                            `json:"tool_grant_ids"`
	Version             *uint64                             `json:"version"`
}

type environmentPolicyRequest struct {
	IsEnabled bool              `json:"is_enabled"`
	Overrides map[string]string `json:"overrides"`
}

type adapterRequest struct {
	AdapterID      string             `json:"adapter_id"`
	TransportType  string             `json:"transport_type"`
	EndpointURL    string             `json:"endpoint_url"`
	ServiceRef     string             `json:"service_ref"`
	Weight         int                `json:"weight"`
	TimeoutMS      int                `json:"timeout_ms"`
	MaxConcurrency *int               `json:"max_concurrency"`
	Labels         map[string]string  `json:"labels"`
	Visibility     *visibilityRequest `json:"visibility"`
}

type visibilityRequest struct {
	Environments *visibilityRule `json:"environments"`
	Tenants      *visibilityRule `json:"tenants"`
}

type visibilityRule struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

type routingPolicyRequest struct {
	Strategy         string              `json:"strategy"`
	TenantStrategies map[string]string   `json:"tenant_strategies"`
	RateLimit        *registry.RateLimit `json:"rate_limit"`
	FallbackSequence []string            `json:"fallback_sequence"`
	CooldownSeconds  int                 `json:"cooldown_seconds"`
	StickyKeys       []string            `json:"sticky_keys"`
}

type fallbackPlanRequest struct {
	FallbackTargets     []string                 `json:"fallback_targets"`
	StaticResponse      *registry.StaticResponse `json:"static_response"`
	TriggerConditions   map[string]interface{}   `json:"trigger_conditions"`
	NotificationChannel string                   `json:"notification_channel"`
}

type disableRequest struct {
	Reason  string  `json:"reason"`
	Version *uint64 `json:"version"`
}

func (r registrationRequest) toPayload() registry.RegistrationPayload {
	envPolicies := make(map[string]registry.EnvironmentPolicy, len(r.EnvironmentPolicies))
	for key, value := range r.EnvironmentPolicies {
		envPolicies[key] = registry.EnvironmentPolicy{
			IsEnabled: value.IsEnabled,
			Overrides: value.Overrides,
		}
	}
	adapters := make([]registry.AdapterEndpoint, 0, len(r.Adapters))
	for _, adapter := range r.Adapters {
		vis := registry.VisibilityPolicy{}
		if adapter.Visibility != nil {
			if adapter.Visibility.Environments != nil {
				vis.Environments = registry.VisibilityRule{
					Allow: adapter.Visibility.Environments.Allow,
					Deny:  adapter.Visibility.Environments.Deny,
				}
			}
			if adapter.Visibility.Tenants != nil {
				vis.Tenants = registry.VisibilityRule{
					Allow: adapter.Visibility.Tenants.Allow,
					Deny:  adapter.Visibility.Tenants.Deny,
				}
			}
		}
		adapters = append(adapters, registry.AdapterEndpoint{
			AdapterID:      adapter.AdapterID,
			TransportType:  adapter.TransportType,
			Endpoint:       adapter.EndpointURL,
			ServiceRef:     adapter.ServiceRef,
			Weight:         adapter.Weight,
			TimeoutMS:      adapter.TimeoutMS,
			MaxConcurrency: adapter.MaxConcurrency,
			Labels:         adapter.Labels,
			Visibility:     vis,
		})
	}
	routing := registry.RoutingPolicy{
		Strategy:         r.RoutingPolicy.Strategy,
		TenantStrategies: r.RoutingPolicy.TenantStrategies,
		RateLimit:        r.RoutingPolicy.RateLimit,
		FallbackSequence: r.RoutingPolicy.FallbackSequence,
		CooldownSeconds:  r.RoutingPolicy.CooldownSeconds,
		StickyKeys:       r.RoutingPolicy.StickyKeys,
	}
	var fallback *registry.FallbackPlan
	if r.FallbackPlan != nil {
		fallback = &registry.FallbackPlan{
			FallbackTargets:     r.FallbackPlan.FallbackTargets,
			StaticResponse:      r.FallbackPlan.StaticResponse,
			TriggerConditions:   r.FallbackPlan.TriggerConditions,
			NotificationChannel: r.FallbackPlan.NotificationChannel,
		}
	}
	return registry.RegistrationPayload{
		CapabilityID:        r.CapabilityID,
		TenantID:            r.TenantID,
		ContractRef:         r.ContractRef,
		Status:              r.Status,
		EnvironmentPolicies: envPolicies,
		Adapters:            adapters,
		RoutingPolicy:       routing,
		FallbackPlan:        fallback,
		ToolGrantIDs:        r.ToolGrantIDs,
	}
}
