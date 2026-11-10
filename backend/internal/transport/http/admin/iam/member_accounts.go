package iam

import (
	"errors"
	"net/http"

	svc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *MemberHandler) authorizeManagement(c *gin.Context) {
	action := "write"
	if c.Request.Method == http.MethodGet {
		action = "read"
	} else if c.Request.Method == http.MethodDelete {
		action = "delete"
	}
	if err := h.S.AuthorizeManagement(c.Request.Context(), action); err != nil {
		dto.RespondErrorFrom(c, err)
		c.Abort()
		return
	}
	c.Next()
}

func (h *MemberHandler) requireManagedMember(c *gin.Context, tenant string, includeDeleted ...bool) (uint64, bool) {
	member, err := h.S.ResolveManagedMember(c.Request.Context(), tenant, c.Param("id"), includeDeleted...)
	if err != nil {
		respondMemberAccountError(c, err)
		return 0, false
	}
	return member.ID, true
}

func respondMemberAccountError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		dto.ResponseError(c, http.StatusNotFound, "当前租户中不存在该成员", nil)
		return
	}
	dto.RespondErrorFrom(c, err)
}

func (h *MemberHandler) ListRoles(c *gin.Context) {
	tenant, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	id, ok := h.requireManagedMember(c, tenant)
	if !ok {
		return
	}
	roles, err := h.S.MemberRoleUUIDs(c.Request.Context(), tenant, id)
	if err != nil {
		respondMemberAccountError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"role_uuids": roles})
}

func (h *MemberHandler) SetRoles(c *gin.Context) {
	tenant, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	id, ok := h.requireManagedMember(c, tenant)
	if !ok {
		return
	}
	var request struct {
		RoleUUIDs []string `json:"role_uuids" validate:"required,dive,uuid"`
	}
	if err := dto.ValidateRequestWithContext(c, &request); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if err := h.S.UpdateMemberByTenantUUID(c.Request.Context(), tenant, id, svc.UpdateMemberInput{RoleUUIDs: &request.RoleUUIDs}); err != nil {
		respondMemberAccountError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *MemberHandler) ResetPassword(c *gin.Context) {
	tenant, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	var request struct {
		NewPassword string `json:"new_password" validate:"required,min=6,max=64"`
	}
	if err := dto.ValidateRequestWithContext(c, &request); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if err := h.S.ResetMemberPassword(c.Request.Context(), tenant, c.Param("id"), request.NewPassword); err != nil {
		respondMemberAccountError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}
