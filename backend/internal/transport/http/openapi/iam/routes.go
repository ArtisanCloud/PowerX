package iam

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type handler struct {
	svc             *iamsvc.ProvisioningService
	members         *iamsvc.MemberService
	directoryAccess *iamsvc.DirectoryAccessService
	directory       *iamsvc.TenantDirectoryService
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

type batchGetMembersRequest struct {
	MemberUUIDs []string `json:"member_uuids" validate:"required,min=1"`
}

type memberResponse struct {
	MemberUUID      string   `json:"member_uuid"`
	TenantUUID      string   `json:"tenant_uuid"`
	UserUUID        string   `json:"user_uuid"`
	DisplayName     string   `json:"display_name,omitempty"`
	DepartmentUUIDs []string `json:"department_uuids"`
	Status          int16    `json:"status"`
}

type memberResolveResponse struct {
	MemberUUID  string `json:"member_uuid"`
	UserUUID    string `json:"user_uuid"`
	DisplayName string `json:"display_name,omitempty"`
}

type memberDirectoryPageQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Tenant   string `form:"tenant_uuid"`
}

type authorizationCheckRequest struct {
	MemberUUID string `json:"member_uuid" validate:"required"`
	UserUUID   string `json:"user_uuid" validate:"required"`
	Resource   string `json:"resource" validate:"required"`
	Action     string `json:"action" validate:"required"`
	TraceID    string `json:"trace_id"`
}

func RegisterTenantRoutes(protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	if protectedGroup == nil || deps == nil || deps.DB == nil {
		return
	}
	h := &handler{svc: iamsvc.NewProvisioningService(deps.DB, deps.AuditSvc), members: iamsvc.NewMemberService(deps.DB), directoryAccess: iamsvc.NewDirectoryAccessService(deps.DB), directory: iamsvc.NewTenantDirectoryService(deps.DB)}
	group := protectedGroup.Group("/tenant/iam")
	{
		group.GET("/tenant", h.getTenant)
		group.GET("/members", h.listMembers)
		group.GET("/departments", h.listDepartments)
		group.GET("/roles", h.listDirectoryRoles)
		group.GET("/permissions", h.listPermissions)
		group.POST("/authorization:check", h.checkAuthorization)
		group.GET("/roles/provisionable", h.listProvisionRoles)
		group.POST("/roles/provision", h.provisionRole)
		group.POST("/members/provision", h.provisionMember)
		group.GET("/members/:member_uuid", h.getMember)
		// Gin treats ':' as a path-parameter delimiter. Keep the two published
		// colon-action URLs behind one constrained dispatcher; it rejects every
		// operation other than the declared contract actions.
		group.POST("/members:operation", h.batchMemberOperation)
	}
}

func (h *handler) getTenant(c *gin.Context) {
	tenantUUID, err := h.authorizeDirectoryRead(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	tenant, err := h.directory.GetTenant(c.Request.Context(), tenantUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
		return
	}
	if err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
		return
	}
	dto.ResponseSuccess(c, tenant)
}

func (h *handler) listMembers(c *gin.Context) {
	var query memberDirectoryPageQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(err))
		return
	}
	if strings.TrimSpace(query.Tenant) != "" {
		dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(errors.New("tenant_uuid must not be supplied")))
		return
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 200 {
		dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(errors.New("invalid pagination")))
		return
	}
	tenantUUID, err := h.authorizeDirectoryRead(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	base := h.members.DB.WithContext(c.Request.Context()).Model(&modeliam.Member{}).Where("tenant_uuid = ? AND username <> ?", tenantUUID, iamsvc.ROOT_USERNAME)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
		return
	}
	var members []modeliam.Member
	if err := base.Order("uuid ASC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&members).Error; err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
		return
	}
	items := make([]memberResponse, 0, len(members))
	for i := range members {
		response, err := h.memberResponse(c, tenantUUID, &iamsvc.MemberWithProfile{Member: &members[i]})
		if err != nil {
			dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
			return
		}
		items = append(items, response)
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "pagination": gin.H{"page": query.Page, "page_size": query.PageSize, "total": total}})
}

