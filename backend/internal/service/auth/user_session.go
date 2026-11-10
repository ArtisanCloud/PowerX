package auth

import (
	"context"
	"errors"
	"time"

	pkgauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func authFailure(status int, code, reason string) *dto.AppError {
	return &dto.AppError{HTTPCode: status, Code: code, Message: code, Details: map[string]any{"reason": reason}}
}
func sessionLookupError(err error) error {
	var business *dto.AppError
	if errors.As(err, &business) {
		return business
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return authFailure(401, "AUTH_SESSION_INVALID", "session_identity_not_found")
	}
	return authFailure(503, "AUTH_SESSION_UNAVAILABLE", "session_lookup_failed")
}

// loginSession 校验已有登录记录，不创建、不续长，也不修改撤销状态。
func (s *AuthService) loginSession(ctx context.Context, claims *reqctx.CoreXClaims) (*model.RefreshToken, error) {
	if s == nil || s.RTRepo == nil {
		return nil, authFailure(503, "AUTH_SESSION_UNAVAILABLE", "session_store_unavailable")
	}
	if claims == nil || claims.ID == "" {
		return nil, authFailure(401, "AUTH_SESSION_INVALID", "session_id_missing")
	}
	rt, err := s.RTRepo.GetByJTI(ctx, claims.ID)
	if err != nil {
		return nil, sessionLookupError(err)
	}
	if rt == nil {
		return nil, authFailure(401, "AUTH_SESSION_INVALID", "session_not_found")
	}
	if rt.RevokedAt != nil {
		return nil, authFailure(401, "AUTH_SESSION_REVOKED", "session_revoked")
	}
	if rt.ExpiresAt <= time.Now().UnixMilli() {
		return nil, authFailure(401, "AUTH_SESSION_EXPIRED", "session_expired")
	}
	if rt.MemberUUID != claims.MemberUUID || rt.TenantUUID != claims.TenantUUID || rt.UserUUID != claims.UserUUID {
		return nil, authFailure(401, "AUTH_SESSION_INVALID", "session_identity_mismatch")
	}
	return rt, nil
}

// issueSessionTokens 以 refresh JWT 的固定截止时间作为数据库和 access 的共同上限。
func (s *AuthService) issueSessionTokens(ctx context.Context, claims reqctx.CoreXClaims) (string, string, error) {
	if s.AccessTTL <= 0 || s.RefreshTTL <= 0 {
		return "", "", authFailure(503, "AUTH_SESSION_UNAVAILABLE", "invalid_session_ttl")
	}
	claims.ID = uuid.NewString()
	refresh, err := pkgauth.GenerateRefreshJWT(claims, s.Issuer, []string{s.Audience}, claims.ID, s.RefreshTTL, s.JWTSecret)
	if err != nil {
		return "", "", sessionLookupError(err)
	}
	refreshClaims, err := pkgauth.ParseAndValidate(refresh, s.JWTSecret, s.Issuer, s.Audience)
	if err != nil || refreshClaims.ExpiresAt == nil {
		return "", "", authFailure(503, "AUTH_SESSION_UNAVAILABLE", "session_signing_failed")
	}
	deadline := refreshClaims.ExpiresAt.Time
	access, err := s.refreshSessionAccess(claims, deadline)
	if err != nil {
		return "", "", err
	}
	if err = s.RTRepo.Issue(ctx, &model.RefreshToken{JTI: claims.ID, TenantUUID: claims.TenantUUID, MemberUUID: claims.MemberUUID, UserUUID: claims.UserUUID, ExpiresAt: deadline.UnixMilli()}); err != nil {
		return "", "", sessionLookupError(err)
	}
	return access, refresh, nil
}
func (s *AuthService) refreshSessionAccess(claims reqctx.CoreXClaims, deadline time.Time) (string, error) {
	if deadline.Unix() <= time.Now().Unix() {
		return "", authFailure(401, "AUTH_SESSION_EXPIRED", "session_expired")
	}
	access, err := pkgauth.GenerateAccessJWTUntil(claims, s.Issuer, []string{s.Audience}, s.AccessTTL, s.JWTSecret, deadline)
	if err != nil {
		return "", authFailure(503, "AUTH_SESSION_UNAVAILABLE", "access_signing_failed")
	}
	return access, nil
}

// AccessExpiresIn 用已签发 token 的真实期限回包，不能再使用未裁剪的 AccessTTL。
func (s *AuthService) AccessExpiresIn(access string) int {
	claims, err := pkgauth.ParseAndValidate(access, s.JWTSecret, s.Issuer, s.Audience)
	if err != nil || claims.ExpiresAt == nil {
		return 0
	}
	seconds := int(time.Until(claims.ExpiresAt.Time) / time.Second)
	if seconds < 0 {
		return 0
	}
	return seconds
}

type UserSessionStatus struct {
	Status           string `json:"status"`
	SessionExpiresAt int64  `json:"session_expires_at"`
	AccessExpiresAt  int64  `json:"access_expires_at"`
}

// CheckUserSession 只检查已认证的当前用户；请求不能指定其他用户、租户或会话 ID。
func (s *AuthService) CheckUserSession(ctx context.Context) (*UserSessionStatus, error) {
	if s == nil {
		return nil, authFailure(503, "AUTH_SESSION_UNAVAILABLE", "session_service_unavailable")
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.Issuer != s.Issuer || claims.Scope != "access" || claims.UserUUID == "" || claims.TenantUUID != reqctx.GetTenantUUID(ctx) {
		return nil, authFailure(401, "AUTH_SESSION_INVALID", "user_credential_required")
	}
	audience := false
	for _, a := range claims.Audience {
		if a == s.Audience {
			audience = true
		}
	}
	if !audience {
		return nil, authFailure(401, "AUTH_SESSION_INVALID", "user_audience_invalid")
	}
	if claims.ExpiresAt == nil || !time.Now().Before(claims.ExpiresAt.Time) {
		return nil, authFailure(401, "AUTH_ACCESS_EXPIRED", "access_expired")
	}
	rt, err := s.loginSession(ctx, claims)
	if err != nil {
		return nil, err
	}
	u, err := s.UserRepo.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, sessionLookupError(err)
	}
	if u == nil || u.UUID.String() != claims.UserUUID || u.IsRoot != claims.IsRoot {
		return nil, authFailure(401, "AUTH_SESSION_INVALID", "user_identity_mismatch")
	}
	if u.Status != 1 {
		return nil, authFailure(403, "AUTH_USER_FORBIDDEN", "user_disabled")
	}
	ten, err := s.tenantByUUID(ctx, claims.TenantUUID)
	if err != nil {
		return nil, sessionLookupError(err)
	}
	if ten == nil {
		return nil, authFailure(401, "AUTH_SESSION_INVALID", "tenant_not_found")
	}
	if ten.Status != 1 {
		return nil, authFailure(403, "AUTH_USER_FORBIDDEN", "tenant_disabled")
	}
	if !u.IsRoot || claims.MemberUUID != "" {
		member, err := s.MemberRepo.GetByCondition(ctx, map[string]interface{}{"id = ?": claims.MemberID, "tenant_uuid = ?": claims.TenantUUID}, nil)
		if err != nil {
			return nil, sessionLookupError(err)
		}
		if member == nil || member.UUID.String() != claims.MemberUUID || member.UserID != u.ID {
			return nil, authFailure(401, "AUTH_SESSION_INVALID", "member_identity_mismatch")
		}
		if member.Status != 1 {
			return nil, authFailure(403, "AUTH_USER_FORBIDDEN", "member_disabled")
		}
	}
	return &UserSessionStatus{Status: "active", SessionExpiresAt: rt.ExpiresAt, AccessExpiresAt: claims.ExpiresAt.UnixMilli()}, nil
}
