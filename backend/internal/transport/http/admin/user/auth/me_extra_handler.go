package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
)

type MeExtraHandler struct {
	me  *authsvc.MeService
	org *iamsvc.OrgService
}

func NewMeExtraHandler(deps *shared.Deps) *MeExtraHandler {
	return &MeExtraHandler{
		me:  deps.MeService,
		org: iamsvc.NewOrgService(deps.DB),
	}
}

type switchTenantReq struct {
	TenantUUID string `json:"tenant_uuid"`
}

// POST /api/v1/admin/user/auth/me/switch-tenant
func (h *MeExtraHandler) SwitchTenant(c *gin.Context) {
	var req switchTenantReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	target := strings.TrimSpace(req.TenantUUID)
	if target == "" {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid required", nil)
		return
	}

	// 先用当前 ctx 拉一份 members，校验用户确实属于该租户（避免 root 误切到不存在成员的租户导致 member_id 语义混乱）
	baseCtx := c.Request.Context()
	meCtx, err := h.me.GetMeContext(baseCtx)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取上下文失败", err)
		return
	}

	allowed := false
	for _, m := range meCtx.Members {
		if strings.EqualFold(strings.TrimSpace(m.TenantUUID), target) {
			allowed = true
			break
		}
	}
	if !allowed {
		dto.ResponseError(c, http.StatusForbidden, "no membership in tenant", nil)
		return
	}

	// 用目标 tenantUUID 重建一个“视图上下文”，让后续依赖 reqctx.GetTenantUUID 的逻辑对齐
	nextCtx := reqctx.WithTenantUUID(baseCtx, target)
	c.Request = c.Request.WithContext(nextCtx)

	resp, err := h.me.GetMeContext(nextCtx)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取上下文失败", err)
		return
	}
	dto.ResponseSuccess(c, resp)
}

// GET /api/v1/admin/user/auth/me/tenants
func (h *MeExtraHandler) ListTenants(c *gin.Context) {
	resp, err := h.me.GetMeContext(c.Request.Context())
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取上下文失败", err)
		return
	}
	dto.ResponseSuccess(c, resp.Members)
}

// GET /api/v1/admin/user/auth/me/departments
func (h *MeExtraHandler) ListDepartments(c *gin.Context) {
	ctx := c.Request.Context()
	tenantUUID, err := reqctx.RequireTenantUUID(ctx)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant uuid required", err)
		return
	}

	tree, err := h.org.GetDepartmentTree(ctx, tenantUUID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取部门失败", err)
		return
	}

	type deptView struct {
		ID       uint64  `json:"id"`
		Name     string  `json:"name"`
		Code     string  `json:"code"`
		ParentID *uint64 `json:"parent_id,omitempty"`
	}

	out := make([]deptView, 0, 32)

	var walk func(nodes []*m.Department)
	walk = func(nodes []*m.Department) {
		for _, n := range nodes {
			out = append(out, deptView{
				ID:       n.ID,
				Name:     n.Name,
				Code:     n.Key,
				ParentID: n.ParentID,
			})
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(tree)
	dto.ResponseSuccess(c, out)
}
