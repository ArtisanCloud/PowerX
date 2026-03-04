package plugin_release

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/local"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type localInstallHandler struct {
	svc *local.InstallService
}

type startLocalInstallRequest struct {
	TenantUUID   string   `json:"tenant_uuid" binding:"required"`
	DeveloperID  uint64   `json:"developerId" binding:"required"`
	ArtifactURI  string   `json:"artifactUri" binding:"required"`
	FeatureFlags []string `json:"featureFlags"`
	ResetCache   bool     `json:"resetCache"`
}

type localInstallSessionResponse struct {
	SessionID    string   `json:"sessionId"`
	TenantUUID   string   `json:"tenant_uuid"`
	DeveloperID  uint64   `json:"developerId"`
	ArtifactURI  string   `json:"artifactUri"`
	FeatureFlags []string `json:"featureFlags,omitempty"`
	Status       string   `json:"status"`
	LogURL       string   `json:"logUrl,omitempty"`
	CreatedAt    string   `json:"createdAt"`
	ExpiresAt    string   `json:"expiresAt,omitempty"`
}

func newLocalInstallHandler(svc *local.InstallService) *localInstallHandler {
	if svc == nil {
		return nil
	}
	return &localInstallHandler{svc: svc}
}

func (h *localInstallHandler) startSession(c *gin.Context) {
	if h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "local install not available", nil)
		return
	}

	var req startLocalInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	tenantUUID := strings.TrimSpace(req.TenantUUID)
	if tenantUUID == "" {
		dto.ResponseValidationError(c, gin.Error{
			Err:  errTenantUUIDRequired,
			Type: gin.ErrorTypeBind,
		})
		return
	}

	actor := c.GetHeader("Authorization")
	session, err := h.svc.Start(c.Request.Context(), local.StartInput{
		TenantUUID:   tenantUUID,
		DeveloperID:  req.DeveloperID,
		ArtifactURI:  req.ArtifactURI,
		FeatureFlags: req.FeatureFlags,
		ResetCache:   req.ResetCache,
		Actor:        actor,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}

	dto.ResponseSuccessWithStatus(c, http.StatusCreated, h.toResponse(session))
}

func (h *localInstallHandler) getSession(c *gin.Context) {
	if h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "local install not available", nil)
		return
	}

	sessionUUID, err := parseSessionID(c.Param("sessionId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid sessionId", err)
		return
	}

	session, err := h.svc.Get(c.Request.Context(), sessionUUID)
	if err != nil {
		h.writeError(c, err)
		return
	}

	dto.ResponseSuccess(c, h.toResponse(session))
}

func (h *localInstallHandler) stopSession(c *gin.Context) {
	if h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "local install not available", nil)
		return
	}

	sessionUUID, err := parseSessionID(c.Param("sessionId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid sessionId", err)
		return
	}

	force := false
	if v := strings.TrimSpace(c.Query("force")); v != "" {
		force, err = strconv.ParseBool(v)
		if err != nil {
			dto.ResponseError(c, http.StatusBadRequest, "force must be boolean", err)
			return
		}
	}

	tenantUUID, err := resolveTenantUUIDFromRequest(c)
	if err != nil {
		dto.ResponseValidationError(c, gin.Error{
			Err:  err,
			Type: gin.ErrorTypeBind,
		})
		return
	}

	if err := h.svc.Stop(c.Request.Context(), local.StopInput{
		SessionID:  sessionUUID,
		TenantUUID: tenantUUID,
		Force:      force,
		Actor:      c.GetHeader("Authorization"),
	}); err != nil {
		h.writeError(c, err)
		return
	}

	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{"sessionId": sessionUUID.String()})
}

var errTenantUUIDRequired = errors.New("tenant_uuid is required")

func resolveTenantUUIDFromRequest(c *gin.Context) (string, error) {
	tenant := strings.TrimSpace(reqctx.TenantUUIDFromGin(c))
	if tenant != "" {
		canonical, err := reqctx.CanonicalTenantUUID(tenant)
		if err != nil {
			return "", err
		}
		return canonical, nil
	}
	return "", errTenantUUIDRequired
}

func (h *localInstallHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, local.ErrFeatureDisabled):
		dto.ResponseError(c, http.StatusForbidden, "local install disabled", err)
	case errors.Is(err, local.ErrInvalidInput):
		dto.ResponseError(c, http.StatusBadRequest, "invalid local install request", err)
	case errors.Is(err, local.ErrPermissionDenied):
		dto.ResponseError(c, http.StatusForbidden, "insufficient permission for local install", err)
	case errors.Is(err, local.ErrSignatureInvalid):
		dto.ResponseError(c, http.StatusUnprocessableEntity, "artifact signature verification failed", err)
	case errors.Is(err, local.ErrActiveSession):
		dto.ResponseError(c, http.StatusConflict, "local install session already active", err)
	case errors.Is(err, local.ErrArtifactTooLarge):
		dto.ResponseError(c, http.StatusRequestEntityTooLarge, "artifact exceeds configured limit", err)
	case errors.Is(err, local.ErrSessionNotFound):
		dto.ResponseError(c, http.StatusNotFound, "local install session not found", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "local install operation failed", err)
	}
}

func (h *localInstallHandler) toResponse(session *models.LocalInstallSession) localInstallSessionResponse {
	var expiresAt string
	if session.ExpiredAt != nil {
		expiresAt = session.ExpiredAt.UTC().Format(time.RFC3339)
	}

	return localInstallSessionResponse{
		SessionID:   session.UUID.String(),
		TenantUUID:  sessionTenantUUID(session),
		DeveloperID: session.DeveloperID,
		ArtifactURI: session.ArtifactURI,
		FeatureFlags: func() []string {
			flags := local.ExtractFeatureFlags(session.FeatureFlags)
			if flags == nil {
				return []string{}
			}
			return flags
		}(),
		Status:    session.Status,
		LogURL:    extractLogURL(session.LogPointers),
		CreatedAt: session.CreatedAt.UTC().Format(time.RFC3339),
		ExpiresAt: expiresAt,
	}
}

func extractLogURL(raw datatypes.JSON) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if v, ok := payload["log_url"].(string); ok && v != "" {
		return v
	}
	if v, ok := payload["logUrl"].(string); ok && v != "" {
		return v
	}
	return ""
}

func sessionTenantUUID(session *models.LocalInstallSession) string {
	if session == nil {
		return ""
	}
	return strings.TrimSpace(session.TenantUUID)
}

func parseSessionID(value string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(value))
}
