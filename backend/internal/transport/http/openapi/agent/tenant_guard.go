package agent

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) requireServiceTenantAgent(c *gin.Context) (uuid.UUID, string, bool) {
	if c == nil {
		return uuid.Nil, "", false
	}
	claims := reqctx.GetClaims(c.Request.Context())
	if claims == nil || !strings.EqualFold(strings.TrimSpace(claims.Issuer), "powerx-sts") || !audienceContains(claims.Audience, "powerx:api") {
		dto.ResponseError(c, http.StatusForbidden, "agent.service_actor_required", errors.New("sts service actor required"))
		return uuid.Nil, "", false
	}
	tenantUUID, err := reqctx.RequireTenantUUID(c.Request.Context())
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "agent.tenant_required", err)
		return uuid.Nil, "", false
	}
	tenantUUID, err = reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "agent.tenant_invalid", err)
		return uuid.Nil, "", false
	}
	agentID, err := uuid.Parse(strings.TrimSpace(c.Param("agent_id")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "agent.agent_uuid_invalid", err)
		return uuid.Nil, "", false
	}
	if h == nil || h.service == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent.service_unavailable", nil)
		return uuid.Nil, "", false
	}
	if err := h.service.EnsureTenantAccess(c.Request.Context(), agentID, tenantUUID); err != nil {
		h.handleError(c, err)
		return uuid.Nil, "", false
	}
	return agentID, tenantUUID, true
}

func audienceContains(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func serviceActorSubject(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if claims := reqctx.GetClaims(c.Request.Context()); claims != nil {
		return strings.TrimSpace(claims.Subject)
	}
	return strings.TrimSpace(reqctx.GetSubject(c.Request.Context()))
}
