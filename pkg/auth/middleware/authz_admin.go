package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// AdminOnlyMiddleware 允许 root 或具备指定角色代码的用户访问
// 不传 allowed 时，默认放行 system_admin / role_admin
func AdminOnlyMiddleware(allowed ...iam.RoleCode) gin.HandlerFunc {
	allow := map[iam.RoleCode]struct{}{
		iam.CodeSystemAdmin: {},
		iam.CodeRoleAdmin:   {},
	}
	if len(allowed) > 0 {
		allow = map[iam.RoleCode]struct{}{}
		for _, r := range allowed {
			allow[r] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		claims := reqctx.GetClaims(c.Request.Context())
		if claims == nil {
			dto.ResponseError(c, http.StatusUnauthorized, "admin only unauthorized", nil)
			c.Abort()
			return
		}
		// root 直接放行
		if claims.IsRoot {
			c.Next()
			return
		}
		// 角色命中其一则放行
		ok := false
		for _, r := range claims.Roles {
			rc := iam.RoleCode(strings.ToLower(strings.TrimSpace(r)))
			if _, exists := allow[rc]; exists {
				ok = true
				break
			}
		}
		if !ok {
			dto.ResponseError(c, http.StatusForbidden, "forbidden: admin only", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
