package agent

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentauthz "github.com/ArtisanCloud/PowerX/internal/service/agent_authz"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AgentAuthzHandler struct {
	svc       *agentauthz.Service
	memberSvc *iamsvc.MemberService
}

func NewAgentAuthzHandler(deps *shared.Deps) *AgentAuthzHandler {
	return &AgentAuthzHandler{
		svc:       agentauthz.NewService(deps.DB),
		memberSvc: iamsvc.NewMemberService(deps.DB),
	}
}

type replaceAgentGrantsReq struct {
	Grants []struct {
		CapabilityUUID string `json:"capability_uuid" validate:"required"`
		PermissionCode string `json:"permission_code" validate:"required"`
		Enabled        bool   `json:"enabled"`
	} `json:"grants"`
}

type patchAgentAccessGrantsReq struct {
	Grants []struct {
		SubjectType string `json:"subject_type" validate:"required"`
		SubjectUUID string `json:"subject_uuid" validate:"required"`
		Enabled     bool   `json:"enabled"`
	} `json:"grants"`
}

func (h *AgentAuthzHandler) ListGrantableCapabilities(c *gin.Context) {
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	items, err := h.svc.ListGrantableCapabilities(c.Request.Context(), tenantCtx.UUID())
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "agent.grantable_capabilities_failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"items": items})
}

func (h *AgentAuthzHandler) ListAgentGrants(c *gin.Context) {
	env := c.DefaultQuery("env", "dev")
	tenantCtx, agentUUID, ok := h.agentGrantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.ListAgentGrants(c.Request.Context(), env, tenantCtx.UUID(), agentUUID)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "agent.grants_failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"items": items})
}

func (h *AgentAuthzHandler) ReplaceAgentGrants(c *gin.Context) {
	env := c.DefaultQuery("env", "dev")
	tenantCtx, agentUUID, ok := h.agentGrantContext(c)
	if !ok {
		return
	}
	var req replaceAgentGrantsReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	inputs := make([]agentauthz.AgentGrantInput, 0, len(req.Grants))
	for _, grant := range req.Grants {
		capUUID, err := uuid.Parse(strings.TrimSpace(grant.CapabilityUUID))
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "capability_uuid must be valid", err)
			return
		}
		inputs = append(inputs, agentauthz.AgentGrantInput{
			CapabilityUUID: capUUID,
			PermissionCode: strings.TrimSpace(grant.PermissionCode),
			Enabled:        grant.Enabled,
		})
	}
	items, err := h.svc.ReplaceAgentGrants(c.Request.Context(), env, tenantCtx.UUID(), agentUUID, reqctx.GetUserUUID(c.Request.Context()), inputs)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "agent.replace_grants_failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"items": items})
}

func (h *AgentAuthzHandler) PatchAgentGrants(c *gin.Context) {
	env := c.DefaultQuery("env", "dev")
	tenantCtx, agentUUID, ok := h.agentGrantContext(c)
	if !ok {
		return
	}
	var req replaceAgentGrantsReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	inputs := make([]agentauthz.AgentGrantInput, 0, len(req.Grants))
	for _, grant := range req.Grants {
		capUUID, err := uuid.Parse(strings.TrimSpace(grant.CapabilityUUID))
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "capability_uuid must be valid", err)
			return
		}
		inputs = append(inputs, agentauthz.AgentGrantInput{
			CapabilityUUID: capUUID,
			PermissionCode: strings.TrimSpace(grant.PermissionCode),
			Enabled:        grant.Enabled,
		})
	}
	items, err := h.svc.PatchAgentGrants(c.Request.Context(), env, tenantCtx.UUID(), agentUUID, reqctx.GetUserUUID(c.Request.Context()), inputs)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "agent.patch_grants_failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"items": items})
}

func (h *AgentAuthzHandler) ListAgentAccessGrants(c *gin.Context) {
	env := c.DefaultQuery("env", "dev")
	tenantCtx, agentUUID, ok := h.agentGrantContext(c)
	if !ok {
		return
	}
	items, err := h.svc.ListAgentAccessGrants(c.Request.Context(), env, tenantCtx.UUID(), agentUUID, strings.TrimSpace(c.Query("subject_type")))
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "agent.access_grants_failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"items": items})
}

func (h *AgentAuthzHandler) PatchAgentAccessGrants(c *gin.Context) {
	env := c.DefaultQuery("env", "dev")
	tenantCtx, agentUUID, ok := h.agentGrantContext(c)
	if !ok {
		return
	}
	var req patchAgentAccessGrantsReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	inputs := make([]agentauthz.AgentAccessGrantInput, 0, len(req.Grants))
	for _, grant := range req.Grants {
		inputs = append(inputs, agentauthz.AgentAccessGrantInput{
			SubjectType: strings.TrimSpace(grant.SubjectType),
			SubjectUUID: strings.TrimSpace(grant.SubjectUUID),
			Enabled:     grant.Enabled,
		})
	}
	items, err := h.svc.PatchAgentAccessGrants(c.Request.Context(), env, tenantCtx.UUID(), agentUUID, reqctx.GetUserUUID(c.Request.Context()), inputs)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "agent.patch_access_grants_failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"items": items})
}

func (h *AgentAuthzHandler) MyEffectivePermissions(c *gin.Context) {
	env := c.DefaultQuery("env", "dev")
	tenantCtx, agentUUID, ok := h.agentGrantContext(c)
	if !ok {
		return
	}
	result, err := h.svc.ResolveEffectivePermissions(
		c.Request.Context(),
		env,
		tenantCtx.UUID(),
		reqctx.GetUserUUID(c.Request.Context()),
		reqctx.GetMemberUUID(c.Request.Context()),
		reqctx.GetMemberID(c.Request.Context()),
		reqctx.IsRoot(c.Request.Context()),
		agentUUID,
	)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "agent.effective_permissions_failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, result)
}

func (h *AgentAuthzHandler) EffectivePermissions(c *gin.Context) {
	env := c.DefaultQuery("env", "dev")
	tenantCtx, agentUUID, ok := h.agentGrantContext(c)
	if !ok {
		return
	}
	memberUUID := strings.TrimSpace(c.Query("member_uuid"))
	if _, err := uuid.Parse(memberUUID); err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "member_uuid must be a valid UUID", err)
		return
	}
	member, err := h.memberSvc.GetMemberByUUID(c.Request.Context(), tenantCtx.UUID(), memberUUID)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusNotFound, "member not found", err)
		return
	}
	if member == nil || member.Member == nil || member.User == nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "member profile incomplete", nil)
		return
	}
	result, err := h.svc.ResolveEffectivePermissions(
		c.Request.Context(),
		env,
		tenantCtx.UUID(),
		member.User.UUID.String(),
		member.Member.UUID.String(),
		member.Member.ID,
		member.User.IsRoot,
		agentUUID,
	)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "agent.effective_permissions_failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, result)
}

func (h *AgentAuthzHandler) agentGrantContext(c *gin.Context) (*tenantContext, uuid.UUID, bool) {
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return nil, uuid.Nil, false
	}
	agentUUID, err := parseAgentUUIDParam(c)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "agent_uuid must be valid", err)
		return nil, uuid.Nil, false
	}
	return tenantCtx, agentUUID, true
}
