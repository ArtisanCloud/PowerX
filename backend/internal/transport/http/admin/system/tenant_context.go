package system

import (
	"net/http"

	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type tenantContext struct {
	uuid string
}

func (t *tenantContext) UUID() string {
	if t == nil {
		return ""
	}
	return t.uuid
}

func requireTenantContext(c *gin.Context, repo *tenantrepo.TenantRepository) (*tenantContext, bool) {
	tenantUUID, err := reqctx.RequireTenantUUIDValueFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return nil, false
	}
	if repo == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "tenant repository 未配置", nil)
		return nil, false
	}
	tenant, err := repo.GetByUUID(c.Request.Context(), tenantUUID.String())
	if err != nil || tenant == nil || tenant.ID == 0 {
		dto.ResponseError(c, http.StatusBadRequest, "解析租户失败", err)
		return nil, false
	}
	return &tenantContext{uuid: tenant.UUID.String()}, true
}
