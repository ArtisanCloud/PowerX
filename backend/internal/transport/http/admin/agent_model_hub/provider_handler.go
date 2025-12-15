package agentmodelhub

import (
	"errors"
	"io"
	"net/http"
	"strings"

	appshared "github.com/ArtisanCloud/PowerX/internal/app/shared"
	amhinst "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	amhshared "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	providerregistry "github.com/ArtisanCloud/PowerX/internal/service/provider_registry"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ProviderHandler struct {
	registry *providerregistry.Service
}

func NewProviderHandler(deps *appshared.Deps) *ProviderHandler {
	if deps == nil || deps.DB == nil {
		return &ProviderHandler{}
	}
	artifactOpts := providerregistry.ValidationArtifactOptions{
		MediaManager: deps.MediaMgr,
		Bucket:       "agent",
		Prefix:       "providers",
	}
	registry := providerregistry.NewService(providerregistry.Options{
		Options: amhshared.Options{
			DB:              deps.DB,
			Cache:           cache.NewMemoryCache(),
			AuditSvc:        deps.AuditSvc,
			TenantKeySvc:    buildTenantKeyService(deps.DB),
			Instrumentation: amhinst.NewInstrumentation(nil, nil),
		},
		Artifacts: artifactOpts,
	})
	return &ProviderHandler{registry: registry}
}

func (h *ProviderHandler) registerProvider(c *gin.Context) {
	if h.registry == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "provider registry unavailable", nil)
		return
	}
	tenantUUID, ok := requireTenantUUID(c)
	if !ok {
		return
	}
	var req registerProviderRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = "default"
	}

	profile, err := h.registry.RegisterProvider(
		c.Request.Context(),
		env,
		tenantUUID,
		providerregistry.ProviderProfileInput{
			Name:            req.Name,
			Capabilities:    req.Capabilities,
			PrimaryEndpoint: req.PrimaryEndpoint,
			Regions:         req.Regions,
			TenantWhitelist: dtoToTenantRefs(req.TenantWhitelist),
			Credentials:     req.Credentials,
			RolloutStatus:   "",
			AuditTrailID:    req.AuditTrailID,
		},
	)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{
		"provider": buildProviderDTO(profile),
	})
}

func (h *ProviderHandler) validateProvider(c *gin.Context) {
	if h.registry == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "provider registry unavailable", nil)
		return
	}
	providerID, err := uuid.Parse(c.Param("providerId"))
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "invalid providerId", err)
		return
	}
	suite := c.Query("suite")
	var payload validateProviderBody
	if err := c.ShouldBindJSON(&payload); err != nil {
		if !errors.Is(err, io.EOF) {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "invalid validation payload", err)
			return
		}
	}
	profile, err := h.registry.ValidateProvider(c.Request.Context(), providerID, suite, payload.Report)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, providerregistry.ErrProviderNotFound) {
			status = http.StatusNotFound
		}
		dtoRequest.ResponseError(c, status, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{
		"provider": buildProviderDTO(profile),
	})
}

func (h *ProviderHandler) publishProvider(c *gin.Context) {
	if h.registry == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "provider registry unavailable", nil)
		return
	}
	providerID, err := uuid.Parse(c.Param("providerId"))
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "invalid providerId", err)
		return
	}
	var req publishProviderRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	profile, err := h.registry.PublishProvider(c.Request.Context(), providerID, providerregistry.PublishOptions{
		TenantWhitelist:       dtoToTenantRefs(req.TenantWhitelist),
		RolloutStrategy:       req.RolloutStrategy,
		RollbackTimeoutMinute: req.RollbackTimeoutMinutes,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, providerregistry.ErrProviderNotFound) {
			status = http.StatusNotFound
		}
		dtoRequest.ResponseError(c, status, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"provider": buildProviderDTO(profile),
	})
}

func (h *ProviderHandler) rolloutProvider(c *gin.Context) {
	if h.registry == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "provider registry unavailable", nil)
		return
	}
	providerID, err := uuid.Parse(c.Param("providerId"))
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "invalid providerId", err)
		return
	}
	var req rolloutProviderRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	profile, err := h.registry.ScheduleRollout(c.Request.Context(), providerID, providerregistry.RolloutPlanInput{
		Env:              req.Env,
		Strategy:         req.Strategy,
		Percentage:       req.Percentage,
		Tenants:          dtoToTenantRefs(req.Tenants),
		Note:             req.Note,
		ExpiresInMinutes: req.ExpiresInMinutes,
		RequestedBy:      req.RequestedBy,
	})
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"provider": buildProviderDTO(profile)})
}

