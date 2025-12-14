package iam

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func requireTenantUUIDFromContext(c *gin.Context) (string, bool) {
	tenantUUID, err := reqctx.RequireTenantUUIDValueFromGin(c)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("缺少有效租户上下文", err))
		return "", false
	}
	return tenantUUID.String(), true
}
