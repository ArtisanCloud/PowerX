package auth

import (
	"context"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

// IsPlatformRootSession 检查已通过 JWT 验证的访问用户会话。
// 本函数不替代签名、有效期、受众和吊销检查；HTTP 路由仍须先经过 JwtMiddleware。
// 租户角色、API Key、STS、插件身份及单独注入的 Root 标志不能获得整机权限。
func IsPlatformRootSession(ctx context.Context) bool {
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.UserID == 0 || !claims.IsRoot || !reqctx.IsRoot(ctx) || claims.Scope != "access" || claims.PluginID != "" || strings.EqualFold(strings.TrimSpace(claims.Issuer), "powerx-sts") {
		return false
	}
	for _, plane := range claims.Platforms {
		switch strings.ToLower(strings.TrimSpace(plane)) {
		case "api_key", "service_actor":
			return false
		}
	}
	return true
}
