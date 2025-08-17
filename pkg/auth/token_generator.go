package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateJWT 生成JWT令牌 (HMAC版本)
func GenerateJWT(tenantID, subject, platform, audience, scope string, ttl time.Duration, secret []byte) (string, error) {
	now := time.Now()
	claims := CoreXClaims{
		TenantID: tenantID,
		Platform: platform,
		Scope:    scope,
		Audience: jwt.ClaimStrings{audience},
		Subject:  subject,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "corex-auth",                     // 签发者
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)), // 过期时间
			IssuedAt:  jwt.NewNumericDate(now),          // 签发时间
			NotBefore: jwt.NewNumericDate(now),          // 生效时间
			ID:        "",                               // 可填uuid作为jti
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// GenerateJWTWithJTI 生成带有JTI的JWT令牌
func GenerateJWTWithJTI(tenantID, subject, platform, audience, scope, jti string, ttl time.Duration, secret []byte) (string, error) {
	now := time.Now()
	claims := CoreXClaims{
		TenantID: tenantID,
		Platform: platform,
		Scope:    scope,
		Audience: jwt.ClaimStrings{audience},
		Subject:  subject,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "corex-auth",                     // 签发者
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)), // 过期时间
			IssuedAt:  jwt.NewNumericDate(now),          // 签发时间
			NotBefore: jwt.NewNumericDate(now),          // 生效时间
			ID:        jti,                              // JWT ID
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}
