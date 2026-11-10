package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	pkgauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func authHTTPFixture(t *testing.T) (*authsvc.AuthService, *gorm.DB, reqctx.CoreXClaims, string) {
	t.Helper()
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/auth.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	require.NoError(t, db.AutoMigrate(&iam.RefreshToken{}, &iam.User{}, &tenant.Tenant{}))
	user := iam.User{IsRoot: true, Email: "fixture@example.invalid", Status: 1}
	ten := tenant.Tenant{Key: "fixture", Name: "fixture", Status: 1}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&ten).Error)
	s := authsvc.NewAuthService(db, authsvc.AuthOptions{JWTSecret: []byte("controlled-http-auth"), Issuer: "powerx-auth", Audience: "user", AccessTTL: 720 * time.Hour, RefreshTTL: 336 * time.Hour})
	s.Cache = nil
	c := reqctx.CoreXClaims{UserID: user.ID, UserUUID: user.UUID.String(), TenantUUID: ten.UUID.String(), TenantID: ten.ID, IsRoot: true}
	c.ID = "controlled-session"
	refresh, err := pkgauth.GenerateRefreshJWT(c, s.Issuer, []string{s.Audience}, c.ID, s.RefreshTTL, s.JWTSecret)
	require.NoError(t, err)
	access, err := pkgauth.GenerateAccessJWT(c, s.Issuer, []string{s.Audience}, s.AccessTTL, s.JWTSecret)
	require.NoError(t, err)
	parsed, err := pkgauth.ParseAndValidate(access, s.JWTSecret, s.Issuer, s.Audience)
	require.NoError(t, err)
	require.NoError(t, db.Create(&iam.RefreshToken{JTI: c.ID, UserUUID: c.UserUUID, TenantUUID: c.TenantUUID, ExpiresAt: time.Now().Add(time.Minute).Truncate(time.Second).UnixMilli()}).Error)
	return s, db, *parsed, refresh
}
func TestRefreshHTTPHasTypedTerminalAndUnavailableResponses(t *testing.T) {
	for _, scenario := range []string{"expired", "revoked", "database", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			s, db, claims, refresh := authHTTPFixture(t)
			status, code := 401, "AUTH_SESSION_EXPIRED"
			switch scenario {
			case "expired":
				require.NoError(t, db.Model(&iam.RefreshToken{}).Where("jti = ?", claims.ID).Update("expires_at", time.Now().Add(-time.Second).UnixMilli()).Error)
			case "revoked":
				require.NoError(t, s.RTRepo.RevokeByJTI(context.Background(), claims.ID, time.Now().UnixMilli()))
				code = "AUTH_SESSION_REVOKED"
			case "database":
				pool, err := db.DB()
				require.NoError(t, err)
				require.NoError(t, pool.Close())
				status = 503
				code = "AUTH_SESSION_UNAVAILABLE"
			case "invalid":
				refresh = "invalid"
				code = "AUTH_REFRESH_INVALID"
			}
			body, _ := json.Marshal(map[string]any{"refresh_token": refresh})
			response := httptest.NewRecorder()
			ctx, _ := ginContext(response, http.MethodPost, "/api/v1/admin/user/auth/refresh", string(body))
			(&AuthUserHandler{AuthService: s}).RefreshHandler(s)(ctx)
			require.Equal(t, status, response.Code)
			require.Contains(t, response.Body.String(), code)
			require.NotContains(t, response.Body.String(), "database is closed")
		})
	}
}
func TestRefreshHTTPExpiresInMatchesActualJWTAndOmitsRefreshToken(t *testing.T) {
	s, _, _, refresh := authHTTPFixture(t)
	body, _ := json.Marshal(map[string]any{"refresh_token": refresh})
	response := httptest.NewRecorder()
	ctx, _ := ginContext(response, "POST", "/api/v1/admin/user/auth/refresh", string(body))
	(&AuthUserHandler{AuthService: s}).RefreshHandler(s)(ctx)
	require.Equal(t, 200, response.Code)
	var result struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.NotContains(t, result.Data, "refresh_token")
	require.LessOrEqual(t, result.Data["expires_in"].(float64), float64(60))
	require.Greater(t, result.Data["expires_in"].(float64), float64(0))
}
func TestCurrentSessionHTTPUsesAuthenticatedIdentityAndRejectsOverrides(t *testing.T) {
	s, db, claims, _ := authHTTPFixture(t)
	h := &AuthUserHandler{AuthService: s}
	for _, request := range []struct {
		path, body string
		status     int
	}{{"/api/v1/admin/user/auth/me/session", "", 200}, {"/api/v1/admin/user/auth/me/session?tenant_uuid=other", "", 400}, {"/api/v1/admin/user/auth/me/session", `{"session_id":"other"}`, 400}} {
		response := httptest.NewRecorder()
		ctx, _ := ginContext(response, "GET", request.path, request.body)
		ctx.Request = ctx.Request.WithContext(reqctx.WithClaims(reqctx.WithTenantUUID(ctx.Request.Context(), claims.TenantUUID), &claims))
		h.CurrentSession(ctx)
		require.Equal(t, request.status, response.Code)
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	}
	require.NoError(t, db.Model(&iam.RefreshToken{}).Where("jti = ?", claims.ID).Update("expires_at", time.Now().Add(-time.Hour).UnixMilli()).Error)
	response := httptest.NewRecorder()
	ctx, _ := ginContext(response, "GET", "/api/v1/admin/user/auth/me/session", "")
	ctx.Request = ctx.Request.WithContext(reqctx.WithClaims(reqctx.WithTenantUUID(ctx.Request.Context(), claims.TenantUUID), &claims))
	h.CurrentSession(ctx)
	require.Equal(t, 401, response.Code)
	require.Contains(t, response.Body.String(), "AUTH_SESSION_EXPIRED")
}

func ginContext(response *httptest.ResponseRecorder, method, path, body string) (*gin.Context, *gin.Engine) {
	c, e := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, e
}
