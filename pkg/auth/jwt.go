package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// 统一的业务 Claims（sub 将使用 MemberUUID）
type CoreXClaims struct {
	TenantUUID string   `json:"tid"`
	TenantID   uint64   `json:"tid_n"`
	MemberUUID string   `json:"mid"` // 也会作为 Subject
	MemberID   uint64   `json:"mid_n"`
	UserUUID   string   `json:"uid"`
	UserID     uint64   `json:"uid_n"`
	IsRoot     bool     `json:"is_root"`
	Roles      []string `json:"roles,omitempty"`
	Platforms  []string `json:"plats,omitempty"`
	Scope      string   `json:"scope"` // "access" | "refresh"

	jwt.RegisteredClaims // Issuer, Subject, Audience, ExpiresAt, NotBefore, IssuedAt, ID(JTI)
}

// --- 签发 ---

func GenerateAccessJWT(c CoreXClaims, issuer string, audiences []string, ttl time.Duration, secret []byte) (string, error) {
	now := time.Now()
	c.Scope = "access"
	c.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   c.MemberUUID,                // sub = member.uuid
		Audience:  jwt.ClaimStrings(audiences), // 支持多 audience
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
	}
	//fmt2.Dump("GenerateAccessJWT", c)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return tok.SignedString(secret)
}

func GenerateRefreshJWT(c CoreXClaims, issuer string, audiences []string, jti string, ttl time.Duration, secret []byte) (string, error) {
	now := time.Now()
	c.Scope = "refresh"
	c.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   c.MemberUUID,
		Audience:  jwt.ClaimStrings(audiences),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ID:        jti, // JTI 用于撤销/刷新
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return tok.SignedString(secret)
}

// --- 解析 & 校验（v5 用 ParserOption）---

func ParseAndValidate(tokenString string, secret []byte, expectedIssuer string, expectedAudiences ...string) (*CoreXClaims, error) {
	claims := &CoreXClaims{}

	keyFunc := func(token *jwt.Token) (any, error) {
		// 只接受 HMAC 系列
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %T", token.Method)
		}
		return secret, nil
	}

	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"HS256", "HS384", "HS512"}),
		jwt.WithIssuer(expectedIssuer), // 校验 iss
		jwt.WithLeeway(30 * time.Second),
		jwt.WithTimeFunc(time.Now),
	}
	if len(expectedAudiences) > 0 {
		// 任一 audience 匹配即可（v5 的 WithAudience 支持可变参数）
		opts = append(opts, jwt.WithAudience(expectedAudiences...))
	}

	// 解析 + 校验（会自动跑 RegisteredClaims.Valid + 上述 Option）
	tkn, err := jwt.ParseWithClaims(tokenString, claims, keyFunc, opts...)
	if err != nil {
		return nil, err
	}
	if !tkn.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

/* --- 如仍有旧调用，保留兼容包装；否则可以删除 --- */

func GenerateJWT(
	tenantUUID, memberUUID string, platforms []string,
	audience, scope string, legacyIssuer string,
	ttl time.Duration, secret []byte) (string, error) {
	if scope == "refresh" {
		return "", errors.New("GenerateJWT(deprecated): refresh not supported, use GenerateRefreshJWT")
	}
	c := CoreXClaims{
		TenantUUID: tenantUUID,
		MemberUUID: memberUUID,
		Platforms:  platforms,
		Scope:      "access",
	}

	return GenerateAccessJWT(c, legacyIssuer, []string{audience}, ttl, secret)
}

func GenerateJWTWithJTI(tenantUUID, memberUUID string, platforms []string, audience, scope, jti string, ttl time.Duration, secret []byte) (string, error) {
	c := CoreXClaims{
		TenantUUID: tenantUUID,
		MemberUUID: memberUUID,
		Platforms:  platforms,
	}
	const legacyIssuer = "powerx-auth"
	if scope == "refresh" {
		return GenerateRefreshJWT(c, legacyIssuer, []string{audience}, jti, ttl, secret)
	}
	return GenerateAccessJWT(c, legacyIssuer, []string{audience}, ttl, secret)
}