func (h *ProviderHandler) rollbackProvider(c *gin.Context) {
	if h.registry == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "provider registry unavailable", nil)
		return
	}
	providerID, err := uuid.Parse(c.Param("providerId"))
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "invalid providerId", err)
		return
	}
	var req rollbackProviderRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	profile, err := h.registry.RollbackProvider(c.Request.Context(), providerID, providerregistry.RollbackInput{
		Env:    req.Env,
		Reason: req.Reason,
	})
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"provider": buildProviderDTO(profile)})
}

type registerProviderRequest struct {
	Env             string            `json:"env"`
	Name            string            `json:"name" binding:"required"`
	Capabilities    []string          `json:"capabilities"`
	PrimaryEndpoint string            `json:"primary_endpoint" binding:"required"`
	Regions         []string          `json:"regions"`
	TenantWhitelist []tenantRefDTO    `json:"tenantWhitelist"`
	Credentials     map[string]string `json:"credentials"`
	AuditTrailID    string            `json:"auditTrailId"`
}

type validateProviderBody struct {
	Report *providerregistry.ValidationReport `json:"report"`
}

type publishProviderRequest struct {
	TenantWhitelist        []tenantRefDTO `json:"tenantWhitelist"`
	RolloutStrategy        string         `json:"rolloutStrategy"`
	RollbackTimeoutMinutes uint32         `json:"rollbackTimeoutMinutes"`
}

type rolloutProviderRequest struct {
	Env              string         `json:"env"`
	Strategy         string         `json:"strategy"`
	Percentage       int            `json:"percentage"`
	Tenants          []tenantRefDTO `json:"tenants" binding:"required"`
	Note             string         `json:"note"`
	ExpiresInMinutes uint32         `json:"expiresInMinutes"`
	RequestedBy      string         `json:"requestedBy"`
}

type rollbackProviderRequest struct {
	Env    string `json:"env"`
	Reason string `json:"reason"`
}

type tenantRefDTO struct {
	TenantUUID  string `json:"tenant_uuid"`
	Environment string `json:"environment"`
}

type providerDTO struct {
	ProviderID      string                 `json:"provider_id"`
	Env             string                 `json:"env"`
	TenantUUID      string                 `json:"tenant_uuid"`
	Name            string                 `json:"name"`
	Capabilities    []string               `json:"capabilities"`
	PrimaryEndpoint string                 `json:"primary_endpoint"`
	Regions         []string               `json:"regions"`
	TenantWhitelist []tenantRefDTO         `json:"tenant_whitelist"`
	RolloutStatus   string                 `json:"rollout_status"`
	HealthScore     float64                `json:"health_score"`
	SecretRefs      map[string]string      `json:"secret_refs"`
	Metadata        map[string]interface{} `json:"metadata"`
}

func buildProviderDTO(profile *model.ProviderProfile) providerDTO {
	return providerDTO{
		ProviderID:      profile.UUID.String(),
		Env:             profile.Env,
		TenantUUID:      profile.TenantUUID,
		Name:            profile.Name,
		Capabilities:    cloneStringSlice([]string(profile.Capabilities)),
		PrimaryEndpoint: profile.PrimaryEndpoint,
		Regions:         cloneStringSlice([]string(profile.Regions)),
		TenantWhitelist: refsToDTO(providerregistry.DecodeTenantWhitelist(profile.TenantWhitelist)),
		RolloutStatus:   profile.RolloutStatus,
		HealthScore:     profile.HealthScore,
		SecretRefs:      convertSecretRefs(profile.SecretRefs),
		Metadata:        convertMetadataMap(profile.Metadata),
	}
}

func dtoToTenantRefs(items []tenantRefDTO) []providerregistry.TenantRef {
	out := make([]providerregistry.TenantRef, 0, len(items))
	for _, it := range items {
		out = append(out, providerregistry.TenantRef{
			TenantUUID:  strings.TrimSpace(it.TenantUUID),
			Environment: strings.TrimSpace(it.Environment),
		})
	}
	return out
}

func refsToDTO(items []providerregistry.TenantRef) []tenantRefDTO {
	out := make([]tenantRefDTO, 0, len(items))
	for _, it := range items {
		out = append(out, tenantRefDTO{
			TenantUUID:  it.TenantUUID,
			Environment: it.Environment,
		})
	}
	return out
}

func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return []string{}
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func convertSecretRefs(src datatypes.JSONMap) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		if str, ok := v.(string); ok && str != "" {
			out[k] = str
		}
	}
	return out
}

func convertMetadataMap(src datatypes.JSONMap) map[string]interface{} {
	if len(src) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func buildTenantKeyService(db *gorm.DB) *tenantkeys.TenantKeyService {
	if db == nil {
		return nil
	}
	return tenantkeys.NewTenantKeyService(db)
}
