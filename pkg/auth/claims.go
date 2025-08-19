// pkg/auth/claims.go
package auth

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type CoreXClaims struct {
	TenantID string   `json:"tenant"` // 租户ID（字符串，建议存 "1" 这种）
	Platform string   `json:"platform,omitempty"`
	Scope    string   `json:"scope"` // "access" | "refresh"
	Audience []string `json:"aud,omitempty"`
	Subject  string   `json:"subject,omitempty"`
	Roles    []string `json:"roles,omitempty"` // 角色code数组
	Username string   `json:"username,omitempty"`
	jwt.RegisteredClaims
}

// ParseAndValidate：解析并校验 HMAC JWT（HS256）
func ParseAndValidate(tokenStr string, secret []byte, expectedIssuer string, expectedAud string) (*CoreXClaims, error) {
	claims := &CoreXClaims{}
	tok, err := jwt.ParseWithClaims(
		tokenStr,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return secret, nil
		},
		jwt.WithIssuer(expectedIssuer),
		jwt.WithAudience(expectedAud),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return nil, err
	}
	if !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// 小工具：把 claims.Subject（字符串）转 uint64
func SubjectUint64(claims *CoreXClaims) (uint64, error) {
	return strconv.ParseUint(strings.TrimSpace(claims.Subject), 10, 64)
}
