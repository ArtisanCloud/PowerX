package agentmodelhub

import (
	"encoding/json"
	"net/http"
	"strings"

	appshared "github.com/ArtisanCloud/PowerX/internal/app/shared"
	amhinst "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	amhshared "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	connectorguard "github.com/ArtisanCloud/PowerX/internal/service/connector_guard"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type ConnectorHandler struct {
	svc *connectorguard.Service
}

func NewConnectorHandler(deps *appshared.Deps) *ConnectorHandler {
	if deps == nil || deps.DB == nil {
		return &ConnectorHandler{}
	}
	svc := connectorguard.NewService(connectorguard.Options{
		Options: amhshared.Options{
			DB:              deps.DB,
			Cache:           cache.NewMemoryCache(),
			AuditSvc:        deps.AuditSvc,
			TenantKeySvc:    buildTenantKeyService(deps.DB),
			Instrumentation: amhinst.NewInstrumentation(nil, nil),
		},
	})
	return &ConnectorHandler{svc: svc}
}

type connectorInstanceRequest struct {
	Env                  string            `json:"env"`
	TenantID             string            `json:"tenantId" binding:"required"`
	Region               string            `json:"region"`
	OAuthRef             string            `json:"oauthRef"`
	WebhookSigningKeyRef string            `json:"webhookSigningKeyRef"`
	MappingTemplate      map[string]any    `json:"mappingTemplate"`
	RateLimitPerMinute   uint32            `json:"rateLimitPerMinute"`
	Status               string            `json:"status"`
	Secrets              map[string]string `json:"secrets"`
	InstanceID           string            `json:"instanceId"`
}

func (h *ConnectorHandler) upsertInstance(c *gin.Context) {
	if h.svc == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "connector guard service unavailable", nil)
		return
	}
	var req connectorInstanceRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	platform := strings.TrimSpace(c.Param("platform"))
	if platform == "" {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "platform required", nil)
		return
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = "default"
	}

	mappingJSON := datatypes.JSON([]byte("{}"))
	if req.MappingTemplate != nil {
		buf, err := json.Marshal(req.MappingTemplate)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "mappingTemplate must be serializable object", err)
			return
		}
		mappingJSON = datatypes.JSON(buf)
	}

	input := connectorguard.ConnectorInstanceInput{
		TenantScope:          req.TenantID,
		Platform:             platform,
		Region:               req.Region,
		OAuthRef:             req.OAuthRef,
		WebhookSigningKeyRef: req.WebhookSigningKeyRef,
		MappingTemplate:      mappingJSON,
		RateLimitPerMinute:   req.RateLimitPerMinute,
		Status:               req.Status,
		Secrets:              req.Secrets,
		InstanceID:           req.InstanceID,
	}

	instance, err := h.svc.UpsertInstance(c.Request.Context(), env, input)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	var mapping map[string]any
	if len(instance.MappingTemplate) > 0 {
		_ = json.Unmarshal(instance.MappingTemplate, &mapping)
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"instance": gin.H{
			"instance_id":        instance.UUID.String(),
			"platform":           instance.Platform,
			"tenantScope":        instance.TenantScope,
			"status":             instance.Status,
			"rateLimitPerMinute": instance.RateLimitPerMinute,
			"mappingTemplate":    mapping,
			"region":             instance.Region,
			"errorRate":          instance.ErrorRate,
			"lastPauseReason":    instance.LastPauseReason,
		},
	})
}

func (h *ConnectorHandler) pauseInstance(c *gin.Context) {
	if h.svc == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "connector guard service unavailable", nil)
		return
	}
	instanceParam := strings.TrimSpace(c.Param("instanceId"))
	if instanceParam == "" {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "instanceId required", nil)
		return
	}
	instanceID, err := uuid.Parse(instanceParam)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "invalid instanceId", err)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := h.svc.PauseInstance(c.Request.Context(), instanceID, strings.TrimSpace(body.Reason), ""); err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}
