package iam

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
)

type handler struct {
	svc *iamsvc.ProvisioningService
}

type provisionRoleRequest struct {
	Code        string `json:"code" validate:"required"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

type listProvisionRolesQuery struct {
	Keyword        string `form:"q"`
	Code           string `form:"code"`
	IncludeBuiltin string `form:"include_builtin"`
	Page           int    `form:"page"`
	PageSize       int    `form:"page_size"`
}

type provisionMemberRequest struct {
	Username         string         `json:"username" validate:"required"`
	Email            string         `json:"email" validate:"required"`
	DisplayName      string         `json:"display_name" validate:"required"`
	AvatarURL        string         `json:"avatar_url"`
	Status           string         `json:"status"`
	InitialPassword  string         `json:"initial_password"`
	RoleCodes        []string       `json:"role_codes"`
	Metadata         map[string]any `json:"metadata"`
	SourceExternalID string         `json:"source_external_id"`
}

func RegisterTenantRoutes(protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if protectedGroup == nil || deps == nil || deps.DB == nil {
		return
	}
	h := &handler{svc: iamsvc.NewProvisioningService(deps.DB, deps.AuditSvc)}
	group := protectedGroup.Group("/tenant/iam")
	{
		group.GET("/roles", h.listProvisionRoles)
		group.POST("/roles/provision", h.provisionRole)
		group.POST("/members/provision", h.provisionMember)
	}
}

func (h *handler) listProvisionRoles(c *gin.Context) {
	if !requireServiceActorSTS(c) {
		return
	}
	var req listProvisionRolesQuery
	if err := c.ShouldBindQuery(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "iam.provision.role_list_failed", err)
		return
	}
	tenantUUID, ok := tenantUUID(c)
	if !ok {
		return
	}
	includeBuiltin := true
	if raw := strings.TrimSpace(req.IncludeBuiltin); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			dto.ResponseError(c, http.StatusBadRequest, "iam.provision.include_builtin_invalid", err)
			return
		}
		includeBuiltin = parsed
	}
	out, err := h.svc.ListProvisionRoles(c.Request.Context(), iamsvc.ListProvisionRolesInput{
		TenantUUID:     tenantUUID,
		Keyword:        req.Keyword,
		Code:           req.Code,
		IncludeBuiltin: includeBuiltin,
		Page:           req.Page,
		PageSize:       req.PageSize,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "iam.provision.role_list_failed", err)
		return
	}
	dto.ResponseList(c, out.Items, &dto.PaginationResponse{
		Total:    out.Total,
		Page:     out.Page,
		PageSize: out.PageSize,
	})
}

func (h *handler) provisionRole(c *gin.Context) {
	if !requireServiceActorSTS(c) {
		return
	}
	var req provisionRoleRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := tenantUUID(c)
	if !ok {
		return
	}
	out, err := h.svc.ProvisionRole(c.Request.Context(), iamsvc.ProvisionRoleInput{
		TenantUUID:   tenantUUID,
		Code:         req.Code,
		Name:         req.Name,
		Description:  req.Description,
		ActorSubject: actorSubject(c),
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "iam.provision.role_failed", err)
		return
	}
	logger.InfoF(c.Request.Context(), "[iam-provision] action=role.create tenant_uuid=%s role_uuid=%s code=%s actor_subject=%s",
		out.TenantUUID, out.RoleUUID, out.Code, actorSubject(c))
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"payload": out})
}

func (h *handler) provisionMember(c *gin.Context) {
	if !requireServiceActorSTS(c) {
		return
	}
	var req provisionMemberRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := tenantUUID(c)
	if !ok {
		return
	}
	out, err := h.svc.ProvisionMember(c.Request.Context(), iamsvc.ProvisionMemberInput{
		TenantUUID:       tenantUUID,
		Username:         req.Username,
		Email:            req.Email,
		DisplayName:      req.DisplayName,
		AvatarURL:        req.AvatarURL,
		Status:           req.Status,
		InitialPassword:  req.InitialPassword,
		RoleCodes:        req.RoleCodes,
		Metadata:         req.Metadata,
		SourceExternalID: req.SourceExternalID,
		ActorSubject:     actorSubject(c),
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "iam.provision.member_failed", err)
		return
	}
	logger.InfoF(c.Request.Context(), "[iam-provision] action=member.create tenant_uuid=%s user_uuid=%s member_uuid=%s username=%s roles=%s actor_subject=%s",
		out.TenantUUID, out.UserUUID, out.MemberUUID, out.Username, strings.Join(out.RoleCodes, ","), actorSubject(c))
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"payload": out})
}

func requireServiceActorSTS(c *gin.Context) bool {
	claims := reqctx.GetClaims(c.Request.Context())
	if claims == nil ||
		!strings.EqualFold(strings.TrimSpace(claims.Issuer), "powerx-sts") ||
		!audienceContains(claims.Audience, "powerx:api") {
		dto.ResponseError(c, http.StatusForbidden, "iam.provision.service_actor_required", errors.New("sts service actor required"))
		return false
	}
	return true
}

func tenantUUID(c *gin.Context) (string, bool) {
	raw, err := reqctx.RequireTenantUUID(c.Request.Context())
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "iam.provision.tenant_required", err)
		return "", false
	}
	tenantUUID, err := reqctx.CanonicalTenantUUID(raw)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "iam.provision.tenant_invalid", err)
		return "", false
	}
	return tenantUUID, true
}

func audienceContains(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func actorSubject(c *gin.Context) string {
	if claims := reqctx.GetClaims(c.Request.Context()); claims != nil {
		return strings.TrimSpace(claims.Subject)
	}
	return strings.TrimSpace(reqctx.GetSubject(c.Request.Context()))
}
