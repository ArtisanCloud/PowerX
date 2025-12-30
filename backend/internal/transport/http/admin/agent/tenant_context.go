package agent

import (
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

// tenantContext 仅封装 UUID，后续完全依赖字符串租户标识。
type tenantContext struct {
	uuid string
}

func (t *tenantContext) UUID() string {
	if t == nil {
		return ""
	}
	return t.uuid
}

func (t *tenantContext) UUIDPtr() *string {
	if t == nil {
		return nil
	}
	val := strings.TrimSpace(t.uuid)
	if val == "" {
		return nil
	}
	return &val
}

func requireTenantContext(c *gin.Context) (*tenantContext, error) {
	uuid, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		return nil, err
	}
	return &tenantContext{
		uuid: strings.TrimSpace(uuid),
	}, nil
}
