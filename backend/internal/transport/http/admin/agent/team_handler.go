package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type TeamHandler struct {
	svc *agentSvc.TeamService
}

func NewTeamHandler(dep *shared.Deps) *TeamHandler {
	if dep == nil {
		return &TeamHandler{}
	}
	return &TeamHandler{svc: agentSvc.NewTeamService(dep.DB)}
}

type createTeamRequest struct {
	ParentAgentID        uint64          `json:"parent_agent_id" binding:"required"`
	TeamKey              string          `json:"team_key" binding:"required"`
	DisplayNameI18n      json.RawMessage `json:"display_name_i18n" binding:"required"`
	DispatchMode         string          `json:"dispatch_mode"`
	DefaultFailurePolicy string          `json:"default_failure_policy"`
	OrchestrationSpec    json.RawMessage `json:"orchestration_spec"`
}

type updateTeamRequest struct {
	ParentAgentID        *uint64         `json:"parent_agent_id"`
	TeamKey              *string         `json:"team_key"`
	DisplayNameI18n      json.RawMessage `json:"display_name_i18n"`
	DispatchMode         *string         `json:"dispatch_mode"`
	DefaultFailurePolicy *string         `json:"default_failure_policy"`
	OrchestrationSpec    json.RawMessage `json:"orchestration_spec"`
}

type upsertTeamMemberRequest struct {
	ChildAgentID uint64 `json:"child_agent_id" binding:"required"`
	Role         string `json:"role"`
	Priority     int    `json:"priority"`
	Enabled      *bool  `json:"enabled"`
}

func (h *TeamHandler) CreateTeam(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent team service unavailable", nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "tenant context required", err)
		return
	}
	var req createTeamRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	created, err := h.svc.CreateTeam(c.Request.Context(), agentSvc.TeamCreateInput{
		TenantUUID:           tenantCtx.UUID(),
		ParentAgentID:        req.ParentAgentID,
		TeamKey:              req.TeamKey,
		DisplayNameI18n:      req.DisplayNameI18n,
		DispatchMode:         req.DispatchMode,
		DefaultFailurePolicy: req.DefaultFailurePolicy,
		OrchestrationSpec:    req.OrchestrationSpec,
		CreatedBy:            "admin",
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "create team failed", err)
		return
	}
	dto.ResponseSuccess(c, created)
}

func (h *TeamHandler) ListTeams(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent team service unavailable", nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "tenant context required", err)
		return
	}
	parentAgentID, _ := strconv.ParseUint(strings.TrimSpace(c.Query("parent_agent_id")), 10, 64)
	includeDisabled := strings.EqualFold(strings.TrimSpace(c.Query("include_disabled")), "true")
	if parentAgentID > 0 {
		rows, listErr := h.svc.ListByParent(c.Request.Context(), tenantCtx.UUID(), parentAgentID, includeDisabled)
		if listErr != nil {
			dto.ResponseError(c, http.StatusInternalServerError, "list teams failed", listErr)
			return
		}
		dto.ResponseSuccess(c, gin.H{"items": rows, "total": len(rows)})
		return
	}
	rows, listErr := h.svc.ListByTenant(c.Request.Context(), tenantCtx.UUID(), includeDisabled)
	if listErr != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "list teams failed", listErr)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": rows, "total": len(rows)})
}

func (h *TeamHandler) SetTeamStatus(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent team service unavailable", nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "tenant context required", err)
		return
	}
	teamID, _ := strconv.ParseUint(strings.TrimSpace(c.Param("teamId")), 10, 64)
	if teamID == 0 {
		dto.ResponseError(c, http.StatusBadRequest, "invalid team_id", nil)
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err = c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	if err = h.svc.SetTeamStatus(c.Request.Context(), teamID, tenantCtx.UUID(), req.Status); err != nil {
		switch {
		case errors.Is(err, agentSvc.ErrTeamNotFound):
			dto.ResponseError(c, http.StatusNotFound, "team not found", err)
		case errors.Is(err, agentSvc.ErrTeamInvalidTenant):
			dto.ResponseError(c, http.StatusForbidden, "team tenant mismatch", err)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "update team status failed", err)
		}
		return
	}
	dto.ResponseSuccess(c, gin.H{"team_id": teamID, "status": strings.ToLower(strings.TrimSpace(req.Status))})
}

func (h *TeamHandler) UpdateTeam(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent team service unavailable", nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "tenant context required", err)
		return
	}
	teamID, _ := strconv.ParseUint(strings.TrimSpace(c.Param("teamId")), 10, 64)
	if teamID == 0 {
		dto.ResponseError(c, http.StatusBadRequest, "invalid team_id", nil)
		return
	}
	var req updateTeamRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	if req.ParentAgentID == nil && req.TeamKey == nil && req.DisplayNameI18n == nil && req.DispatchMode == nil && req.DefaultFailurePolicy == nil && req.OrchestrationSpec == nil {
		dto.ResponseError(c, http.StatusBadRequest, "empty update payload", nil)
		return
	}

	in := agentSvc.TeamUpdateInput{
		TeamID:     teamID,
		TenantUUID: tenantCtx.UUID(),
	}
	if req.ParentAgentID != nil {
		in.ParentAgentID = *req.ParentAgentID
	}
	if req.TeamKey != nil {
		in.TeamKey = *req.TeamKey
	}
	if req.DisplayNameI18n != nil {
		in.DisplayNameI18n = req.DisplayNameI18n
		in.UpdateDisplayNameI18n = true
	}
	if req.DispatchMode != nil {
		in.DispatchMode = *req.DispatchMode
	}
	if req.DefaultFailurePolicy != nil {
		in.DefaultFailurePolicy = *req.DefaultFailurePolicy
	}
	if req.OrchestrationSpec != nil {
		in.OrchestrationSpec = req.OrchestrationSpec
		in.UpdateOrchestration = true
	}

	updated, updateErr := h.svc.UpdateTeam(c.Request.Context(), in)
	if updateErr != nil {
		switch {
		case errors.Is(updateErr, agentSvc.ErrTeamNotFound):
			dto.ResponseError(c, http.StatusNotFound, "team not found", updateErr)
		case errors.Is(updateErr, agentSvc.ErrTeamInvalidTenant):
			dto.ResponseError(c, http.StatusForbidden, "team tenant mismatch", updateErr)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "update team failed", updateErr)
		}
		return
	}
	dto.ResponseSuccess(c, updated)
}

