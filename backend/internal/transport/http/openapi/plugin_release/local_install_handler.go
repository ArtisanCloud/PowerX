package plugin_release

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/local"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type localInstallHandler struct {
	svc    *local.InstallService
	access *hostContractAccess
}

type startLocalInstallRequest struct {
	TenantUUID   string   `json:"tenant_uuid"`
	ArtifactURI  string   `json:"artifact_uri" binding:"required"`
	FeatureFlags []string `json:"feature_flags"`
	ResetCache   bool     `json:"reset_cache"`
}

type localInstallSessionResponse struct {
	SessionUUID  string   `json:"session_uuid"`
	TenantUUID   string   `json:"tenant_uuid"`
	PluginID     string   `json:"plugin_id"`
	ServiceActor string   `json:"service_actor"`
	ArtifactURI  string   `json:"artifact_uri"`
	FeatureFlags []string `json:"feature_flags,omitempty"`
	Status       string   `json:"status"`
	LogURL       string   `json:"log_url,omitempty"`
	CreatedAt    string   `json:"created_at"`
	ExpiresAt    string   `json:"expires_at,omitempty"`
}

func newLocalInstallHandler(svc *local.InstallService, db *gorm.DB) *localInstallHandler {
	if svc == nil || db == nil {
		return nil
	}
	return &localInstallHandler{svc: svc, access: newHostContractAccess(db)}
}

func (h *localInstallHandler) startSession(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, pluginReleaseUpstream(errors.New("local install not available")))
		return
	}

	var req startLocalInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondErrorFrom(c, pluginReleaseInvalidArgument(err))
		return
	}

	if strings.TrimSpace(req.TenantUUID) != "" {
		dto.RespondErrorFrom(c, pluginReleaseInvalidArgument(errors.New("tenant_uuid must not be supplied")))
		return
	}
	if !rejectTenantOverride(c) {
		return
	}
	tenantUUID, err := h.access.authorize(c.Request.Context(), hostAPIKeyHash(c), pluginReleaseSessionsManageCapabilityID, pluginReleaseSessionsManageAPIKeyScope, "sessions", "manage")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	claims := reqctx.GetClaims(c.Request.Context())
	if claims == nil || strings.TrimSpace(claims.PluginID) == "" || strings.TrimSpace(claims.Subject) == "" {
		dto.RespondErrorFrom(c, pluginReleaseUnauthorized(errors.New("plugin service actor is required")))
		return
	}

	actor := c.GetHeader("Authorization")
	session, err := h.svc.Start(c.Request.Context(), local.StartInput{
		TenantUUID:   tenantUUID,
		PluginID:     strings.TrimSpace(claims.PluginID),
		ServiceActor: strings.TrimSpace(claims.Subject),
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
		dto.RespondErrorFrom(c, pluginReleaseUpstream(errors.New("local install not available")))
		return
	}

	if !rejectTenantOverride(c) {
		return
	}
	sessionUUID, err := parseSessionID(c.Param("session_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, pluginReleaseInvalidArgument(err))
		return
	}

	tenantUUID, err := h.access.authorize(c.Request.Context(), hostAPIKeyHash(c), pluginReleaseSessionsReadCapabilityID, pluginReleaseSessionsReadAPIKeyScope, "sessions", "read")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	session, err := h.svc.Get(c.Request.Context(), tenantUUID, sessionUUID)
	if err != nil {
		h.writeError(c, err)
		return
	}

	dto.ResponseSuccess(c, h.toResponse(session))
}

func (h *localInstallHandler) stopSession(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, pluginReleaseUpstream(errors.New("local install not available")))
		return
	}

	if !rejectTenantOverride(c) {
		return
	}
	sessionUUID, err := parseSessionID(c.Param("session_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, pluginReleaseInvalidArgument(err))
		return
	}

	var body struct {
		Force bool `json:"force"`
	}
	if c.Request.ContentLength > 0 && c.ShouldBindJSON(&body) != nil {
		dto.RespondErrorFrom(c, pluginReleaseInvalidArgument(errors.New("invalid stop request")))
		return
	}
	tenantUUID, err := h.access.authorize(c.Request.Context(), hostAPIKeyHash(c), pluginReleaseSessionsManageCapabilityID, pluginReleaseSessionsManageAPIKeyScope, "sessions", "manage")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}

	if err := h.svc.Stop(c.Request.Context(), local.StopInput{
		SessionID:  sessionUUID,
		TenantUUID: tenantUUID,
		Force:      body.Force,
		Actor:      c.GetHeader("Authorization"),
	}); err != nil {
		h.writeError(c, err)
		return
	}

	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{"session_uuid": sessionUUID.String()})
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
		dto.RespondErrorFrom(c, pluginReleaseForbidden(err))
	case errors.Is(err, local.ErrInvalidInput):
		dto.RespondErrorFrom(c, pluginReleaseInvalidArgument(err))
	case errors.Is(err, local.ErrPermissionDenied):
		dto.RespondErrorFrom(c, pluginReleaseForbidden(err))
	case errors.Is(err, local.ErrSignatureInvalid):
		dto.RespondErrorFrom(c, pluginReleasePackageVerificationFailed(err))
	case errors.Is(err, local.ErrActiveSession):
		dto.RespondErrorFrom(c, pluginReleaseActiveSessionConflict(err))
	case errors.Is(err, local.ErrArtifactTooLarge):
		dto.RespondErrorFrom(c, pluginReleaseInvalidArgument(err))
	case errors.Is(err, local.ErrSessionNotFound):
		dto.RespondErrorFrom(c, pluginReleaseSessionNotFound(err))
	default:
		dto.RespondErrorFrom(c, pluginReleaseUpstream(err))
	}
}

func (h *localInstallHandler) toResponse(session *models.LocalInstallSession) localInstallSessionResponse {
	var expiresAt string
	if session.ExpiredAt != nil {
		expiresAt = session.ExpiredAt.UTC().Format(time.RFC3339)
	}

	return localInstallSessionResponse{
		SessionUUID:  session.UUID.String(),
		TenantUUID:   sessionTenantUUID(session),
		PluginID:     session.PluginID,
		ServiceActor: session.ServiceActor,
		ArtifactURI:  session.ArtifactURI,
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

func currentUserMemberUUID(c *gin.Context) (string, error) {
	memberUUID := strings.TrimSpace(reqctx.GetMemberUUID(c.Request.Context()))
	if _, err := uuid.Parse(memberUUID); err != nil {
		return "", err
	}
	return memberUUID, nil
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