func (h *handler) batchMemberOperation(c *gin.Context) {
	// Gin retains the separator in a parameter embedded in a path segment, so
	// normalize only that routing artifact before comparing against the fixed
	// operation allow-list.
	switch strings.TrimPrefix(c.Param("operation"), ":") {
	case "batch-get":
		h.batchGetMembers(c)
	case "batch-resolve":
		h.batchResolveMembers(c)
	default:
		c.Status(http.StatusNotFound)
	}
}

func (h *handler) getMember(c *gin.Context) {
	tenantUUID, err := h.authorizeMembersRead(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	memberUUID := strings.TrimSpace(c.Param("member_uuid"))
	if _, err := uuid.Parse(memberUUID); err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(err))
		return
	}
	member, err := h.members.GetMemberByUUID(c.Request.Context(), tenantUUID, memberUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		dto.RespondErrorFrom(c, iamsvc.DirectoryMemberNotFoundError(err))
		return
	}
	if err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
		return
	}
	response, err := h.memberResponse(c, tenantUUID, member)
	if err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
		return
	}
	dto.ResponseSuccess(c, response)
}

func (h *handler) batchGetMembers(c *gin.Context) {
	var req batchGetMembersRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(err))
		return
	}
	tenantUUID, err := h.authorizeMembersRead(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	seen := make(map[string]struct{}, len(req.MemberUUIDs))
	memberUUIDs := make([]string, 0, len(req.MemberUUIDs))
	for _, raw := range req.MemberUUIDs {
		memberUUID := strings.TrimSpace(raw)
		if _, err := uuid.Parse(memberUUID); err != nil {
			dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(err))
			return
		}
		if _, exists := seen[memberUUID]; exists {
			dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(errors.New("duplicate member_uuid")))
			return
		}
		seen[memberUUID] = struct{}{}
		memberUUIDs = append(memberUUIDs, memberUUID)
	}

	result := make([]memberResponse, 0, len(memberUUIDs))
	for _, memberUUID := range memberUUIDs {
		member, err := h.members.GetMemberByUUID(c.Request.Context(), tenantUUID, memberUUID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dto.RespondErrorFrom(c, iamsvc.DirectoryMemberNotFoundError(err))
			return
		}
		if err != nil {
			dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
			return
		}
		response, err := h.memberResponse(c, tenantUUID, member)
		if err != nil {
			dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
			return
		}
		result = append(result, response)
	}
	dto.ResponseSuccess(c, gin.H{"items": result})
}

// batchResolveMembers is intentionally distinct from batchGetMembers: historical
// audit records may reference members that were deleted or belong to another
// tenant, so missing entries are returned explicitly instead of failing the
// complete request. Invalid or duplicate input remains a contract failure.
func (h *handler) batchResolveMembers(c *gin.Context) {
	var req batchGetMembersRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(err))
		return
	}
	tenantUUID, err := h.authorizeDirectoryRead(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	seen := make(map[string]struct{}, len(req.MemberUUIDs))
	memberUUIDs := make([]string, 0, len(req.MemberUUIDs))
	for _, raw := range req.MemberUUIDs {
		memberUUID := strings.TrimSpace(raw)
		if _, err := uuid.Parse(memberUUID); err != nil {
			dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(err))
			return
		}
		if _, exists := seen[memberUUID]; exists {
			dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(errors.New("duplicate member_uuid")))
			return
		}
		seen[memberUUID] = struct{}{}
		memberUUIDs = append(memberUUIDs, memberUUID)
	}
	items := make([]memberResolveResponse, 0, len(memberUUIDs))
	missing := make([]string, 0)
	for _, memberUUID := range memberUUIDs {
		member, err := h.members.GetMemberByUUID(c.Request.Context(), tenantUUID, memberUUID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			missing = append(missing, memberUUID)
			continue
		}
		if err != nil {
			dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
			return
		}
		profile := memberResponseFromProfile(member)
		items = append(items, memberResolveResponse{MemberUUID: profile.MemberUUID, UserUUID: profile.UserUUID, DisplayName: profile.DisplayName})
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "missing_member_uuids": missing})
}

func (h *handler) authorizeMembersRead(c *gin.Context) (string, error) {
	if raw, ok := c.Get("auth_api_key_hash"); ok {
		if apiKeyHash, ok := raw.(string); ok && strings.TrimSpace(apiKeyHash) != "" {
			return h.directoryAccess.AuthorizeMembersReadAPIKey(c.Request.Context(), apiKeyHash)
		}
	}
	return h.directoryAccess.AuthorizeMembersRead(c.Request.Context())
}