func (h *TeamHandler) UpsertTeamMember(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent team service unavailable", nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "tenant context required", err)
		return
	}
	teamID, _ := strconv.ParseUint(strings.TrimSpace(c.Param("teamId")), 10, 64)
	if teamID == 0 {
		dto.ResponseError(c, http.StatusBadRequest, "invalid team_id", nil)
		return
	}
	var req upsertTeamMemberRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	priority := req.Priority
	if priority <= 0 {
		priority = 1
	}
	item, err := h.svc.UpsertMember(c.Request.Context(), agentSvc.TeamMemberUpsertInput{
		TeamID:       teamID,
		TenantUUID:   tenantCtx.UUID(),
		ChildAgentID: req.ChildAgentID,
		Role:         req.Role,
		Priority:     priority,
		Enabled:      enabled,
	})
	if err != nil {
		if errors.Is(err, agentSvc.ErrTeamMemberRole) {
			dto.ResponseError(c, http.StatusBadRequest, "role is not allowed for child agent", err)
			return
		}
		if errors.Is(err, agentSvc.ErrTeamMemberAgent) {
			dto.ResponseError(c, http.StatusBadRequest, "child agent must be tenant-owned active non-system agent", err)
			return
		}
		dto.ResponseError(c, http.StatusInternalServerError, "upsert team member failed", err)
		return
	}
	dto.ResponseSuccess(c, item)
}

func (h *TeamHandler) ListTeamMembers(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent team service unavailable", nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "tenant context required", err)
		return
	}
	teamID, _ := strconv.ParseUint(strings.TrimSpace(c.Param("teamId")), 10, 64)
	if teamID == 0 {
		dto.ResponseError(c, http.StatusBadRequest, "invalid team_id", nil)
		return
	}
	rows, err := h.svc.ListMembers(c.Request.Context(), teamID, tenantCtx.UUID())
	if err != nil {
		switch {
		case errors.Is(err, agentSvc.ErrTeamNotFound):
			dto.ResponseError(c, http.StatusNotFound, "team not found", err)
		case errors.Is(err, agentSvc.ErrTeamInvalidTenant):
			dto.ResponseError(c, http.StatusForbidden, "team tenant mismatch", err)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "list team members failed", err)
		}
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": rows, "total": len(rows)})
}

func (h *TeamHandler) DeleteTeamMember(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent team service unavailable", nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "tenant context required", err)
		return
	}
	teamID, _ := strconv.ParseUint(strings.TrimSpace(c.Param("teamId")), 10, 64)
	childID, _ := strconv.ParseUint(strings.TrimSpace(c.Param("childAgentId")), 10, 64)
	if teamID == 0 || childID == 0 {
		dto.ResponseError(c, http.StatusBadRequest, "invalid team/member id", nil)
		return
	}
	if err = h.svc.RemoveMember(c.Request.Context(), teamID, tenantCtx.UUID(), childID); err != nil {
		switch {
		case errors.Is(err, agentSvc.ErrTeamNotFound):
			dto.ResponseError(c, http.StatusNotFound, "team not found", err)
		case errors.Is(err, agentSvc.ErrTeamInvalidTenant):
			dto.ResponseError(c, http.StatusForbidden, "team tenant mismatch", err)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "delete team member failed", err)
		}
		return
	}
	dto.ResponseSuccess(c, gin.H{"team_id": teamID, "child_agent_id": childID, "deleted": true})
}

func (h *TeamHandler) DeleteTeam(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "agent team service unavailable", nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "tenant context required", err)
		return
	}
	teamID, _ := strconv.ParseUint(strings.TrimSpace(c.Param("teamId")), 10, 64)
	if teamID == 0 {
		dto.ResponseError(c, http.StatusBadRequest, "invalid team_id", nil)
		return
	}
	deleteErr := h.svc.DeleteTeam(c.Request.Context(), teamID, tenantCtx.UUID())
	if deleteErr != nil {
		switch {
		case errors.Is(deleteErr, agentSvc.ErrTeamNotFound):
			dto.ResponseError(c, http.StatusNotFound, "team not found", deleteErr)
		case errors.Is(deleteErr, agentSvc.ErrTeamInvalidTenant):
			dto.ResponseError(c, http.StatusForbidden, "team tenant mismatch", deleteErr)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "delete team failed", deleteErr)
		}
		return
	}
	dto.ResponseSuccess(c, gin.H{"team_id": teamID, "deleted": true})
}
