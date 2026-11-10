package auth

import (
	"context"
	"testing"
	"time"

	pkgauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func sessionFixture(t *testing.T) (*AuthService, *gorm.DB, reqctx.CoreXClaims) {
	t.Helper()
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/session.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, db.AutoMigrate(&model.RefreshToken{}, &model.User{}, &model.Member{}, &model.Role{}, &model.RoleBinding{}, &tenant.Tenant{}))
	u := model.User{Email: "session-fixture@example.invalid", IsRoot: true, Status: 1}
	ten := tenant.Tenant{Key: "session-fixture", Name: "fixture", Status: 1}
	require.NoError(t, db.Create(&u).Error)
	require.NoError(t, db.Create(&ten).Error)
	s := NewAuthService(db, AuthOptions{JWTSecret: []byte("controlled-session-key"), Issuer: "powerx-auth", Audience: "user", AccessTTL: 720 * time.Hour, RefreshTTL: 336 * time.Hour})
	s.Cache = nil
	return s, db, reqctx.CoreXClaims{TenantUUID: ten.UUID.String(), TenantID: ten.ID, UserUUID: u.UUID.String(), UserID: u.ID, IsRoot: true}
}
func parseSessionToken(t *testing.T, s *AuthService, token string) *reqctx.CoreXClaims {
	t.Helper()
	c, err := pkgauth.ParseAndValidate(token, s.JWTSecret, s.Issuer, s.Audience)
	require.NoError(t, err)
	return c
}
func sessionCode(t *testing.T, err error, status int, code string) {
	t.Helper()
	var e *dto.AppError
	require.ErrorAs(t, err, &e)
	require.Equal(t, code, e.Code)
	require.Equal(t, status, e.HTTPCode)
}
func TestUserSessionIssuanceClampsAccessAndPreservesFixedRefreshDeadline(t *testing.T) {
	s, db, c := sessionFixture(t)
	access, refresh, err := s.issueSessionTokens(context.Background(), c)
	require.NoError(t, err)
	a, r := parseSessionToken(t, s, access), parseSessionToken(t, s, refresh)
	require.Equal(t, r.ID, a.ID)
	require.False(t, a.ExpiresAt.After(r.ExpiresAt.Time))
	require.LessOrEqual(t, s.AccessExpiresIn(access), int(s.RefreshTTL.Seconds()))
	var row model.RefreshToken
	require.NoError(t, db.First(&row, "jti = ?", r.ID).Error)
	require.Equal(t, r.ExpiresAt.UnixMilli(), row.ExpiresAt)
	// 剩余会话不足 AccessTTL 时，仅签发剩余期限，不更新原会话到期时间。
	deadline := time.Now().Add(2 * time.Minute).Truncate(time.Second).UnixMilli()
	require.NoError(t, db.Model(&row).Update("expires_at", deadline).Error)
	renewed, err := s.Refresh(context.Background(), refresh)
	require.NoError(t, err)
	next := parseSessionToken(t, s, renewed)
	require.Equal(t, r.ID, next.ID)
	require.LessOrEqual(t, next.ExpiresAt.UnixMilli(), deadline)
	require.NoError(t, db.First(&row, "jti = ?", r.ID).Error)
	require.Equal(t, deadline, row.ExpiresAt)
}
func TestUserSessionProbeRejectsExpiredRevokedMissingAndForeignSessions(t *testing.T) {
	for _, scenario := range []string{"expired", "revoked", "missing", "mismatch", "service"} {
		t.Run(scenario, func(t *testing.T) {
			s, db, c := sessionFixture(t)
			access, refresh, err := s.issueSessionTokens(context.Background(), c)
			require.NoError(t, err)
			claims := parseSessionToken(t, s, access)
			r := parseSessionToken(t, s, refresh)
			switch scenario {
			case "expired":
				require.NoError(t, db.Model(&model.RefreshToken{}).Where("jti = ?", r.ID).Update("expires_at", time.Now().Add(-time.Hour).UnixMilli()).Error)
			case "revoked":
				require.NoError(t, s.RTRepo.RevokeByJTI(context.Background(), r.ID, time.Now().UnixMilli()))
			case "missing":
				require.NoError(t, db.Where("jti = ?", r.ID).Delete(&model.RefreshToken{}).Error)
			case "mismatch":
				claims.TenantUUID = "different-tenant"
			case "service":
				claims.Issuer = "powerx-sts"
				claims.UserUUID = ""
			}
			ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), claims.TenantUUID), claims)
			_, err = s.CheckUserSession(ctx)
			code := "AUTH_SESSION_INVALID"
			if scenario == "expired" {
				code = "AUTH_SESSION_EXPIRED"
			}
			if scenario == "revoked" {
				code = "AUTH_SESSION_REVOKED"
			}
			sessionCode(t, err, 401, code)
			if scenario == "expired" || scenario == "revoked" || scenario == "missing" {
				_, err = s.Refresh(context.Background(), refresh)
				sessionCode(t, err, 401, code)
			}
		})
	}
}
func TestUserSessionProbeIsReadOnlyAndDatabaseFailureIsUnavailable(t *testing.T) {
	s, db, c := sessionFixture(t)
	access, refresh, err := s.issueSessionTokens(context.Background(), c)
	require.NoError(t, err)
	claims := parseSessionToken(t, s, access)
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), claims.TenantUUID), claims)
	var before model.RefreshToken
	require.NoError(t, db.First(&before, "jti = ?", claims.ID).Error)
	status, err := s.CheckUserSession(ctx)
	require.NoError(t, err)
	require.Equal(t, "active", status.Status)
	require.Equal(t, before.ExpiresAt, status.SessionExpiresAt)
	var after model.RefreshToken
	require.NoError(t, db.First(&after, "jti = ?", claims.ID).Error)
	require.Equal(t, before, after)
	pool, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, pool.Close())
	_, err = s.CheckUserSession(ctx)
	sessionCode(t, err, 503, "AUTH_SESSION_UNAVAILABLE")
	_, err = s.Refresh(context.Background(), refresh)
	sessionCode(t, err, 503, "AUTH_SESSION_UNAVAILABLE")
}
func TestUserSessionProbeSupportsMembersAndRejectsDisabledPrincipal(t *testing.T) {
	s, db, c := sessionFixture(t)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", c.UserID).Update("is_root", false).Error)
	member := model.Member{TenantUUID: c.TenantUUID, UserID: c.UserID, UserUUID: c.UserUUID, Username: "session-member", Status: 1}
	require.NoError(t, db.Create(&member).Error)
	c.IsRoot = false
	c.MemberID = member.ID
	c.MemberUUID = member.UUID.String()
	access, refresh, err := s.issueSessionTokens(context.Background(), c)
	require.NoError(t, err)
	claims := parseSessionToken(t, s, access)
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), c.TenantUUID), claims)
	_, err = s.CheckUserSession(ctx)
	require.NoError(t, err)
	renewed, err := s.Refresh(context.Background(), refresh)
	require.NoError(t, err)
	require.Equal(t, claims.ID, parseSessionToken(t, s, renewed).ID)
	require.NoError(t, db.Model(&member).Update("status", 2).Error)
	_, err = s.CheckUserSession(ctx)
	sessionCode(t, err, 403, "AUTH_USER_FORBIDDEN")
}
