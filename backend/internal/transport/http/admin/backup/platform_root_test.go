package backup

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 使用真实注册路由与签名 JWT；资源与数据库均为测试自有，不加载日常配置。
func TestAllRegisteredHostBackupRoutesRequirePlatformRootSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	require.NoError(t, db.AutoMigrate(&modelops.BackupPolicy{}))
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", t.TempDir())
	secret := []byte("owned-commercial-root-contract-secret")
	const issuer = "owned-commercial-root-contract"
	issue := func(root bool, user uint64, planes []string, plugin string) string {
		claims := reqctx.CoreXClaims{IsRoot: root, UserID: user, MemberID: 1, TenantID: 1, TenantUUID: uuid.NewString(), MemberUUID: uuid.NewString(), Roles: []string{"admin", "superadmin"}, PermissionCodes: []string{"ops:backup:read", "ops:backup:execute", "admin:root"}, Platforms: planes, PluginID: plugin}
		token, e := auth.GenerateAccessJWT(claims, issuer, []string{"admin"}, time.Hour, secret)
		require.NoError(t, e)
		return token
	}
	router := gin.New()
	router.Use(gin.Recovery(), authmw.JwtMiddleware(secret, issuer, []string{"admin"}, []string{"access"}, nil))
	RegisterAPIRoutes(nil, router.Group("/custom/v2"), &shared.Deps{DB: db})
	routes := router.Routes()
	require.Greater(t, len(routes), 70)
	parameter := regexp.MustCompile(`:[^/]+`)
	cases := []struct {
		name, token string
		status      int
	}{
		{"anonymous with forged root header", "", http.StatusUnauthorized},
		{"tenant admin with backup grants", issue(false, 2, nil, ""), http.StatusForbidden},
		{"API key with forged root claim", issue(true, 1, []string{"api_key"}, ""), http.StatusForbidden},
		{"service actor with forged root claim", issue(true, 1, []string{"service_actor"}, ""), http.StatusForbidden},
		{"plugin identity with root claim", issue(true, 1, nil, "plugin.test"), http.StatusForbidden},
		{"root without human user", issue(true, 0, nil, ""), http.StatusUnauthorized},
	}
	sts, err := auth.GenerateAccessJWT(reqctx.CoreXClaims{IsRoot: true, UserID: 1, Platforms: []string{"service_actor"}}, "powerx-sts", []string{"admin"}, time.Hour, secret)
	require.NoError(t, err)
	ordinary, err := auth.GenerateAccessJWT(reqctx.CoreXClaims{UserID: 3, MemberID: 3, MemberUUID: uuid.NewString(), TenantID: 1, TenantUUID: uuid.NewString()}, issuer, []string{"admin"}, time.Hour, secret)
	require.NoError(t, err)
	cases = append(cases, struct {
		name, token string
		status      int
	}{"signed STS token with Root claim", sts, http.StatusUnauthorized}, struct {
		name, token string
		status      int
	}{"ordinary signed user", ordinary, http.StatusForbidden})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, route := range routes {
				path := parameter.ReplaceAllString(route.Path, "1")
				request := httptest.NewRequest(route.Method, path, strings.NewReader(`{}`))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("X-PowerX-Root", "true")
				if tc.token != "" {
					request.Header.Set("Authorization", "Bearer "+tc.token)
				}
				out := httptest.NewRecorder()
				router.ServeHTTP(out, request)
				require.Equal(t, tc.status, out.Code, "%s %s: %s", route.Method, route.Path, out.Body.String())
			}
		})
	}
	// Root 读取真实新旧列表和类型校验，证明没有只会拒绝所有请求的伪修复。
	token := issue(true, 1, nil, "")
	for _, prefix := range []string{"/custom/v2/admin/ops/backup", "/custom/v2/admin/backup"} {
		request := httptest.NewRequest("GET", prefix+"/policies", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		out := httptest.NewRecorder()
		router.ServeHTTP(out, request)
		require.Equal(t, http.StatusOK, out.Code, out.Body.String())
		request = httptest.NewRequest("PATCH", prefix+"/jobs/0/protection", strings.NewReader(`{"protected":true}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
		out = httptest.NewRecorder()
		router.ServeHTTP(out, request)
		require.Equal(t, http.StatusBadRequest, out.Code, out.Body.String())
	}
	t.Logf("verified %d registered routes against %d denied identity types and allowed Root reads", len(routes), len(cases))
}

func TestPlatformRootGuardRejectsContextOnlyRootAndSTS(t *testing.T) {
	for _, test := range []struct {
		name   string
		claims *reqctx.CoreXClaims
		root   bool
		status int
	}{
		{name: "context only root", root: true, status: http.StatusUnauthorized},
		{name: "STS", claims: &reqctx.CoreXClaims{UserID: 1, IsRoot: true, Scope: "access"}, root: true, status: http.StatusForbidden},
		{name: "root claim missing", claims: &reqctx.CoreXClaims{UserID: 1, Scope: "access"}, root: true, status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				ctx := reqctx.WithIsRoot(c.Request.Context(), test.root)
				if test.claims != nil {
					if test.name == "STS" {
						test.claims.Issuer = "powerx-sts"
					}
					ctx = reqctx.WithClaims(ctx, test.claims)
				}
				c.Request = c.Request.WithContext(ctx)
			})
			router.GET("/host", RequireBackupRoot(), func(c *gin.Context) { c.Status(200) })
			out := httptest.NewRecorder()
			router.ServeHTTP(out, httptest.NewRequest("GET", "/host", nil))
			require.Equal(t, test.status, out.Code)
		})
	}
}
