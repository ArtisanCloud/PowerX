// Package auth 提供JWT认证和授权功能
package auth

import (
	"github.com/golang-jwt/jwt/v5"
)

// CoreXClaims 自定义JWT声明
type CoreXClaims struct {
	TenantID string           `json:"tenant_id"` // 租户ID
	Platform string           `json:"platform"`  // 平台标识
	Scope    string           `json:"scope"`     // 权限范围
	Audience jwt.ClaimStrings `json:"aud"`       // 受众
	Subject  string           `json:"sub"`       // 主体
	jwt.RegisteredClaims
}