func (h *handler) authorizeDirectoryRead(c *gin.Context) (string, error) {
	if raw, ok := c.Get("auth_api_key_hash"); ok {
		if apiKeyHash, ok := raw.(string); ok && strings.TrimSpace(apiKeyHash) != "" {
			return h.directoryAccess.AuthorizeDirectoryReadAPIKey(c.Request.Context(), apiKeyHash)
		}
	}
	return h.directoryAccess.AuthorizeDirectoryRead(c.Request.Context())
}

func (h *handler) authorizeAuthorizationCheck(c *gin.Context) (string, error) {
	if raw, ok := c.Get("auth_api_key_hash"); ok {
		if apiKeyHash, ok := raw.(string); ok && strings.TrimSpace(apiKeyHash) != "" {
			return h.directoryAccess.AuthorizeAuthorizationCheckAPIKey(c.Request.Context(), apiKeyHash)
		}
	}
	return h.directoryAccess.AuthorizeAuthorizationCheck(c.Request.Context())
}

func (h *handler) memberResponse(c *gin.Context, tenantUUID string, profile *iamsvc.MemberWithProfile) (memberResponse, error) {
	response := memberResponseFromProfile(profile)
	if profile == nil || profile.Member == nil {
		return response, nil
	}
	departmentUUIDs, err := h.directory.MemberDepartmentUUIDs(c.Request.Context(), tenantUUID, profile.Member.ID)
	if err != nil {
		return memberResponse{}, err
	}
	response.DepartmentUUIDs = departmentUUIDs
	return response, nil
}

func memberResponseFromProfile(profile *iamsvc.MemberWithProfile) memberResponse {
	if profile == nil || profile.Member == nil {
		return memberResponse{}
	}
	response := memberResponse{
		MemberUUID:  profile.Member.UUID.String(),
		TenantUUID:  strings.TrimSpace(profile.Member.TenantUUID),
		UserUUID:    strings.TrimSpace(profile.Member.UserUUID),
		DisplayName: strings.TrimSpace(profile.Member.DisplayName),
		Status:      profile.Member.Status,
	}
	if response.DisplayName == "" && profile.User != nil {
		response.DisplayName = strings.TrimSpace(profile.User.DisplayName)
	}
	return response
}

func (h *handler) listDepartments(c *gin.Context) {
	tenantUUID, err := h.authorizeDirectoryRead(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	items, err := h.directory.ListDepartments(c.Request.Context(), tenantUUID)
	if err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}

func (h *handler) listDirectoryRoles(c *gin.Context) {
	tenantUUID, err := h.authorizeDirectoryRead(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	items, err := h.directory.ListRoles(c.Request.Context(), tenantUUID)
	if err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}

func (h *handler) listPermissions(c *gin.Context) {
	if _, err := h.authorizeDirectoryRead(c); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	items, err := h.directory.ListPermissions(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}

func (h *handler) checkAuthorization(c *gin.Context) {
	var req authorizationCheckRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(err))
		return
	}
	tenantUUID, err := h.authorizeAuthorizationCheck(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	if _, err := uuid.Parse(strings.TrimSpace(req.MemberUUID)); err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(err))
		return
	}
	if _, err := uuid.Parse(strings.TrimSpace(req.UserUUID)); err != nil {
		dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(err))
		return
	}
	if strings.TrimSpace(req.Resource) != "iam.member" || strings.TrimSpace(req.Action) != "read" {
		dto.RespondErrorFrom(c, iamsvc.DirectoryInvalidArgumentError(errors.New("unsupported IAM authorization resource/action")))
		return
	}
	result, err := h.directory.CheckAuthorization(c.Request.Context(), tenantUUID, iamsvc.AuthorizationCheckInput{MemberUUID: req.MemberUUID, UserUUID: req.UserUUID, Resource: req.Resource, Action: req.Action, TraceID: req.TraceID})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		dto.RespondErrorFrom(c, iamsvc.DirectoryMemberNotFoundError(err))
		return
	}
	if err != nil {
		var coded *dto.AppError
		if errors.As(err, &coded) {
			dto.RespondErrorFrom(c, err)
			return
		}
		dto.RespondErrorFrom(c, iamsvc.DirectoryUpstreamDependencyError(err))
		return
	}
	dto.ResponseSuccess(c, result)
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
